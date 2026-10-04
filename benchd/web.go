package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed ui
var uiFiles embed.FS

// webServer serves the single-page UI and the JSON API it reads.
type webServer struct {
	cfg     *config
	db      *store
	sched   *scheduler // git access and job templates for the admin endpoint
	runners *runnerAPI // the worker-facing API
	pages   pageCache
}

func (w *webServer) handler() (http.Handler, error) {
	ui, err := fs.Sub(uiFiles, "ui")
	if err != nil {
		return nil, err
	}
	// The page names its script and stylesheet with a version taken from
	// their content, so that a browser never runs a script of an earlier
	// deploy against the current API.
	index, _ := fs.ReadFile(ui, "index.html")
	sum := sha256.New()
	for _, name := range []string{"app.js", "app.css"} {
		data, _ := fs.ReadFile(ui, name)
		sum.Write(data)
	}
	version := hex.EncodeToString(sum.Sum(nil))[:12]
	for _, name := range []string{"/ui/app.js", "/ui/app.css"} {
		index = bytes.ReplaceAll(index, []byte(`"`+name+`"`), []byte(`"`+name+`?v=`+version+`"`))
	}
	files := http.StripPrefix("/ui/", http.FileServer(http.FS(ui)))
	mux := http.NewServeMux()
	mux.HandleFunc("/ui/", func(rw http.ResponseWriter, req *http.Request) {
		// A versioned file never changes; anything else is checked again.
		if req.URL.Query().Get("v") != "" {
			rw.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			rw.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(rw, req)
	})
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/" {
			http.NotFound(rw, req)
			return
		}
		rw.Header().Set("Content-Type", "text/html; charset=utf-8")
		rw.Header().Set("Cache-Control", "no-store")
		_, _ = rw.Write(index)
	})
	mux.HandleFunc("/raw/", w.handleRaw)
	mux.HandleFunc("/api/status", w.apiStatus)
	mux.HandleFunc("/api/dashboard", w.apiDashboard)
	mux.HandleFunc("/api/jobs", w.apiJobs)
	mux.HandleFunc("/api/job/", w.apiJob)
	mux.HandleFunc("/api/ops", w.apiOps)
	mux.HandleFunc("/api/commit/", w.apiCommit)
	mux.HandleFunc("/api/op/", w.apiOp)
	mux.HandleFunc("/api/compare", w.apiCompare)
	mux.HandleFunc("/api/pr/", w.apiPR)
	mux.HandleFunc("/webhook", w.webhook)
	mux.HandleFunc("/api/noise", w.apiNoise)
	mux.HandleFunc("/admin/queue", w.adminQueue)
	mux.HandleFunc("/admin/harness", w.adminHarness)
	mux.HandleFunc("/api/runners", w.apiRunners)
	if w.runners != nil {
		w.runners.register(mux)
	}
	return mux, nil
}

func (w *webServer) serve(ctx context.Context) error {
	h, err := w.handler()
	if err != nil {
		return err
	}
	srv := &http.Server{Addr: w.cfg.listen, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	err = srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

func fmtNs(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.3f s", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.2f ms", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.2f µs", v/1e3)
	default:
		return fmt.Sprintf("%.1f ns", v)
	}
}

func shortSHA(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// statusInfo is the live state of the service and its runners.
type statusInfo struct {
	Runners    []runnerLive // the controller's own runner first
	Active     int          // runners with a job
	Queued     int
	LastFetch  time.Time
	FetchErr   string
	Done       int
	Failed     int
	Host       string
	Load       string
	Uptime     string
	Now        time.Time
	DaemonSeen time.Time
	DaemonDown bool
}

// runnerLive is one machine's published state as the UI shows it.
type runnerLive struct {
	Runner   string
	Current  *job
	Phase    string
	Progress progress
	Percent  int
	Running  float64
	Seen     time.Time
	Down     bool // nothing published for two minutes
	Host     string
	State    string // qualification state; "controller" for the controller
	Note     string
}

// status reads what every runner last published. A record older than two
// minutes means that runner is not running.
func (w *webServer) status() statusInfo {
	st := statusInfo{Now: time.Now(), Runners: []runnerLive{}}
	statuses, _ := w.db.runnerStatuses()
	infos, _ := w.db.listRunners()
	byName := map[string]*runnerInfo{}
	for _, r := range infos {
		byName[r.Name] = r
	}
	for _, ls := range statuses {
		rl := runnerLive{Runner: ls.Runner, Phase: ls.Phase, Progress: ls.Progress, Seen: ls.Updated, Host: ls.Host, State: "controller"}
		if ri := byName[ls.Runner]; ri != nil {
			rl.State, rl.Note = ri.State, ri.Note
		}
		if time.Since(ls.Updated) > 2*time.Minute {
			rl.Down = true
		}
		if ls.JobID != 0 && !rl.Down {
			rl.Current, _ = w.db.getJob(ls.JobID)
			if rl.Current != nil && rl.Current.State != stateRunning {
				rl.Current = nil
			}
		}
		if rl.Current != nil && !ls.Started.IsZero() {
			rl.Running = time.Since(ls.Started).Seconds()
		}
		if p := ls.Progress; p.Leaves > 0 {
			// Until the first pass has been timed the plan is the minimum:
			// one pass per layout seed.
			if p.PlannedPasses == 0 {
				p.PlannedPasses = len(w.cfg.seeds)
				rl.Progress.PlannedPasses = p.PlannedPasses
			}
			done := float64(p.Passes) + float64(p.Leaf-1)/float64(p.Leaves)
			rl.Percent = min(99, int(done/float64(p.PlannedPasses)*100))
		}
		if ls.Runner == w.cfg.name {
			st.LastFetch, st.FetchErr = ls.LastFetch, ls.FetchErr
			st.Host = ls.Host
			st.DaemonSeen = ls.Updated
			st.DaemonDown = rl.Down
			st.Runners = append([]runnerLive{rl}, st.Runners...)
			continue
		}
		st.Runners = append(st.Runners, rl)
	}
	if st.DaemonSeen.IsZero() {
		st.DaemonDown = true
	}
	for _, rl := range st.Runners {
		if rl.Current != nil && !rl.Down {
			st.Active++
		}
	}
	st.Queued, _ = w.db.queuedCount()
	st.Done, _ = w.db.countJobs(kindCommit, stateDone)
	st.Failed, _ = w.db.countJobs(kindCommit, stateFailed)
	if st.Host == "" {
		st.Host, _ = os.Hostname()
	}
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		if f := strings.Fields(string(data)); len(f) >= 3 {
			st.Load = strings.Join(f[:3], " ")
		}
	}
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		if f := strings.Fields(string(data)); len(f) > 0 {
			if secs, err := strconv.ParseFloat(f[0], 64); err == nil {
				st.Uptime = (time.Duration(secs) * time.Second).Round(time.Minute).String()
			}
		}
	}
	return st
}

// engineSummary is the geomean of time ratios of one engine on one object.
type engineSummary struct {
	Engine, Object string
	Geomean        float64 // of head/base time
	CyclesGeomean  float64 // of head/base cycles; zero without counters
	N              int
}

func summaries(results []result) []engineSummary {
	type key struct{ e, o string }
	groups := map[key][]result{}
	var keys []key
	for _, r := range results {
		if r.Baseline {
			continue
		}
		k := key{r.Engine, r.Object}
		if _, ok := groups[k]; !ok {
			keys = append(keys, k)
		}
		groups[k] = append(groups[k], r)
	}
	sort.Slice(keys, func(i, j int) bool { return leafLess(keys[i].o, "", keys[i].e, keys[j].o, "", keys[j].e) })
	out := []engineSummary{}
	for _, k := range keys {
		out = append(out, engineSummary{Engine: k.e, Object: k.o, Geomean: geomeanDelta(groups[k]), CyclesGeomean: geomeanCycles(groups[k]), N: len(groups[k])})
	}
	return out
}

func (w *webServer) writeJSON(rw http.ResponseWriter, v any) {
	rw.Header().Set("Content-Type", "application/json")
	rw.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(rw).Encode(v)
}

func (w *webServer) jobFromPath(req *http.Request, prefix string) (*job, string, error) {
	idStr, tail, _ := strings.Cut(strings.TrimPrefix(req.URL.Path, prefix), "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return nil, "", err
	}
	j, err := w.db.getJob(id)
	return j, tail, err
}

func (w *webServer) handleRaw(rw http.ResponseWriter, req *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(req.URL.Path, "/raw/"), "/", 2)
	if len(parts) != 2 || strings.Contains(parts[1], "/") || strings.Contains(parts[1], "..") {
		http.NotFound(rw, req)
		return
	}
	if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
		http.NotFound(rw, req)
		return
	}
	rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
	serveJobFile(rw, req, filepath.Join(w.cfg.dataDir, "jobs", parts[0], parts[1]))
}

func (w *webServer) apiStatus(rw http.ResponseWriter, req *http.Request) {
	st := w.status()
	w.writeJSON(rw, struct {
		statusInfo
		GoVersion       string
		Cpus            string
		Gomaxprocs      string
		AsyncGomaxprocs string
		Seeds           []string
		BenchTime       string
		MinIters        int
		Target          string
		Godebug         string
		Repo            string
		MainBranch      string
		Controller      string
		// Subjects maps every measured library to its repository.
		Subjects map[string]string
	}{st, runtime.Version(), w.cfg.cpus, w.cfg.gomaxprocs, w.cfg.asyncGomaxprocs, w.cfg.seeds, w.cfg.benchTime, w.cfg.minIters, w.cfg.target.String(), w.cfg.godebug, "https://github.com/" + w.cfg.github, w.cfg.mainBranch, w.cfg.name, w.subjectRepos()})
}

func (w *webServer) subjectRepos() map[string]string {
	repos := map[string]string{}
	for _, name := range subjectNames() {
		repos[name] = repoURL(name)
	}
	return repos
}

// apiDashboard: status, the queue in execution order, recently finished
// jobs with their per-engine summaries, and the noise floor.
func (w *webServer) apiDashboard(rw http.ResponseWriter, req *http.Request) {
	queue, _ := w.db.queuedJobs()
	finished, _ := w.db.finishedJobs(30)
	type jobRow struct {
		Job       *job
		Summaries []engineSummary
	}
	rows := []jobRow{}
	for _, j := range finished {
		row := jobRow{Job: j}
		if j.State == stateDone {
			results, _ := w.db.resultsFor(j.ID)
			row.Summaries = summaries(results)
		}
		rows = append(rows, row)
	}
	w.writeJSON(rw, struct {
		Status   statusInfo
		Queue    []*job
		Finished []jobRow
		Noise    noiseFloor
		Repo     string
	}{w.status(), queue, rows, w.noiseFloor(), "https://github.com/" + w.cfg.github})
}

// queueStamp changes when a job is queued, started or removed.
func (w *webServer) queueStamp() int {
	var n, last, running int
	_ = w.db.db.QueryRow(`SELECT count(*), coalesce(max(id), 0), coalesce(sum(state = ?), 0) FROM jobs`, stateRunning).Scan(&n, &last, &running)
	return n*31 + last*7 + running
}

func (w *webServer) apiJobs(rw http.ResponseWriter, req *http.Request) {
	limit := 300
	if n, err := strconv.Atoi(req.URL.Query().Get("limit")); err == nil && n > 0 {
		limit = n
	}
	kind := req.URL.Query().Get("kind")
	if req.URL.Query().Get("summaries") == "" {
		jobs, _ := w.db.listJobs(limit, kind)
		if jobs == nil {
			jobs = []*job{}
		}
		w.writeJSON(rw, jobs)
		return
	}
	// With the ratio per engine of every finished job, as on the dashboard.
	rows := w.pages.get(w.db.doneStamp()+"|"+strconv.Itoa(w.queueStamp()), fmt.Sprintf("jobs|%d|%s", limit, kind), func() any {
		type jobRow struct {
			Job       *job
			Summaries []engineSummary
		}
		jobs, _ := w.db.listJobs(limit, kind)
		rows := []jobRow{}
		for _, j := range jobs {
			row := jobRow{Job: j}
			if j.State == stateDone && j.BaseSHA != "" {
				results, _ := w.db.resultsFor(j.ID)
				row.Summaries = summaries(results)
			}
			rows = append(rows, row)
		}
		return rows
	})
	w.writeJSON(rw, rows)
}

// apiJob: /api/job/<id> (results; provisional results for a running job),
// /api/job/<id>/samples and /api/job/<id>/leaf/<engine>/<object>/<op>
// (every sample of one leaf pooled over the runs of the pair).
func (w *webServer) apiJob(rw http.ResponseWriter, req *http.Request) {
	j, tail, err := w.jobFromPath(req, "/api/job/")
	if err != nil || j == nil {
		http.NotFound(rw, req)
		return
	}
	switch {
	case tail == "":
		st := w.status()
		var live *runnerLive
		for i := range st.Runners {
			if st.Runners[i].Current != nil && st.Runners[i].Current.ID == j.ID {
				live = &st.Runners[i]
			}
		}
		var results []result
		if j.State == stateRunning {
			samples, _ := w.db.samplesFor(j.ID)
			results = summarize(samples)
		} else {
			// A job's page shows what the job measured; the runs of all
			// jobs of a commit are on the commit's page.
			samples, _ := w.db.samplesFor(j.ID)
			if len(samples) > 0 {
				results = summarize(samples)
			} else {
				results, _ = w.db.resultsFor(j.ID)
			}
		}
		if results == nil {
			results = []result{}
		}
		builds, _ := w.db.buildsFor(j.ID)
		files := jobFiles(filepath.Join(w.cfg.dataDir, "jobs", strconv.FormatInt(j.ID, 10)))
		steal := 0
		for _, r := range results {
			steal += r.Steal
		}
		runs := []int64{}

		// The pooled values of the other libraries stand next to every
		// job's own results.
		if j.Kind != kindBaseline {
			refs := w.otherResults(j.Subject)
			have := map[string]bool{}
			for _, r := range results {
				have[joinKey(r.Engine, r.Object, r.Op)] = true
			}
			for _, r := range refs {
				if !have[joinKey(r.Engine, r.Object, r.Op)] {
					results = append(results, r)
				}
			}
		}
		w.writeJSON(rw, struct {
			Job       *job
			Live      bool
			Runner    *runnerLive
			Status    statusInfo
			Results   []result
			Summaries []engineSummary
			Files     []string
			Steal     int
			Builds    []buildFact
			Runs      []int64
			Noise     noiseFloor
			Repo      string
			Subject   string
		}{j, j.State == stateRunning, live, st, results, summaries(results), files, steal, builds, runs, w.noiseFloor(), "https://github.com/" + w.cfg.github, j.Subject})
	case tail == "samples":
		samples, _ := w.db.samplesFor(j.ID)
		w.writeJSON(rw, samples)
	case strings.HasPrefix(tail, "leaf/"):
		parts := strings.SplitN(strings.TrimPrefix(tail, "leaf/"), "/", 3)
		if len(parts) != 3 {
			http.NotFound(rw, req)
			return
		}
		ids := []int64{j.ID}
		samples := []sample{}
		for _, id := range ids {
			all, _ := w.db.samplesFor(id)
			for _, sm := range all {
				if sm.Engine == parts[0] && sm.Object == parts[1] && sm.Op == parts[2] {
					samples = append(samples, sm)
				}
			}
		}
		var res *result
		if rs := summarize(samples); len(rs) > 0 {
			res = &rs[0]
		}
		w.writeJSON(rw, struct {
			Job     *job
			Runs    []int64
			Samples []sample
			Result  *result
		}{j, ids, samples, res})
	default:
		http.NotFound(rw, req)
	}
}

// webhook receives the GitHub App's deliveries. Only one thing is acted
// on: the approval label being applied to a pull request, which approves
// measuring the head the pull request has at that moment (the event
// carries it, so a later push is not covered). A delivery must be signed
// with the webhook secret.
func (w *webServer) webhook(rw http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(rw, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if w.cfg.webhookSecret == "" {
		http.Error(rw, "webhooks are not configured", http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, 8<<20))
	if err != nil {
		http.Error(rw, "read error", http.StatusBadRequest)
		return
	}
	mac := hmac.New(sha256.New, []byte(w.cfg.webhookSecret))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(want), []byte(req.Header.Get("X-Hub-Signature-256"))) != 1 {
		http.Error(rw, "bad signature", http.StatusUnauthorized)
		return
	}
	if req.Header.Get("X-GitHub-Event") != "pull_request" {
		_, _ = io.WriteString(rw, "ignored\n")
		return
	}
	var ev struct {
		Action string `json:"action"`
		Label  struct {
			Name string `json:"name"`
		} `json:"label"`
		PullRequest struct {
			Number int `json:"number"`
			Head   struct {
				SHA string `json:"sha"`
			} `json:"head"`
		} `json:"pull_request"`
		Repository struct {
			FullName string `json:"full_name"`
		} `json:"repository"`
		Sender struct {
			Login string `json:"login"`
		} `json:"sender"`
	}
	if err := json.Unmarshal(body, &ev); err != nil {
		http.Error(rw, "bad payload", http.StatusBadRequest)
		return
	}
	if ev.Action != "labeled" || ev.Label.Name != w.cfg.approveLabel || !strings.EqualFold(ev.Repository.FullName, w.cfg.github) || len(ev.PullRequest.Head.SHA) != 40 {
		_, _ = io.WriteString(rw, "ignored\n")
		return
	}
	if err := w.db.approvePR(ev.PullRequest.Number, ev.PullRequest.Head.SHA, ev.Sender.Login); err != nil {
		http.Error(rw, "store error", http.StatusInternalServerError)
		return
	}
	log.Printf("pull request #%d: measuring %s approved by %s (label %s)", ev.PullRequest.Number, shortSHA(ev.PullRequest.Head.SHA), ev.Sender.Login, ev.Label.Name)
	_, _ = io.WriteString(rw, "approved\n")
}

// apiPR is a pull request as the benchmarks see it: every commit of its
// branch, oldest first, with the results of the commits that were measured
// (against the base, and against the measured commit before), and the
// heads measured earlier that a force push took out of the branch.
func (w *webServer) apiPR(rw http.ResponseWriter, req *http.Request) {
	n, err := strconv.Atoi(strings.TrimPrefix(req.URL.Path, "/api/pr/"))
	if err != nil {
		http.NotFound(rw, req)
		return
	}
	jobs, err := w.db.prJobs(n)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	type row struct {
		SHA       string
		Subject   string
		Time      *time.Time
		Job       *job            // newest job with this commit as head, if any
		Summaries []engineSummary // head against the base
		Step      []engineSummary // head against the previous measured commit
		Previous  int64           // job of that previous commit
		Results   []result        // the measured values, for the charts
		Builds    []buildFact     // what the head and the base were built into
		Detached  bool            // measured, but no longer part of the branch
	}
	// The newest job of every head.
	byHead := map[string]*job{}
	var headOrder []string
	for _, j := range jobs {
		if _, ok := byHead[j.HeadSHA]; !ok {
			headOrder = append(headOrder, j.HeadSHA)
		}
		byHead[j.HeadSHA] = j
	}
	rows := []row{}
	branch, title := "", ""
	if len(jobs) > 0 {
		last := jobs[len(jobs)-1]
		branch = last.Branch
		inBranch := map[string]bool{}
		for _, c := range prCommits(w.cfg.dataDir, last.BaseSHA, last.HeadSHA) {
			c := c
			inBranch[c.sha] = true
			rows = append(rows, row{SHA: c.sha, Subject: c.subject, Time: &c.when, Job: byHead[c.sha]})
		}
		// Heads a force push removed come first: they were measured earlier.
		var detached []row
		for _, sha := range headOrder {
			if !inBranch[sha] {
				j := byHead[sha]
				detached = append(detached, row{SHA: sha, Subject: strings.TrimPrefix(j.HeadDesc, shortSHA(sha)[:min(7, len(sha))]+" "), Job: j, Detached: len(inBranch) > 0})
			}
		}
		rows = append(detached, rows...)
	}
	var prev []result
	var prevJob int64
	for i := range rows {
		r := &rows[i]
		r.Summaries, r.Step, r.Results, r.Builds = []engineSummary{}, []engineSummary{}, []result{}, []buildFact{}
		if r.Job == nil || r.Job.State != stateDone {
			continue
		}
		r.Builds, _ = w.db.buildsFor(r.Job.ID)
		results, _ := w.db.resultsFor(r.Job.ID)
		for _, res := range results {
			if !res.Baseline {
				r.Results = append(r.Results, res)
			}
		}
		r.Summaries = summaries(r.Results)
		if prev != nil {
			r.Step, r.Previous = summaries(stepResults(prev, r.Results)), prevJob
		}
		prev, prevJob = r.Results, r.Job.ID
	}
	w.writeJSON(rw, struct {
		PR     int
		Branch string
		Title  string
		Rows   []row
		Repo   string
	}{n, branch, title, rows, "https://github.com/" + w.cfg.github})
}

type prCommit struct {
	sha, subject string
	when         time.Time
}

// prCommits lists the commits between base and head from the mirror,
// oldest first; nothing when the mirror no longer has them.
func prCommits(dataDir, base, head string) []prCommit {
	if len(base) != 40 || len(head) != 40 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", filepath.Join(dataDir, "repo.git"), "log", "--reverse", "--first-parent", "--format=%H%x09%ct%x09%s", "-n", "500", base+".."+head).Output()
	if err != nil {
		return nil
	}
	var commits []prCommit
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.SplitN(line, "\t", 3)
		if len(f) != 3 || len(f[0]) != 40 {
			continue
		}
		ts, _ := strconv.ParseInt(f[1], 10, 64)
		commits = append(commits, prCommit{sha: f[0], subject: f[2], when: time.Unix(ts, 0)})
	}
	return commits
}

// stepResults pairs the head values of two jobs of the same branch: the
// earlier head takes the place of the base. The two were measured in
// separate jobs, so this is less exact than a job's own comparison.
func stepResults(prev, cur []result) []result {
	before := map[string]result{}
	for _, r := range prev {
		before[joinKey(r.Engine, r.Object, r.Op)] = r
	}
	pair := func(a, b metric) metric {
		m := metric{Base: a.Head, Head: b.Head}
		if a.Head != 0 {
			m.Delta = (b.Head - a.Head) / a.Head * 100
		}
		return m
	}
	var out []result
	for _, r := range cur {
		p, ok := before[joinKey(r.Engine, r.Object, r.Op)]
		if !ok {
			continue
		}
		out = append(out, result{Engine: r.Engine, Object: r.Object, Op: r.Op, N: r.N,
			Ns: pair(p.Ns, r.Ns), Bytes: pair(p.Bytes, r.Bytes), Allocs: pair(p.Allocs, r.Allocs), Cycles: pair(p.Cycles, r.Cycles), Instrs: pair(p.Instrs, r.Instrs)})
	}
	return out
}

// opView is the history of one object/op across engines.
type opView struct {
	Object, Op string
	Engines    []string
	Latest     map[string]*result
	Trends     map[string]trendFit
	History    []opHistoryRow
	Noise      map[string]float64
}

type opHistoryRow struct {
	Job     *job
	Results map[string]*result
}

func (w *webServer) opView(object, op string, limit int, refs []result) (opView, bool) {
	results, jobs, err := w.db.leafHistory(object, op, []string{kindCommit, kindRelease, kindNoise}, limit)
	if err != nil || len(results) == 0 {
		return opView{}, false
	}
	v := opView{Object: object, Op: op, Latest: map[string]*result{}, Trends: map[string]trendFit{}, Noise: map[string]float64{}}
	engines := map[string]bool{}
	byJob := map[int64]*opHistoryRow{}
	var order []int64
	for i := range results {
		r := &results[i]
		engines[r.Engine] = true
		if _, ok := v.Latest[r.Engine]; !ok {
			v.Latest[r.Engine] = r
		}
		row, ok := byJob[r.JobID]
		if !ok {
			row = &opHistoryRow{Job: jobs[r.JobID], Results: map[string]*result{}}
			byJob[r.JobID] = row
			order = append(order, r.JobID)
		}
		row.Results[r.Engine] = r
	}
	// The reference libraries' latest values do not depend on how far back
	// the history reaches.
	if len(refs) > 0 {
		for i := range refs {
			r := &refs[i]
			if r.Object == object && r.Op == op && v.Latest[r.Engine] == nil {
				v.Latest[r.Engine] = r
				engines[r.Engine] = true
			}
		}
	}
	v.Engines = sortedBy(engines, engineOrder)
	for _, id := range order {
		v.History = append(v.History, *byJob[id])
	}
	for _, e := range v.Engines {
		var ts []time.Time
		var vals []float64
		for i := len(v.History) - 1; i >= 0; i-- {
			row := v.History[i]
			if row.Job.Branch != w.cfg.mainBranch || row.Job.Finished == nil || row.Job.Kind == kindNoise {
				continue
			}
			if r, ok := row.Results[e]; ok {
				ts = append(ts, *row.Job.Finished)
				vals = append(vals, r.Ns.Head)
			}
		}
		v.Trends[e] = fitTrend(ts, vals)
	}
	nf := w.noiseFloor()
	for _, e := range v.Engines {
		v.Noise[e] = nf.PerLeaf[joinKey(e, object, op)]
	}
	return v, true
}

func sortedBy(set map[string]bool, order []string) []string {
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool {
		if a, b := rank(order, out[i]), rank(order, out[j]); a != b {
			return a < b
		}
		return out[i] < out[j]
	})
	return out
}

func (w *webServer) apiOp(rw http.ResponseWriter, req *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(req.URL.Path, "/api/op/"), "/", 2)
	if len(parts) != 2 {
		http.NotFound(rw, req)
		return
	}
	v, ok := w.opView(parts[0], parts[1], 2000, w.otherResults(subjectDynSSZ))
	if !ok {
		http.NotFound(rw, req)
		return
	}
	w.writeJSON(rw, v)
}

// commitValues collects the most recent measured values of a commit per
// leaf, from the newest finished job that had the commit on either side.
func (w *webServer) commitValues(sha string) (map[string]metricSet, *job, string, error) {
	jobs, err := w.db.jobsWithCommit(sha)
	if err != nil || len(jobs) == 0 {
		return nil, nil, "", err
	}
	j := jobs[0]
	side := "head"
	if j.HeadSHA != sha {
		side = "base"
	}
	results, err := w.db.resultsFor(j.ID)
	if err != nil {
		return nil, nil, "", err
	}
	out := map[string]metricSet{}
	for _, r := range results {
		if r.Baseline {
			continue
		}
		ms := metricSet{Ns: r.Ns.Head, Bytes: r.Bytes.Head, Allocs: r.Allocs.Head, Cycles: r.Cycles.Head, Instrs: r.Instrs.Head}
		if side == "base" {
			ms = metricSet{Ns: r.Ns.Base, Bytes: r.Bytes.Base, Allocs: r.Allocs.Base, Cycles: r.Cycles.Base, Instrs: r.Instrs.Base}
		}
		out[joinKey(r.Engine, r.Object, r.Op)] = ms
	}
	return out, j, side, nil
}

type metricSet struct {
	Ns, Bytes, Allocs, Cycles, Instrs float64
}

type compareRow struct {
	Engine, Object, Op string
	A, B               metricSet
	NsDelta            float64
	BytesDelta         float64
	AllocsDelta        float64
	CyclesDelta        float64
	InstrsDelta        float64
}

func (w *webServer) resolveRef(ref string) string {
	if ref == "" {
		return ""
	}
	if sha, err := w.sched.git.revParse(ref); err == nil {
		return sha
	}
	return ref
}

func (w *webServer) apiCompare(rw http.ResponseWriter, req *http.Request) {
	a := w.resolveRef(req.URL.Query().Get("a"))
	b := w.resolveRef(req.URL.Query().Get("b"))
	out := struct {
		A, B         string
		AJob, BJob   *job
		ASide, BSide string
		Rows         []compareRow
		Jobs         []*job
	}{A: a, B: b, Rows: []compareRow{}}
	out.Jobs, _ = w.db.finishedJobs(100)
	if out.Jobs == nil {
		out.Jobs = []*job{}
	}
	if a != "" && b != "" {
		aVals, aJob, aSide, _ := w.commitValues(a)
		bVals, bJob, bSide, _ := w.commitValues(b)
		out.AJob, out.BJob, out.ASide, out.BSide = aJob, bJob, aSide, bSide
		for k, av := range aVals {
			bv, ok := bVals[k]
			if !ok {
				continue
			}
			parts := strings.SplitN(k, "/", 3)
			row := compareRow{Engine: parts[0], Object: parts[1], Op: parts[2], A: av, B: bv}
			if av.Ns > 0 {
				row.NsDelta = (bv.Ns - av.Ns) / av.Ns * 100
			}
			if av.Bytes > 0 {
				row.BytesDelta = (bv.Bytes - av.Bytes) / av.Bytes * 100
			}
			if av.Allocs > 0 {
				row.AllocsDelta = (bv.Allocs - av.Allocs) / av.Allocs * 100
			}
			if av.Cycles > 0 {
				row.CyclesDelta = (bv.Cycles - av.Cycles) / av.Cycles * 100
			}
			if av.Instrs > 0 {
				row.InstrsDelta = (bv.Instrs - av.Instrs) / av.Instrs * 100
			}
			out.Rows = append(out.Rows, row)
		}
		sort.Slice(out.Rows, func(i, j int) bool {
			return leafLess(out.Rows[i].Object, out.Rows[i].Op, out.Rows[i].Engine, out.Rows[j].Object, out.Rows[j].Op, out.Rows[j].Engine)
		})
	}
	w.writeJSON(rw, out)
}

// noiseFloor summarizes the self-comparison jobs: how far two measurements
// of the same commit drift, overall and per leaf and metric. A delta below
// the floor is not evidence of a change.
type noiseFloor struct {
	Jobs      int
	MedianAbs float64 // median |delta| of ns/op over every leaf and noise job
	P95Abs    float64
	PerLeaf   map[string]float64 // engine/object/op -> p95 |delta| of ns/op, controller machine
	Rows      []noiseRow
	Steal     int
	JobIDs    []int64
	PerRunner map[string]noiseStat // runner -> overall |delta time| statistics
}

type noiseRow struct {
	Runner                            string
	Engine, Object, Op                string
	N                                 int
	Ns, Bytes, Allocs, Cycles, Instrs noiseStat
	Steal                             int
}

type noiseStat struct {
	MedianAbs, P95Abs, MaxAbs, CV float64
}

func (w *webServer) noiseFloor() noiseFloor { return w.db.noiseFloor(w.cfg.name) }

// noiseFloorOf computes the noise floor from the finished noise jobs;
// local names the controller machine, whose jobs feed the overall figures.
func noiseFloorOf(db *store, local string) noiseFloor {
	nf := noiseFloor{PerLeaf: map[string]float64{}, Rows: []noiseRow{}, JobIDs: []int64{}, PerRunner: map[string]noiseStat{}}
	sets, err := noiseSets(db)
	if err != nil {
		return nf
	}
	var all []float64
	byRunner := map[string][]float64{}
	type acc struct {
		runner                                   string
		ns, b, a, c, i, cvNs, cvB, cvA, cvC, cvI []float64
		steal                                    int
	}
	per := map[string]*acc{}
	for _, j := range sets {
		nf.Jobs++
		nf.JobIDs = append(nf.JobIDs, j.ID)
		for _, r := range j.results {
			if r.Baseline {
				continue
			}
			k := j.Runner + "|" + joinKey(r.Engine, r.Object, r.Op)
			g := per[k]
			if g == nil {
				g = &acc{runner: j.Runner}
				per[k] = g
			}
			if j.Runner == local {
				all = append(all, math.Abs(r.Ns.Delta))
			}
			byRunner[j.Runner] = append(byRunner[j.Runner], math.Abs(r.Ns.Delta))
			g.ns = append(g.ns, math.Abs(r.Ns.Delta))
			g.b = append(g.b, math.Abs(r.Bytes.Delta))
			g.a = append(g.a, math.Abs(r.Allocs.Delta))
			g.cvNs = append(g.cvNs, r.Ns.CVBase, r.Ns.CVHead)
			g.cvB = append(g.cvB, r.Bytes.CVBase, r.Bytes.CVHead)
			g.cvA = append(g.cvA, r.Allocs.CVBase, r.Allocs.CVHead)
			g.c = append(g.c, math.Abs(r.Cycles.Delta))
			g.i = append(g.i, math.Abs(r.Instrs.Delta))
			g.cvC = append(g.cvC, r.Cycles.CVBase, r.Cycles.CVHead)
			g.cvI = append(g.cvI, r.Instrs.CVBase, r.Instrs.CVHead)
			g.steal += r.Steal
			nf.Steal += r.Steal
		}
	}
	nf.MedianAbs = percentile(all, 0.5)
	nf.P95Abs = percentile(all, 0.95)
	stat := func(d, cv []float64) noiseStat {
		return noiseStat{MedianAbs: percentile(d, 0.5), P95Abs: percentile(d, 0.95), MaxAbs: percentile(d, 1), CV: median(cv)}
	}
	for name, ds := range byRunner {
		nf.PerRunner[name] = noiseStat{MedianAbs: percentile(ds, 0.5), P95Abs: percentile(ds, 0.95), MaxAbs: percentile(ds, 1)}
	}
	for k, g := range per {
		_, leafKey, _ := strings.Cut(k, "|")
		if g.runner == local {
			nf.PerLeaf[leafKey] = percentile(g.ns, 0.95)
		}
		parts := strings.SplitN(leafKey, "/", 3)
		nf.Rows = append(nf.Rows, noiseRow{Runner: g.runner, Engine: parts[0], Object: parts[1], Op: parts[2], N: len(g.ns),
			Ns: stat(g.ns, g.cvNs), Bytes: stat(g.b, g.cvB), Allocs: stat(g.a, g.cvA), Cycles: stat(g.c, g.cvC), Instrs: stat(g.i, g.cvI), Steal: g.steal})
	}
	sort.Slice(nf.Rows, func(i, j int) bool {
		if nf.Rows[i].Runner != nf.Rows[j].Runner {
			return (nf.Rows[i].Runner == local) || (nf.Rows[j].Runner != local && nf.Rows[i].Runner < nf.Rows[j].Runner)
		}
		return leafLess(nf.Rows[i].Object, nf.Rows[i].Op, nf.Rows[i].Engine, nf.Rows[j].Object, nf.Rows[j].Op, nf.Rows[j].Engine)
	})
	return nf
}

func (w *webServer) apiRunners(rw http.ResponseWriter, req *http.Request) {
	infos, _ := w.db.listRunners()
	w.writeJSON(rw, struct {
		Controller string
		Runners    []*runnerInfo
		Live       []runnerLive
	}{w.cfg.name, infos, w.status().Runners})
}

func (w *webServer) apiNoise(rw http.ResponseWriter, req *http.Request) {
	nf := w.noiseFloor()
	jobs, _ := w.db.listJobs(50, kindNoise)
	if jobs == nil {
		jobs = []*job{}
	}
	w.writeJSON(rw, struct {
		noiseFloor
		JobList []*job
	}{nf, jobs})
}

// adminQueue queues a job by hand. It answers only to the local host:
//
//	curl -X POST localhost/admin/queue -d kind=noise
//	curl -X POST localhost/admin/queue -d kind=release
//	curl -X POST localhost/admin/queue -d kind=commit -d head=<ref> -d base=<ref> [-d priority=1]
func (w *webServer) adminQueue(rw http.ResponseWriter, req *http.Request) {
	host, _, _ := net.SplitHostPort(req.RemoteAddr)
	if ip := net.ParseIP(host); req.Method != http.MethodPost || ip == nil || !ip.IsLoopback() {
		http.Error(rw, "local POST only", http.StatusForbidden)
		return
	}
	if err := req.ParseForm(); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	var j *job
	var err error
	switch kind := req.Form.Get("kind"); kind {
	case kindNoise, kindRelease:
		j, err = w.sched.idleJobOfKind(kind)
	case kindCommit:
		j, err = w.sched.manualJob(req.Form.Get("head"), req.Form.Get("base"))
	default:
		http.Error(rw, "kind must be noise, release or commit", http.StatusBadRequest)
		return
	}
	if err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	j.Priority, _ = strconv.Atoi(req.Form.Get("priority"))
	if err := w.db.insertJob(j); err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(rw, "queued job %d: %s %s vs %s (priority %d)\n", j.ID, j.Kind, shortSHA(j.HeadSHA), shortSHA(j.BaseSHA), j.Priority)
}
