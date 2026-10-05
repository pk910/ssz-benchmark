package main

import (
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeGitHub is the part of the API the reporter uses: installation tokens
// and check runs. It verifies the App's JWT and records every check.
type fakeGitHub struct {
	t      *testing.T
	key    *rsa.PublicKey
	mu     sync.Mutex
	nextID int64
	checks map[int64]checkPayload
	calls  []string
}

func (f *fakeGitHub) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(req.Body)
	f.calls = append(f.calls, req.Method+" "+req.URL.Path)
	auth := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
	switch {
	case req.Method == "POST" && req.URL.Path == "/app/installations/42/access_tokens":
		parts := strings.Split(auth, ".")
		if len(parts) != 3 {
			f.t.Errorf("not a JWT: %q", auth)
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
		if err := rsa.VerifyPKCS1v15(f.key, crypto.SHA256, sum[:], sig); err != nil {
			f.t.Errorf("JWT signature: %v", err)
		}
		claims, _ := base64.RawURLEncoding.DecodeString(parts[1])
		if !strings.Contains(string(claims), `"iss":"7"`) {
			f.t.Errorf("JWT claims %s", claims)
		}
		rw.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(rw).Encode(map[string]any{"token": "inst-token", "expires_at": time.Now().Add(time.Hour)})
	case auth != "inst-token":
		http.Error(rw, "bad token", http.StatusUnauthorized)
	case req.Method == "POST" && req.URL.Path == "/repos/o/r/check-runs":
		var p checkPayload
		_ = json.Unmarshal(body, &p)
		f.nextID++
		f.checks[f.nextID] = p
		rw.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(rw).Encode(map[string]any{"id": f.nextID})
	case req.Method == "PATCH" && strings.HasPrefix(req.URL.Path, "/repos/o/r/check-runs/"):
		var p checkPayload
		_ = json.Unmarshal(body, &p)
		var id int64
		for k := range f.checks {
			if strings.HasSuffix(req.URL.Path, "/"+itoa(k)) {
				id = k
			}
		}
		if id == 0 {
			http.NotFound(rw, req)
			return
		}
		p.HeadSHA = f.checks[id].HeadSHA
		f.checks[id] = p
		_ = json.NewEncoder(rw).Encode(map[string]any{"id": id})
	default:
		http.NotFound(rw, req)
	}
}

func itoa(v int64) string { b, _ := json.Marshal(v); return string(b) }

func TestCheckReporter(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeGitHub{t: t, key: &key.PublicKey, checks: map[int64]checkPayload{}}
	srv := httptest.NewServer(fake)
	defer srv.Close()

	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	// A job from before the reporter existed gets no check.
	old := &job{Kind: kindCommit, Branch: "feature", HeadSHA: strings.Repeat("0", 40), BaseSHA: strings.Repeat("b", 40), BaseRef: "master"}
	if err := db.insertJob(old); err != nil {
		t.Fatal(err)
	}
	app := &ghApp{appID: "7", installationID: "42", key: key, api: srv.URL, client: srv.Client()}
	cfg := &config{github: "o/r", name: "ctl", publicURL: "https://bench.example"}
	c, err := newCheckReporter(app, cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	c.sync(ctx, old)
	if len(fake.checks) != 0 {
		t.Fatalf("check for a job older than the reporter: %+v", fake.checks)
	}

	// Two heads of a branch: the first is queued, then superseded.
	first := &job{Kind: kindCommit, Branch: "feature", HeadSHA: strings.Repeat("1", 40), BaseSHA: strings.Repeat("b", 40), BaseRef: "master"}
	if err := db.insertJob(first); err != nil {
		t.Fatal(err)
	}
	c.syncOpen(ctx)
	if p := fake.checks[1]; p.Status != "queued" || p.HeadSHA != first.HeadSHA || p.Name != checkName || !strings.Contains(p.DetailsURL, "https://bench.example/#/job/") {
		t.Fatalf("queued check %+v", p)
	}
	calls := len(fake.calls)
	c.syncOpen(ctx)
	if len(fake.calls) != calls {
		t.Fatalf("an unchanged job was sent again: %v", fake.calls[calls:])
	}
	skipped, err := db.supersedeQueued("feature", strings.Repeat("2", 40))
	if err != nil || len(skipped) != 2 {
		// the pre-reporter job and the first head were both waiting
		t.Fatalf("superseded %v %v", skipped, err)
	}
	second := &job{Kind: kindCommit, Branch: "feature", HeadSHA: strings.Repeat("2", 40), BaseSHA: strings.Repeat("b", 40), BaseRef: "master", PR: 77}
	if err := db.insertJob(second); err != nil {
		t.Fatal(err)
	}
	c.syncOpen(ctx)
	if p := fake.checks[1]; p.Status != "completed" || p.Conclusion != "skipped" || !strings.Contains(p.Output.Summary, "superseded by 222222222222") {
		t.Fatalf("superseded check %+v %+v", p, p.Output)
	}
	if p := fake.checks[2]; p.Status != "queued" || p.HeadSHA != second.HeadSHA {
		t.Fatalf("second head %+v", p)
	}

	// The second head runs: in progress, provisional tables, then final.
	if _, err := db.claimJob(second, "ctl"); err != nil {
		t.Fatal(err)
	}
	if err := db.startJob(second.ID, "go1.27", "deadbeef", ""); err != nil {
		t.Fatal(err)
	}
	second, _ = db.getJob(second.ID)
	c.sync(ctx, second)
	if p := fake.checks[2]; p.Status != "in_progress" || p.Output.Title != "Building and measuring" {
		t.Fatalf("started check %+v %+v", p, p.Output)
	}
	samples := syntheticSamples(second.ID)
	if err := db.insertSamples(samples); err != nil {
		t.Fatal(err)
	}
	c.sync(ctx, second)
	p := fake.checks[2]
	if p.Status != "in_progress" || !strings.Contains(p.Output.Title, "Codegen -10.00%") || !strings.Contains(p.Output.Title, "provisional") {
		t.Fatalf("provisional check %+v", p.Output.Title)
	}
	for _, want := range []string{"| Codegen | -10.00% |", "<summary><b>FuluState</b></summary>", "| Unmarshal |", "**-10.00%**", "https://bench.example/#/job/", "(https://bench.example/#/pr/77)"} {
		if !strings.Contains(p.Output.Summary, want) {
			t.Fatalf("summary lacks %q:\n%s", want, p.Output.Summary[:min(1500, len(p.Output.Summary))])
		}
	}
	if strings.Contains(p.Output.Summary, "FastSSZ") {
		t.Fatal("a reference library is listed as an engine of the commit")
	}
	if err := db.replaceResults(second.ID, summarize(samples)); err != nil {
		t.Fatal(err)
	}
	if err := db.finishJob(second.ID, stateDone, 4, "", 600); err != nil {
		t.Fatal(err)
	}
	second, _ = db.getJob(second.ID)
	c.sync(ctx, second)
	if p := fake.checks[2]; p.Status != "completed" || p.Conclusion != "neutral" || !strings.Contains(p.Output.Title, "(4 passes)") {
		t.Fatalf("final check %+v %s", p, p.Output.Title)
	}
	calls = len(fake.calls)
	c.syncOpen(ctx)
	if len(fake.calls) != calls {
		t.Fatalf("a final check was touched again: %v", fake.calls[calls:])
	}

	// A failed job reports why, without failing the commit.
	third := &job{Kind: kindCommit, Branch: "other", HeadSHA: strings.Repeat("3", 40), BaseSHA: strings.Repeat("b", 40), BaseRef: "master"}
	if err := db.insertJob(third); err != nil {
		t.Fatal(err)
	}
	if err := db.finishJob(third.ID, stateFailed, 0, "build head: does not compile", 5); err != nil {
		t.Fatal(err)
	}
	c.syncOpen(ctx)
	if p := fake.checks[3]; p.Conclusion != "neutral" || !strings.Contains(p.Output.Summary, "does not compile") {
		t.Fatalf("failed check %+v", p)
	}
}

// A fork head is approved only by a review of a maintainer written on
// exactly that commit and carrying the command.
func TestForkApproval(t *testing.T) {
	head := strings.Repeat("a", 40)
	reviews := `[
		{"user":{"login":"stranger"},"body":"/benchmark","commit_id":"` + head + `","author_association":"CONTRIBUTOR"},
		{"user":{"login":"owner"},"body":"looks fine","commit_id":"` + head + `","author_association":"OWNER"},
		{"user":{"login":"owner"},"body":"/benchmark please","commit_id":"` + strings.Repeat("b", 40) + `","author_association":"OWNER"}
	]`
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/repos/o/r/pulls/5/reviews" {
			http.NotFound(rw, req)
			return
		}
		_, _ = io.WriteString(rw, reviews)
	}))
	defer srv.Close()
	c := newPRCache("o/r", "")
	c.api = srv.URL
	ctx := context.Background()
	if by := c.approvedBy(ctx, 5, head); by != "" {
		t.Fatalf("approved by %q: a stranger's command, a maintainer's plain review and a review of another commit do not approve", by)
	}
	reviews = `[{"user":{"login":"maint"},"body":"ok\n/benchmark","commit_id":"` + head + `","author_association":"COLLABORATOR"}]`
	if by := c.approvedBy(ctx, 5, head); by != "maint" {
		t.Fatalf("approved by %q, want maint", by)
	}
	if by := c.approvedBy(ctx, 5, strings.Repeat("c", 40)); by != "" {
		t.Fatalf("a later head is covered by an earlier review: %q", by)
	}
}

// With a sandbox user a build runs unprivileged, a benchmark additionally
// without network, and neither sees the daemon's credentials.
func TestSandboxedCommand(t *testing.T) {
	t.Setenv("RUNNER_TOKEN", "secret")
	t.Setenv("GH_APP_KEY", "/key")
	r := &runner{cfg: &config{sandboxUser: "bench"}}
	build := r.sandboxed(context.Background(), true, "go", "build")
	if got := strings.Join(build.Args, " "); got != "setpriv --reuid=bench --regid=bench --init-groups --no-new-privs -- go build" {
		t.Fatalf("build command %q", got)
	}
	run := r.sandboxed(context.Background(), false, "setarch", "x86_64")
	if got := strings.Join(run.Args, " "); !strings.HasPrefix(got, "unshare --net -- setpriv --reuid=bench") {
		t.Fatalf("benchmark command %q", got)
	}
	env := strings.Join(run.Env, "\n")
	if strings.Contains(env, "secret") || strings.Contains(env, "GH_APP_KEY") || !strings.Contains(env, "HOME=/home/bench") {
		t.Fatalf("sandbox environment:\n%s", env)
	}
	plain := (&runner{cfg: &config{}}).sandboxed(context.Background(), false, "setarch", "x86_64")
	if got := strings.Join(plain.Args, " "); got != "setarch x86_64" {
		t.Fatalf("without a sandbox user the command is wrapped: %q", got)
	}
}

// The webhook records an approval only for a correctly signed "labeled"
// event with the approval label on this repository, bound to the head the
// event carries.
func TestWebhookApproval(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config{github: "o/r", name: "ctl", approveLabel: "benchmark", webhookSecret: "s3cret"}
	w := &webServer{cfg: cfg, db: db}
	head := strings.Repeat("a", 40)
	deliver := func(event, secret, body string) (int, string) {
		req := httptest.NewRequest("POST", "/webhook", strings.NewReader(body))
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write([]byte(body))
		req.Header.Set("X-Hub-Signature-256", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		req.Header.Set("X-GitHub-Event", event)
		rec := httptest.NewRecorder()
		w.webhook(rec, req)
		return rec.Code, strings.TrimSpace(rec.Body.String())
	}
	payload := func(action, label, repo string) string {
		return `{"action":"` + action + `","label":{"name":"` + label + `"},"pull_request":{"number":9,"head":{"sha":"` + head + `"}},"repository":{"full_name":"` + repo + `"},"sender":{"login":"maint"}}`
	}
	approved := func() string { by, _ := db.prApproval(9, head); return by }

	if code, _ := deliver("pull_request", "wrong", payload("labeled", "benchmark", "o/r")); code != http.StatusUnauthorized || approved() != "" {
		t.Fatalf("a delivery with a bad signature: %d, approved by %q", code, approved())
	}
	for _, c := range []struct{ event, body string }{
		{"ping", `{"zen":"hi"}`},
		{"pull_request", payload("opened", "benchmark", "o/r")},
		{"pull_request", payload("labeled", "bug", "o/r")},
		{"pull_request", payload("labeled", "benchmark", "someone/else")},
	} {
		if code, out := deliver(c.event, "s3cret", c.body); code != 200 || out != "ignored" || approved() != "" {
			t.Fatalf("%s %s: %d %q, approved by %q", c.event, c.body, code, out, approved())
		}
	}
	if code, out := deliver("pull_request", "s3cret", payload("labeled", "benchmark", "o/r")); code != 200 || out != "approved" || approved() != "maint" {
		t.Fatalf("labeled: %d %q, approved by %q", code, out, approved())
	}
	if by, _ := db.prApproval(9, strings.Repeat("b", 40)); by != "" {
		t.Fatalf("another head of the pull request is approved by %q", by)
	}
	w.cfg = &config{github: "o/r", approveLabel: "benchmark"}
	if code, _ := deliver("pull_request", "", payload("labeled", "benchmark", "o/r")); code != http.StatusServiceUnavailable {
		t.Fatalf("without a secret configured: %d", code)
	}
}

func TestWorkflowsRan(t *testing.T) {
	head := strings.Repeat("a", 40)
	runs, status := `{"workflow_runs":[]}`, http.StatusOK
	var tokens []string
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/repos/o/r/actions/runs" || req.URL.Query().Get("head_sha") != head {
			http.NotFound(rw, req)
			return
		}
		tokens = append(tokens, req.Header.Get("Authorization"))
		if status != http.StatusOK && req.Header.Get("Authorization") != "" {
			rw.WriteHeader(status)
			return
		}
		_, _ = io.WriteString(rw, runs)
	}))
	defer srv.Close()
	c := newPRCache("o/r", "tok")
	c.api = srv.URL
	ctx := context.Background()
	if c.workflowsRan(ctx, head) {
		t.Fatal("no run counts as run")
	}
	// A run that waits for a maintainer's approval is no approval.
	runs = `{"workflow_runs":[{"status":"completed","conclusion":"action_required","head_sha":"` + head + `"},{"status":"action_required","conclusion":"","head_sha":"` + head + `"}]}`
	if c.workflowsRan(ctx, head) {
		t.Fatal("a run awaiting approval counts as run")
	}
	runs = `{"workflow_runs":[{"status":"completed","conclusion":"action_required","head_sha":"` + head + `"},{"status":"in_progress","conclusion":"","head_sha":"` + head + `"}]}`
	if !c.workflowsRan(ctx, head) {
		t.Fatal("a started run does not count")
	}
	// A token that does not cover the runs: read without it.
	status, tokens = http.StatusForbidden, nil
	if !c.workflowsRan(ctx, head) || len(tokens) != 2 || tokens[0] == "" || tokens[1] != "" {
		t.Fatalf("no retry without the token: %q", tokens)
	}
}
