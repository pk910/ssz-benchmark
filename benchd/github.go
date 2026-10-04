package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ghApp authenticates as the installation of a GitHub App on the
// repository: a JWT signed with the App's key buys an installation token
// that is good for an hour.
type ghApp struct {
	appID          string
	installationID string
	key            *rsa.PrivateKey
	api            string // API root, replaced in tests
	client         *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func loadGHApp(appID, installationID, keyPath string) (*ghApp, error) {
	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s: no PEM block", keyPath)
	}
	var key *rsa.PrivateKey
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		key = k
	} else if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("%s: not an RSA key", keyPath)
		}
		key = rk
	} else {
		return nil, fmt.Errorf("%s: %w", keyPath, err)
	}
	return &ghApp{appID: appID, installationID: installationID, key: key, api: "https://api.github.com", client: &http.Client{Timeout: 30 * time.Second}}, nil
}

func b64(data []byte) string { return base64.RawURLEncoding.EncodeToString(data) }

// jwt is the App's short-lived identity assertion (RS256).
func (a *ghApp) jwt(now time.Time) (string, error) {
	header := b64([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"iat": now.Add(-30 * time.Second).Unix(), "exp": now.Add(9 * time.Minute).Unix(), "iss": a.appID})
	signing := header + "." + b64(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64(sig), nil
}

// installationToken returns a valid installation token, renewing it a few
// minutes before it expires.
func (a *ghApp) installationToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token != "" && time.Until(a.expires) > 5*time.Minute {
		return a.token, nil
	}
	jwt, err := a.jwt(time.Now())
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", a.api+"/app/installations/"+a.installationID+"/access_tokens", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "benchd")
	resp, err := a.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("installation token: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", err
	}
	a.token, a.expires = out.Token, out.ExpiresAt
	return a.token, nil
}

// call makes one API request as the installation.
func (a *ghApp) call(ctx context.Context, method, path string, in, out any) error {
	token, err := a.installationToken(ctx)
	if err != nil {
		return err
	}
	var body io.Reader
	if in != nil {
		data, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, a.api+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "benchd")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// checkName is the name of the check run on a commit.
const checkName = "benchmark"

// checkReporter mirrors the state of commit jobs into GitHub check runs:
// queued, in progress with the provisional tables after every pass,
// completed with the final ones. The check never fails a commit; it
// informs.
type checkReporter struct {
	app       *ghApp
	repo      string // owner/name
	db        *store
	local     string // name of the controller's runner, for the noise floor
	publicURL string // root of the web UI, for links
	sinceJob  int64  // jobs up to this id predate the reporter

	mu sync.Mutex // one sync at a time: a check is created once
}

// newCheckReporter starts reporting with the jobs queued from now on: the
// first start records the newest job id, and nothing up to it gets a check.
func newCheckReporter(app *ghApp, cfg *config, db *store) (*checkReporter, error) {
	c := &checkReporter{app: app, repo: cfg.github, db: db, local: cfg.name, publicURL: cfg.publicURL}
	v, err := db.getKV("checks:since")
	if err != nil {
		return nil, err
	}
	if v == "" {
		last, err := db.maxJobID()
		if err != nil {
			return nil, err
		}
		v = strconv.FormatInt(last, 10)
		if err := db.setKV("checks:since", v); err != nil {
			return nil, err
		}
	}
	c.sinceJob, _ = strconv.ParseInt(v, 10, 64)
	return c, nil
}

// checkPayload is the body of a check run create or update.
type checkPayload struct {
	Name       string       `json:"name"`
	HeadSHA    string       `json:"head_sha,omitempty"`
	Status     string       `json:"status"`
	Conclusion string       `json:"conclusion,omitempty"`
	DetailsURL string       `json:"details_url,omitempty"`
	Output     *checkOutput `json:"output,omitempty"`
}

type checkOutput struct {
	Title   string `json:"title"`
	Summary string `json:"summary"`
}

// reported says whether a job gets a check: a commit of the repository
// measured against its base, queued after the reporter was switched on.
func (c *checkReporter) reported(j *job) bool {
	return c != nil && j.Kind == kindCommit && j.ID > c.sinceJob && len(j.HeadSHA) == 40
}

// sync brings the job's check run in line with the job. It is idempotent:
// an unchanged payload is not sent again.
func (c *checkReporter) sync(ctx context.Context, j *job) {
	if !c.reported(j) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	checkID, digest, final, err := c.db.jobCheck(j.ID)
	if err != nil || final {
		return
	}
	payload, done := c.payload(j)
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	newDigest := hex.EncodeToString(sum[:8])
	if newDigest == digest {
		return
	}
	if checkID == 0 {
		payload.HeadSHA = j.HeadSHA
		var out struct {
			ID int64 `json:"id"`
		}
		if err := c.app.call(ctx, "POST", "/repos/"+c.repo+"/check-runs", payload, &out); err != nil {
			log.Printf("check for job %d: %v", j.ID, err)
			return
		}
		checkID = out.ID
	} else if err := c.app.call(ctx, "PATCH", fmt.Sprintf("/repos/%s/check-runs/%d", c.repo, checkID), payload, nil); err != nil {
		log.Printf("check for job %d: %v", j.ID, err)
		return
	}
	if err := c.db.setJobCheck(j.ID, checkID, newDigest, done); err != nil {
		log.Printf("check for job %d: %v", j.ID, err)
	}
}

// removeLabel takes a label off a pull request; failing to is harmless.
func (c *checkReporter) removeLabel(ctx context.Context, number int, label string) {
	if c == nil {
		return
	}
	if err := c.app.call(ctx, "DELETE", fmt.Sprintf("/repos/%s/issues/%d/labels/%s", c.repo, number, url.PathEscape(label)), nil, nil); err != nil {
		log.Printf("pull request #%d: label %s not removed: %v", number, label, err)
	}
}

// syncJob is sync for a job id, off the caller's path.
func (c *checkReporter) syncJob(id int64) {
	if c == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if j, err := c.db.getJob(id); err == nil && j != nil {
			c.sync(ctx, j)
		}
	}()
}

// syncOpen syncs every reported job whose check is not final yet.
func (c *checkReporter) syncOpen(ctx context.Context) {
	if c == nil {
		return
	}
	jobs, err := c.db.listJobs(500, kindCommit)
	if err != nil {
		return
	}
	for i := len(jobs) - 1; i >= 0; i-- {
		c.sync(ctx, jobs[i])
	}
}

// payload is the check run a job should show now, and whether that is its
// final form.
func (c *checkReporter) payload(j *job) (checkPayload, bool) {
	p := checkPayload{Name: checkName, DetailsURL: c.jobURL(j)}
	switch j.State {
	case stateQueued:
		p.Status = "queued"
		ahead, _ := c.db.queuedAhead(j)
		p.Output = &checkOutput{Title: "Waiting for the benchmark machine", Summary: fmt.Sprintf("%s\n\n%d job(s) ahead in the queue. A job takes about 35 minutes; the first numbers appear here after its first pass.", c.pairLine(j), ahead)}
		return p, false
	case stateRunning:
		p.Status = "in_progress"
		samples, _ := c.db.samplesFor(j.ID)
		passes := 0
		for _, sm := range samples {
			passes = max(passes, sm.Pass+1)
		}
		if passes == 0 {
			p.Output = &checkOutput{Title: "Building and measuring", Summary: c.pairLine(j) + "\n\nThe first numbers appear after the first pass."}
			return p, false
		}
		results := summarize(samples)
		title, summary := renderCheck(j, results, c.db.noiseFloor(c.local), c.jobURL(j), fmt.Sprintf("provisional, %d pass(es) so far", passes))
		p.Output = &checkOutput{Title: title, Summary: summary}
		return p, false
	case stateDone:
		p.Status, p.Conclusion = "completed", "neutral"
		results, _ := c.db.resultsFor(j.ID)
		title, summary := renderCheck(j, results, c.db.noiseFloor(c.local), c.jobURL(j), fmt.Sprintf("%d passes", j.Passes))
		p.Output = &checkOutput{Title: title, Summary: summary}
		return p, true
	case stateSkipped:
		p.Status, p.Conclusion = "completed", "skipped"
		p.Output = &checkOutput{Title: "Superseded by a newer commit", Summary: c.pairLine(j) + "\n\n" + j.Note}
		return p, true
	default: // failed
		p.Status, p.Conclusion = "completed", "neutral"
		p.Output = &checkOutput{Title: "The benchmark could not run", Summary: c.pairLine(j) + "\n\n```\n" + truncate(j.Error, 4000) + "\n```"}
		return p, true
	}
}

func (c *checkReporter) jobURL(j *job) string {
	if c.publicURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/#/job/%d", strings.TrimRight(c.publicURL, "/"), j.ID)
}

func (c *checkReporter) pairLine(j *job) string {
	return fmt.Sprintf("`%s` against `%s` (%s)", shortSHA(j.HeadSHA), shortSHA(j.BaseSHA), j.BaseRef)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "\n…"
}

// checkMetric picks the quantity a result is judged by: cycles of the
// measured thread, or wall time for an engine that works on other threads
// and for results without counters.
func checkMetric(r result) (metric, string, func(float64) string) {
	if !strings.HasSuffix(r.Engine, "Async") && r.Cycles.Head > 0 {
		return r.Cycles, "cycles", fmtCount
	}
	return r.Ns, "time", fmtNs
}

// fmtCount writes a count with a unit prefix.
func fmtCount(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.2fG", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.1fk", v/1e3)
	}
	return fmt.Sprintf("%.0f", v)
}

// renderCheck writes the check's title and Markdown summary: the geomean
// per engine, then per object the operations with base, head and delta for
// every engine. A delta beyond the operation's noise floor is marked.
func renderCheck(j *job, results []result, nf noiseFloor, link, state string) (string, string) {
	var own []result
	for _, r := range results {
		if !r.Baseline {
			own = append(own, r)
		}
	}
	sort.SliceStable(own, func(a, b int) bool {
		return leafLess(own[a].Object, own[a].Op, own[a].Engine, own[b].Object, own[b].Op, own[b].Engine)
	})
	floor := func(r result) float64 { return max(0.5, nf.PerLeaf[joinKey(r.Engine, r.Object, r.Op)]) }
	beyond := func(r result) bool {
		m, _, _ := checkMetric(r)
		return m.changed(floor(r))
	}
	pct := func(v float64) string { return fmt.Sprintf("%+.2f%%", v) }

	// Per engine: geomean of head/base and the count of marked operations.
	type engineSum struct {
		logs           []float64
		slower, faster int
	}
	engines := map[string]*engineSum{}
	for _, r := range own {
		m, _, _ := checkMetric(r)
		if m.Base <= 0 || m.Head <= 0 {
			continue
		}
		e := engines[r.Engine]
		if e == nil {
			e = &engineSum{}
			engines[r.Engine] = e
		}
		ratio := 1 + m.PMed/100
		if m.PN == 0 || ratio <= 0 {
			ratio = m.Head / m.Base
		}
		e.logs = append(e.logs, math.Log(ratio))
		if beyond(r) {
			if m.PMed > 0 {
				e.slower++
			} else {
				e.faster++
			}
		}
	}
	names := make([]string, 0, len(engines))
	for name := range engines {
		names = append(names, name)
	}
	sort.Slice(names, func(a, b int) bool { return rank(engineOrder, names[a]) < rank(engineOrder, names[b]) })
	var titleParts []string
	var sb strings.Builder
	fmt.Fprintf(&sb, "`%s` against `%s` (%s) · %s\n\n", shortSHA(j.HeadSHA), shortSHA(j.BaseSHA), j.BaseRef, state)
	sb.WriteString("| Engine | geomean Δ | slower | faster | operations |\n|---|---:|---:|---:|---:|\n")
	for _, name := range names {
		e := engines[name]
		gm := (math.Exp(mean(e.logs)) - 1) * 100
		fmt.Fprintf(&sb, "| %s | %s | %d | %d | %d |\n", name, pct(gm), e.slower, e.faster, len(e.logs))
		if !strings.HasSuffix(name, "Async") {
			titleParts = append(titleParts, name+" "+pct(gm))
		}
	}
	sb.WriteString("\nΔ is head against base in cycles of the measured thread (wall time for the async engines): the median over the passes, each of which links both sides with another function layout. ")
	sb.WriteString("**Bold** marks a change: the median lies beyond the operation's noise floor and beyond what the layouts alone did to this pair, and at least three quarters of the passes agree. \"slower\" and \"faster\" count those.\n")

	// Per object: one row per operation, one column pair per engine.
	kinds := false
	byObject := map[string][]result{}
	var objects []string
	for _, r := range own {
		if _, ok := byObject[r.Object]; !ok {
			objects = append(objects, r.Object)
		}
		byObject[r.Object] = append(byObject[r.Object], r)
	}
	for _, obj := range objects {
		rs := byObject[obj]
		cells := map[string]result{}
		var ops, engs []string
		seenOp, seenEng := map[string]bool{}, map[string]bool{}
		for _, r := range rs {
			cells[r.Engine+"/"+r.Op] = r
			if !seenOp[r.Op] {
				seenOp[r.Op] = true
				ops = append(ops, r.Op)
			}
			if !seenEng[r.Engine] {
				seenEng[r.Engine] = true
				engs = append(engs, r.Engine)
			}
		}
		sort.Slice(engs, func(a, b int) bool { return rank(engineOrder, engs[a]) < rank(engineOrder, engs[b]) })
		fmt.Fprintf(&sb, "\n<details><summary><b>%s</b></summary>\n\n| Operation |", obj)
		for _, e := range engs {
			fmt.Fprintf(&sb, " %s base → head | Δ |", e)
		}
		sb.WriteString("\n|---|")
		for range engs {
			sb.WriteString("---:|---:|")
		}
		sb.WriteString("\n")
		for _, op := range ops {
			fmt.Fprintf(&sb, "| %s |", op)
			for _, e := range engs {
				r, ok := cells[e+"/"+op]
				if !ok {
					sb.WriteString(" | |")
					continue
				}
				m, _, f := checkMetric(r)
				delta := pct(m.PMed)
				if beyond(r) {
					delta = "**" + delta + "**"
					if work, ok := r.workChanged(); ok && !strings.HasSuffix(r.Engine, "Async") {
						if work {
							delta += " ¹"
						} else {
							delta += " ²"
						}
						kinds = true
					}
				}
				fmt.Fprintf(&sb, " %s → %s | %s |", f(m.Base), f(m.Head), delta)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n</details>\n")
	}
	if kinds {
		fmt.Fprintf(&sb, "\n¹ the instructions per operation changed by %.1f%% or more: the code does different work. ² same instructions: the same work executes differently (code layout, cache or branch behaviour).\n", instrMoved)
	}
	if link != "" {
		fmt.Fprintf(&sb, "\n[Charts, every sample and the other SSZ libraries](%s)", link)
		if j.PR != 0 {
			if i := strings.Index(link, "#/job/"); i >= 0 {
				fmt.Fprintf(&sb, " · [every measured commit of this pull request](%s#/pr/%d)", link[:i], j.PR)
			}
		}
		sb.WriteString("\n")
	}
	title := strings.Join(titleParts, " · ")
	if title == "" {
		title = "No results"
	}
	return title + " (" + state + ")", truncate(sb.String(), 60000)
}
