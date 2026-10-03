package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	gitTimeout      = 2 * time.Minute
	gitFetchTimeout = 3 * time.Minute
)

// gitRepo is a bare mirror of the benchmarked repository.
type gitRepo struct {
	dir string
	url string
	mu  sync.Mutex // one git command at a time on the mirror
}

// run executes one git command on the mirror with a deadline: a network
// command gets gitFetchTimeout, anything else gitTimeout. A command that
// overruns is killed and reported as an error, never waited for.
func (g *gitRepo) run(args ...string) (string, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	timeout := gitTimeout
	if len(args) > 0 && (args[0] == "fetch" || args[0] == "clone") {
		timeout = gitFetchTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", g.dir}, args...)...)
	cmd.WaitDelay = 5 * time.Second
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func (g *gitRepo) ensureMirror() error {
	if _, err := os.Stat(filepath.Join(g.dir, "HEAD")); err == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", g.url, g.dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("clone mirror: %v: %s", err, out)
	}
	return nil
}

func (g *gitRepo) fetch() error {
	_, err := g.run("fetch", "--prune", "--tags", "--force", "origin")
	return err
}

// fetchPR fetches the head of a pull request into refs/pr/<n> and returns
// its commit.
func (g *gitRepo) fetchPR(number int) (string, error) {
	ref := fmt.Sprintf("refs/pr/%d", number)
	if _, err := g.run("fetch", "--force", "origin", fmt.Sprintf("+refs/pull/%d/head:%s", number, ref)); err != nil {
		return "", err
	}
	return g.revParse(ref)
}

// branches returns every branch tip of the mirror as ref -> sha.
func (g *gitRepo) branches() (map[string]string, error) {
	out, err := g.run("for-each-ref", "--format=%(refname:short) %(objectname)", "refs/heads/")
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 2 {
			refs[f[0]] = f[1]
		}
	}
	return refs, nil
}

func (g *gitRepo) revParse(ref string) (string, error) {
	return g.run("rev-parse", "--verify", ref+"^{commit}")
}

func (g *gitRepo) mergeBase(a, b string) (string, error) {
	return g.run("merge-base", a, b)
}

// firstParentLog lists the newest n first-parent commits from sha, newest first.
func (g *gitRepo) firstParentLog(sha string, n int) ([]string, error) {
	out, err := g.run("rev-list", "--first-parent", "-n", strconv.Itoa(n), sha)
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// firstParentRange lists the first-parent commits after from up to to,
// newest first.
func (g *gitRepo) firstParentRange(from, to string) ([]string, error) {
	out, err := g.run("rev-list", "--first-parent", from+".."+to)
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

func (g *gitRepo) describe(sha string) string {
	out, err := g.run("log", "-1", "--format=%h %s", sha)
	if err != nil {
		return sha[:12]
	}
	if len(out) > 90 {
		out = out[:87] + "..."
	}
	return out
}

// subject is the full first line of a commit message.
func (g *gitRepo) subject(sha string) string {
	out, err := g.run("log", "-1", "--format=%s", sha)
	if err != nil {
		return ""
	}
	return out
}

func (g *gitRepo) commitTime(sha string) (time.Time, error) {
	out, err := g.run("log", "-1", "--format=%ct", sha)
	if err != nil {
		return time.Time{}, err
	}
	n, err := strconv.ParseInt(out, 10, 64)
	if err != nil {
		return time.Time{}, err
	}
	return time.Unix(n, 0), nil
}

// latestRelease returns the highest vX.Y.Z tag without a pre-release suffix.
func (g *gitRepo) latestRelease() (string, string, error) {
	out, err := g.run("tag", "--list", "v*")
	if err != nil {
		return "", "", err
	}
	type ver struct {
		tag   string
		parts [3]int
	}
	var vers []ver
	for _, tag := range strings.Split(out, "\n") {
		tag = strings.TrimSpace(tag)
		if tag == "" || strings.Contains(tag, "-") {
			continue
		}
		p := strings.Split(strings.TrimPrefix(tag, "v"), ".")
		if len(p) != 3 {
			continue
		}
		var v ver
		v.tag = tag
		ok := true
		for i := range p {
			n, err := strconv.Atoi(p[i])
			if err != nil {
				ok = false
				break
			}
			v.parts[i] = n
		}
		if ok {
			vers = append(vers, v)
		}
	}
	if len(vers) == 0 {
		return "", "", fmt.Errorf("no release tag")
	}
	sort.Slice(vers, func(i, j int) bool {
		for k := range 3 {
			if vers[i].parts[k] != vers[j].parts[k] {
				return vers[i].parts[k] > vers[j].parts[k]
			}
		}
		return false
	})
	sha, err := g.revParse(vers[0].tag)
	return vers[0].tag, sha, err
}

// worktree checks sha out under dir (reused when present).
func (g *gitRepo) worktree(dir, sha string) error {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		head, err := exec.Command("git", "-C", dir, "rev-parse", "HEAD").Output()
		if err == nil && strings.TrimSpace(string(head)) == sha {
			return nil
		}
		g.removeWorktree(dir)
	}
	_, err := g.run("worktree", "add", "--detach", "--force", dir, sha)
	return err
}

func (g *gitRepo) removeWorktree(dir string) {
	_, _ = g.run("worktree", "remove", "--force", dir)
	_ = os.RemoveAll(dir)
	_, _ = g.run("worktree", "prune")
}

// pullRequest is an open pull request of the repository.
type pullRequest struct {
	Number  int
	HeadRef string
	HeadSHA string
	BaseRef string
}

// prCache polls the open pull requests of the repository.
type prCache struct {
	repo    string
	token   string
	tokenFn func(ctx context.Context) string // installation token of the GitHub App, when there is one
	api     string                           // API root, replaced in tests
	forks   []forkPR
	mu      sync.Mutex
	byHead  map[string]pullRequest // head branch -> PR
	etag    string
	fetched time.Time
}

func newPRCache(repo, token string) *prCache {
	return &prCache{repo: repo, token: token, api: "https://api.github.com", byHead: map[string]pullRequest{}}
}

// forkPR is an open pull request whose head lives in another repository.
type forkPR struct {
	Number  int
	HeadSHA string
	Label   string // owner:branch
	BaseRef string
}

// bearer is the token for API requests: the App's when present, else the
// configured one.
func (c *prCache) bearer(ctx context.Context) string {
	if c.tokenFn != nil {
		if t := c.tokenFn(ctx); t != "" {
			return t
		}
	}
	return c.token
}

// forkPRs returns the open pull requests from forks as last listed.
func (c *prCache) forkPRs() []forkPR {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]forkPR(nil), c.forks...)
}

// benchmarkCommand in the body of a maintainer's review approves measuring
// the commit the review was submitted on.
const benchmarkCommand = "/benchmark"

// approvedBy returns who approved measuring exactly this head of a fork
// pull request, or "". The approval is a review by an owner, member or
// collaborator of the repository whose body contains the command: GitHub
// binds a review to the commit it was written on, so a later push is not
// covered by it.
func (c *prCache) approvedBy(ctx context.Context, number int, headSHA string) string {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/repos/%s/pulls/%d/reviews?per_page=100", c.api, c.repo, number), nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "benchd")
	if t := c.bearer(ctx); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("reviews of #%d: %v", number, err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("reviews of #%d: HTTP %s", number, resp.Status)
		return ""
	}
	var reviews []struct {
		User struct {
			Login string `json:"login"`
		} `json:"user"`
		Body        string `json:"body"`
		CommitID    string `json:"commit_id"`
		Association string `json:"author_association"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reviews); err != nil {
		return ""
	}
	for _, r := range reviews {
		switch r.Association {
		case "OWNER", "MEMBER", "COLLABORATOR":
		default:
			continue
		}
		if r.CommitID == headSHA && strings.Contains(r.Body, benchmarkCommand) {
			return r.User.Login
		}
	}
	return ""
}

// refresh reloads the open pull requests at most every five minutes.
func (c *prCache) refresh(ctx context.Context) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if time.Since(c.fetched) < 5*time.Minute {
		return
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.api+"/repos/"+c.repo+"/pulls?state=open&per_page=100", nil)
	if err != nil {
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "benchd")
	if t := c.bearer(ctx); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
	if c.etag != "" {
		req.Header.Set("If-None-Match", c.etag)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("pull requests: %v", err)
		return
	}
	defer resp.Body.Close()
	c.fetched = time.Now()
	if resp.StatusCode == http.StatusNotModified {
		return
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("pull requests: HTTP %s", resp.Status)
		return
	}
	var prs []struct {
		Number int `json:"number"`
		Head   struct {
			Ref  string `json:"ref"`
			SHA  string `json:"sha"`
			Repo struct {
				FullName string `json:"full_name"`
			} `json:"repo"`
			Label string `json:"label"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prs); err != nil {
		log.Printf("pull requests: decode: %v", err)
		return
	}
	c.etag = resp.Header.Get("ETag")
	byHead := map[string]pullRequest{}
	var forks []forkPR
	for _, pr := range prs {
		// Only branches of this repository are mirrored; a fork's head is
		// fetched when a maintainer approves it.
		if !strings.EqualFold(pr.Head.Repo.FullName, c.repo) {
			forks = append(forks, forkPR{Number: pr.Number, HeadSHA: pr.Head.SHA, Label: pr.Head.Label, BaseRef: pr.Base.Ref})
			continue
		}
		byHead[pr.Head.Ref] = pullRequest{Number: pr.Number, HeadRef: pr.Head.Ref, HeadSHA: pr.Head.SHA, BaseRef: pr.Base.Ref}
	}
	c.byHead = byHead
	c.forks = forks
}

func (c *prCache) forBranch(branch string) (pullRequest, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	pr, ok := c.byHead[branch]
	return pr, ok
}

// scheduler turns new commits into jobs and keeps the queue non-empty.
type scheduler struct {
	cfg       *config
	db        *store
	git       *gitRepo
	prs       *prCache
	idleCount int
	runner    *runner        // publishes the fetch status with its own
	checks    *checkReporter // nil without a GitHub App
	ready     chan struct{}  // closed after the first fetch
	readyOnce sync.Once
	mu        sync.Mutex
	lastFetch time.Time
	lastErr   string
}

func (s *scheduler) loop(ctx context.Context) {
	for {
		s.tick(ctx)
		s.readyOnce.Do(func() { close(s.ready) })
		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cfg.poll):
		}
	}
}

func (s *scheduler) tick(ctx context.Context) {
	err := s.git.fetch()
	s.mu.Lock()
	s.lastFetch = time.Now()
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
	s.mu.Unlock()
	if s.runner != nil {
		s.runner.publish()
	}
	if err != nil {
		log.Printf("fetch: %v", err)
		return
	}
	s.prs.refresh(ctx)
	branches, err := s.git.branches()
	if err != nil {
		log.Printf("branches: %v", err)
		return
	}
	// The periodic noise job goes in first so its number matches its place
	// in the queue on the first run.
	s.scheduleNoise()
	s.scheduleBaseline()
	if n, err := s.db.seenCount(); err == nil && n == 0 {
		s.bootstrap(branches)
	}
	s.resolvePRs()
	// Oldest tips first so a burst of pushes is measured in order.
	type tip struct {
		ref, sha string
		when     time.Time
	}
	var tips []tip
	for ref, sha := range branches {
		seen, err := s.db.seenRef(ref)
		if err != nil {
			log.Printf("seen: %v", err)
			return
		}
		if seen == sha {
			continue
		}
		when, _ := s.git.commitTime(sha)
		tips = append(tips, tip{ref, sha, when})
	}
	sort.Slice(tips, func(i, j int) bool { return tips[i].when.Before(tips[j].when) })
	for _, t := range tips {
		seen, _ := s.db.seenRef(t.ref)
		var err error
		if t.ref == s.cfg.mainBranch {
			err = s.enqueueMainRange(seen, t.sha)
		} else {
			err = s.enqueueBranchTip(t.ref, t.sha)
		}
		if err != nil {
			log.Printf("enqueue %s: %v", t.ref, err)
			continue
		}
		if err := s.db.markRef(t.ref, t.sha); err != nil {
			log.Printf("mark %s: %v", t.ref, err)
		}
	}
	s.enqueueForkPRs(ctx)
	s.checks.syncOpen(ctx)
}

// enqueueForkPRs queues the heads of fork pull requests a maintainer
// approved. A fork's code is only ever run inside the sandbox, so without
// one configured no fork is measured.
func (s *scheduler) enqueueForkPRs(ctx context.Context) {
	for _, pr := range s.prs.forkPRs() {
		branch := fmt.Sprintf("pr/%d", pr.Number)
		if seen, _ := s.db.seenRef(branch); seen == pr.HeadSHA {
			continue
		}
		if s.cfg.sandboxUser == "" {
			continue
		}
		by := s.prs.approvedBy(ctx, pr.Number, pr.HeadSHA)
		if by == "" {
			continue
		}
		sha, err := s.git.fetchPR(pr.Number)
		if err != nil {
			log.Printf("fork #%d: %v", pr.Number, err)
			continue
		}
		if sha != pr.HeadSHA {
			// The head moved between the listing and the fetch: the approval
			// does not cover what was fetched.
			continue
		}
		baseTip, ok := s.git.branches2(pr.BaseRef)
		if !ok {
			continue
		}
		baseSHA, err := s.git.mergeBase(baseTip, sha)
		if err != nil || baseSHA == sha {
			continue
		}
		if exists, err := s.db.hasJobFor(sha, baseSHA); err == nil && !exists {
			if _, err := s.db.supersedeQueued(branch, sha); err != nil {
				log.Printf("fork #%d: %v", pr.Number, err)
				continue
			}
			j := &job{Kind: kindCommit, Branch: branch, HeadSHA: sha, HeadDesc: s.git.describe(sha), BaseSHA: baseSHA, BaseRef: pr.BaseRef,
				BaseDesc: s.git.describe(baseSHA), PR: pr.Number, Note: fmt.Sprintf("fork %s, measuring approved by %s", pr.Label, by)}
			log.Printf("queue fork #%d %s (%s) against %s, approved by %s", pr.Number, sha[:12], pr.Label, baseSHA[:12], by)
			if err := s.db.insertJob(j); err != nil {
				log.Printf("fork #%d: %v", pr.Number, err)
				continue
			}
		}
		_ = s.db.markRef(branch, sha)
	}
}

// bootstrap fills the queue on the first run: the last backfill commits of
// the main branch (each squash-merged pull request is one first-parent
// commit) and the tips of branches with an open pull request. Every other
// existing branch tip is taken as seen.
func (s *scheduler) bootstrap(branches map[string]string) {
	mainSHA := branches[s.cfg.mainBranch]
	queued := 0
	if mainSHA != "" {
		shas, err := s.git.firstParentLog(mainSHA, s.cfg.backfill)
		if err != nil {
			log.Printf("bootstrap: %v", err)
		}
		for i := len(shas) - 1; i >= 0; i-- {
			if err := s.enqueueMainCommit(shas[i]); err != nil {
				log.Printf("bootstrap %s: %v", shas[i][:12], err)
				continue
			}
			queued++
		}
		_ = s.db.markRef(s.cfg.mainBranch, mainSHA)
	}
	for ref, sha := range branches {
		if ref == s.cfg.mainBranch {
			continue
		}
		if _, ok := s.prs.forBranch(ref); ok {
			continue // queued by the regular tick as a new tip
		}
		_ = s.db.markRef(ref, sha)
	}
	log.Printf("first run: queued the last %d %s commits, %d existing branches without a pull request taken as seen", queued, s.cfg.mainBranch, len(branches)-1)
}

// enqueueMainRange queues every first-parent commit of the main branch
// between the last seen tip and the new one, oldest first, each against its
// parent; a tip with no known predecessor queues just the tip.
func (s *scheduler) enqueueMainRange(seen, sha string) error {
	var shas []string
	if seen != "" {
		var err error
		shas, err = s.git.firstParentRange(seen, sha)
		if err != nil {
			return err
		}
	}
	if len(shas) == 0 || len(shas) > s.cfg.backfill {
		shas = []string{sha}
	}
	for i := len(shas) - 1; i >= 0; i-- {
		if err := s.enqueueMainCommit(shas[i]); err != nil {
			return err
		}
	}
	return nil
}

// enqueueMainCommit queues one main-branch commit against its first parent.
func (s *scheduler) enqueueMainCommit(sha string) error {
	parent, err := s.git.revParse(sha + "^")
	if err != nil {
		return fmt.Errorf("no parent for %s: %w", sha, err)
	}
	j := &job{Kind: kindCommit, Branch: s.cfg.mainBranch, HeadSHA: sha, HeadDesc: s.git.describe(sha),
		BaseSHA: parent, BaseRef: s.cfg.mainBranch + "^", BaseDesc: s.git.describe(parent), PR: prNumber(s.git.subject(sha))}
	if exists, err := s.db.hasJobFor(sha, parent); err != nil || exists {
		return err
	}
	log.Printf("queue %s %s against its parent %s", s.cfg.mainBranch, sha[:12], parent[:12])
	return s.db.insertJob(j)
}

// enqueueBranchTip queues the tip of a branch against its merge base with
// the pull request base branch (when a pull request is open) or the main
// branch; a tip the base already contains is measured against its parent.
func (s *scheduler) enqueueBranchTip(ref, sha string) error {
	j := &job{Kind: kindCommit, Branch: ref, HeadSHA: sha, HeadDesc: s.git.describe(sha)}
	baseRef := s.cfg.mainBranch
	if pr, ok := s.prs.forBranch(ref); ok {
		baseRef = pr.BaseRef
		j.PR = pr.Number
	}
	var baseSHA string
	if baseTip, ok := s.git.branches2(baseRef); ok {
		mb, err := s.git.mergeBase(baseTip, sha)
		if err != nil {
			return err
		}
		if mb != sha {
			baseSHA = mb
		}
	}
	if baseSHA == "" {
		parent, err := s.git.revParse(sha + "^")
		if err != nil {
			return fmt.Errorf("no parent for %s: %w", sha, err)
		}
		baseSHA = parent
		baseRef = "parent"
	}
	j.BaseSHA = baseSHA
	j.BaseRef = baseRef
	j.BaseDesc = s.git.describe(baseSHA)
	if exists, err := s.db.hasJobFor(sha, baseSHA); err != nil || exists {
		return err
	}
	// A newer head makes the branch's waiting jobs pointless; the one
	// running finishes and stays as a point in the branch's history.
	if skipped, err := s.db.supersedeQueued(ref, sha); err != nil {
		return err
	} else if len(skipped) > 0 {
		log.Printf("%s: %d queued job(s) superseded by %s", ref, len(skipped), sha[:12])
	}
	log.Printf("queue %s %s against %s (%s)", ref, sha[:12], baseSHA[:12], baseRef)
	return s.db.insertJob(j)
}

// scheduleNoise queues a self-comparison of the main branch ahead of the
// queue when the last one is older than the noise interval, so the noise
// floor is tracked at a steady cadence even while commits wait.
func (s *scheduler) scheduleNoise() {
	if s.cfg.noiseInterval <= 0 {
		return
	}
	last, err := s.db.lastJobOfKind(kindNoise)
	if err != nil {
		return
	}
	if last != nil && (last.State == stateQueued || last.State == stateRunning || time.Since(last.Created) < s.cfg.noiseInterval) {
		return
	}
	j, err := s.idleJobOfKind(kindNoise)
	if err != nil {
		log.Printf("noise job: %v", err)
		return
	}
	j.Priority = 1
	j.Note = "scheduled noise floor: the same commit on both sides"
	if err := s.db.insertJob(j); err != nil {
		log.Printf("noise job: %v", err)
	}
}

// baselineRef stands in for the commit of a baseline job, which measures
// no checkout.
const baselineRef = "baselines"

// scheduleBaseline queues a measurement of the reference libraries when
// their modules changed since the last one or the interval has passed.
func (s *scheduler) scheduleBaseline() {
	if s.cfg.baselineInterval <= 0 {
		return
	}
	hash, err := baselineHash(s.cfg.harnessDir)
	if err != nil {
		return
	}
	last, err := s.db.lastJobOfKind(kindBaseline)
	if err != nil {
		return
	}
	if last != nil {
		if last.State == stateQueued || last.State == stateRunning {
			return
		}
		if last.Harness == hash && time.Since(last.Created) < s.cfg.baselineInterval {
			return
		}
	}
	j := baselineJob("the other SSZ libraries on the same payload")
	j.Priority = 1
	if err := s.db.insertJob(j); err != nil {
		log.Printf("baseline job: %v", err)
	}
}

// baselineJob is a measurement of the reference libraries; its runs pool
// like those of any pair.
func baselineJob(note string) *job {
	return &job{Kind: kindBaseline, Branch: "-", HeadSHA: baselineRef, HeadDesc: "reference SSZ libraries", BaseSHA: baselineRef, Note: note}
}

// resolvePRs fills the pull request of main-branch commit jobs that have
// none from the commit's full subject (a squash merge ends in "(#N)").
func (s *scheduler) resolvePRs() {
	jobs, err := s.db.listJobs(1<<30, kindCommit)
	if err != nil {
		return
	}
	for _, j := range jobs {
		if j.PR != 0 || j.Branch != s.cfg.mainBranch {
			continue
		}
		if n := prNumber(s.git.subject(j.HeadSHA)); n > 0 {
			_ = s.db.setJobPR(j.ID, n)
		}
	}
}

// prNumber reads the pull request number from a squash-merge subject "(#N)".
func prNumber(desc string) int {
	i := strings.LastIndex(desc, "(#")
	if i < 0 {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSuffix(desc[i+2:], ")"))
	return n
}

// branches2 resolves a branch name to its tip.
func (g *gitRepo) branches2(name string) (string, bool) {
	sha, err := g.revParse("refs/heads/" + name)
	if err != nil {
		return "", false
	}
	return sha, true
}

// idleJob builds the job to run when nothing waits, in rotation: the main
// branch against itself (noise floor), the main branch against the latest
// release, the reference libraries, and reruns of the least-refined
// comparison in between.
// The machine never idles; every rerun pools with the earlier runs of its
// pair and tightens that comparison.
func (s *scheduler) idleJob() (*job, error) {
	s.idleCount++
	switch s.idleCount % 8 {
	case 0:
		return s.idleJobOfKind(kindNoise)
	case 2, 6:
		if j, err := s.releaseJob(); err != nil || j != nil {
			return j, err
		}
	case 4:
		if s.cfg.baselineInterval > 0 {
			return baselineJob("idle refresh of the reference libraries"), nil
		}
	}
	prev, err := s.db.leastRefinedPair()
	if err != nil {
		return nil, err
	}
	if prev == nil {
		if j, err := s.releaseJob(); err != nil || j != nil {
			return j, err
		}
		return s.idleJobOfKind(kindNoise)
	}
	return &job{Kind: kindCommit, Branch: prev.Branch, HeadSHA: prev.HeadSHA, HeadDesc: prev.HeadDesc,
		BaseSHA: prev.BaseSHA, BaseRef: prev.BaseRef, BaseDesc: prev.BaseDesc, PR: prev.PR, Note: "refinement run"}, nil
}

// releaseJob is the main branch against the latest release, or nil when
// that pair is known not to build (the harness needs library options the
// release lacks).
func (s *scheduler) releaseJob() (*job, error) {
	j, err := s.idleJobOfKind(kindRelease)
	if err != nil {
		return nil, err
	}
	failed, err := s.db.pairBuildFailed(j.HeadSHA, j.BaseSHA)
	if err != nil || failed {
		return nil, err
	}
	return j, nil
}

func (s *scheduler) idleJobOfKind(kind string) (*job, error) {
	mainSHA, ok := s.git.branches2(s.cfg.mainBranch)
	if !ok {
		return nil, fmt.Errorf("no %s branch", s.cfg.mainBranch)
	}
	j := &job{Branch: s.cfg.mainBranch, HeadSHA: mainSHA, HeadDesc: s.git.describe(mainSHA), Kind: kind}
	if kind == kindNoise {
		j.BaseSHA = mainSHA
		j.BaseRef = s.cfg.mainBranch
		j.BaseDesc = j.HeadDesc
		j.Note = "noise floor: the same commit on both sides"
		return j, nil
	}
	tag, sha, err := s.git.latestRelease()
	if err != nil {
		return nil, err
	}
	j.BaseSHA = sha
	j.BaseRef = tag
	j.BaseDesc = tag + " " + s.git.describe(sha)
	return j, nil
}

// manualJob builds a commit job for two refs given by hand.
func (s *scheduler) manualJob(headRef, baseRef string) (*job, error) {
	headSHA, err := s.git.revParse(headRef)
	if err != nil {
		return nil, err
	}
	baseSHA, err := s.git.revParse(baseRef)
	if err != nil {
		return nil, err
	}
	return &job{Kind: kindCommit, Branch: headRef, HeadSHA: headSHA, HeadDesc: s.git.describe(headSHA),
		BaseSHA: baseSHA, BaseRef: baseRef, BaseDesc: s.git.describe(baseSHA), Note: "queued by hand"}, nil
}

func (s *scheduler) status() (time.Time, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastFetch, s.lastErr
}
