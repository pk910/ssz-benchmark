package main

import (
	"context"
	"net/http"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// The commit page shows one commit of a library with everything measured
// of it: the runs of every job that had it on either side, compared with
// the runs of a base commit chosen on the page. Jobs stay what they
// measured; any number of them feed the page of the same commit.

// commitRuns returns the runs of a commit from every finished job of the
// harness version that measured it, on whichever side, and those jobs.
// Runs a job took over from another job are left out (they are counted
// where they were measured).
func (s *store) commitRuns(subject, sha, harness string) ([]sample, []*job, error) {
	jobs, err := s.scanJobs(s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE state = ? AND subject = ? AND harness = ? AND (head_sha = ? OR base_sha = ?) ORDER BY id DESC LIMIT ?`,
		stateDone, subject, harness, sha, sha, valueJobs))
	if err != nil {
		return nil, nil, err
	}
	var runs []sample
	var used []*job
	for _, j := range jobs {
		samples, err := s.samplesFor(j.ID)
		if err != nil {
			return nil, nil, err
		}
		n := 0
		for _, sm := range samples {
			if sm.isDiag() || sm.fromJob() != 0 || (sm.Side == "head" && j.HeadSHA != sha) || (sm.Side == "base" && j.BaseSHA != sha) {
				continue
			}
			runs = append(runs, sm)
			n++
		}
		if n > 0 {
			used = append(used, j)
		}
	}
	return runs, used, nil
}

// harnessOf is the harness version of the newest finished job that
// measured the commit, as its head when there is one.
func (s *store) harnessOf(subject, sha string) string {
	var harness string
	_ = s.db.QueryRow(`SELECT harness FROM jobs WHERE state = ? AND subject = ? AND (head_sha = ? OR base_sha = ?) ORDER BY (head_sha = ?) DESC, id DESC LIMIT 1`,
		stateDone, subject, sha, sha, sha).Scan(&harness)
	return harness
}

// bySeed reduces the runs of a commit to one per operation and layout
// seed: the median of every figure over the runs of that seed.
func bySeed(runs []sample, side string) map[string]sample {
	groups := map[string][]sample{}
	for _, sm := range runs {
		k := sm.Engine + "/" + sm.Object + "/" + sm.Op + "|" + sm.Seed
		groups[k] = append(groups[k], sm)
	}
	out := map[string]sample{}
	for k, g := range groups {
		m := g[0]
		m.Side, m.JobID = side, 0
		pick := func(f func(sample) float64) float64 {
			vals := make([]float64, len(g))
			for i, sm := range g {
				vals[i] = f(sm)
			}
			return median(vals)
		}
		m.Ns = pick(func(sm sample) float64 { return sm.Ns })
		m.Bytes = pick(func(sm sample) float64 { return sm.Bytes })
		m.Allocs = pick(func(sm sample) float64 { return sm.Allocs })
		m.Cycles = pick(func(sm sample) float64 { return sm.Cycles })
		m.Instrs = pick(func(sm sample) float64 { return sm.Instrs })
		m.Steal = 0
		extra := map[string][]float64{}
		for _, sm := range g {
			m.Steal += sm.Steal
			for key, v := range sm.Extra {
				extra[key] = append(extra[key], v)
			}
		}
		m.Extra = nil
		for key, vals := range extra {
			if m.Extra == nil {
				m.Extra = map[string]float64{}
			}
			m.Extra[key] = median(vals)
		}
		out[k] = m
	}
	return out
}

// compareCommits compares the runs of a head commit with those of a base
// commit, pairing them by layout seed: per operation, every seed both
// were run under contributes one pair. Without base runs the head's
// values stand alone.
func compareCommits(head, base []sample) []result {
	hs, bs := bySeed(head, "head"), bySeed(base, "base")
	keys := make([]string, 0, len(hs))
	for k := range hs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	hasBase := map[string]bool{}
	for k := range bs {
		leafKey, _, _ := strings.Cut(k, "|")
		hasBase[leafKey] = true
	}
	var paired []sample
	pass := map[string]int{}
	for _, k := range keys {
		leafKey, _, _ := strings.Cut(k, "|")
		h := hs[k]
		b, ok := bs[k]
		if hasBase[leafKey] && !ok {
			// A seed the base was not run under has no pair.
			continue
		}
		h.Pass = pass[leafKey]
		paired = append(paired, h)
		if ok {
			b.Pass = pass[leafKey]
			paired = append(paired, b)
		}
		pass[leafKey]++
	}
	return summarize(paired)
}

// commitBase is a commit the page offers to compare against.
type commitBase struct {
	SHA   string
	Label string
	Runs  int // jobs that measured it with the page's harness version
}

// releaseBefore returns the release tag before the one at the commit, and
// its commit, from the mirror.
func releaseBefore(dataDir, sha string) (tag, commit string) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", filepath.Join(dataDir, "repo.git"), "for-each-ref", "--format=%(refname:short) %(objectname) %(*objectname)", "refs/tags/v*").Output()
	if err != nil {
		return "", ""
	}
	commits := map[string]string{}
	var tags []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || strings.Contains(f[0], "-") || strings.Count(f[0], ".") != 2 {
			continue
		}
		commits[f[0]] = f[len(f)-1] // the peeled commit of an annotated tag comes last
		tags = append(tags, f[0])
	}
	sort.Slice(tags, func(a, b int) bool { return versionLess(tags[a], tags[b]) })
	for i, t := range tags {
		if commits[t] == sha && i > 0 {
			return tags[i-1], commits[tags[i-1]]
		}
	}
	return "", ""
}

// commitPage is everything measured of a commit, compared with a base.
type commitPage struct {
	Subject  string
	SHA      string
	Desc     string
	Harness  string
	Repo     string
	Base     string // the commit compared against ("" for none)
	BaseRuns int    // jobs that measured the base
	NoBase   bool   // the chosen base has no runs with this harness version
	Bases    []commitBase
	Jobs     []*job // the jobs that measured the commit
	BaseJobs []*job
	Results  []result
	Noise    noiseFloor
}

func (w *webServer) apiCommit(rw http.ResponseWriter, req *http.Request) {
	parts := strings.SplitN(strings.TrimPrefix(req.URL.Path, "/api/commit/"), "/", 2)
	if len(parts) != 2 || subjectByName(parts[0]) == nil || parts[1] == "" {
		http.NotFound(rw, req)
		return
	}
	page, err := w.commitView(subjectByName(parts[0]), parts[1], req.URL.Query().Get("base"))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	w.writeJSON(rw, page)
}

// commitView pools the runs of a commit and compares them with the runs
// of a base: the one named ("none" for none), else the default for the
// commit.
func (w *webServer) commitView(sub *subject, sha, baseSHA string) (*commitPage, error) {
	harness := w.db.harnessOf(sub.Name, sha)
	head, jobs, err := w.db.commitRuns(sub.Name, sha, harness)
	if err != nil {
		return nil, err
	}
	if len(jobs) == 0 {
		// Nothing measured (yet): the jobs that have the commit as their
		// head, whatever became of them.
		jobs, _ = w.db.scanJobs(w.db.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE subject = ? AND head_sha = ? ORDER BY id DESC LIMIT 20`, sub.Name, sha))
		if jobs == nil {
			jobs = []*job{}
		}
	}
	// The commits offered as a base: the bases its own jobs compared it
	// with, the library's release and main head, and for a release the one
	// before it.
	var bases []commitBase
	seen := map[string]bool{sha: true}
	offer := func(commit, label string) {
		if commit == "" || seen[commit] {
			return
		}
		seen[commit] = true
		runs, used, _ := w.db.commitRuns(sub.Name, commit, harness)
		_ = runs
		bases = append(bases, commitBase{SHA: commit, Label: label, Runs: len(used)})
	}
	targets, _ := w.db.targets()
	var release, master string
	for _, t := range targets {
		if t.Subject == sub.Name && t.Name == targetRelease {
			release = t.SHA
		}
		if t.Subject == sub.Name && t.Name == targetMaster {
			master = t.SHA
		}
	}
	// The default: the base of the pull request the commit was measured
	// for; for a release the release before it; else the latest release.
	def := ""
	for _, j := range jobs {
		if j.HeadSHA == sha && j.BaseSHA != "" {
			label := "base of job #" + strconvI(j.ID)
			if j.PR != 0 && j.Branch != w.cfg.mainBranch {
				label = "base of pull request #" + strconvI(int64(j.PR))
				if def == "" {
					def = j.BaseSHA
				}
			}
			offer(j.BaseSHA, label+" ("+j.BaseRef+")")
		}
	}
	if sha == release && sub.mirrored() {
		if tag, commit := releaseBefore(w.cfg.dataDir, sha); commit != "" {
			offer(commit, "previous release "+tag)
			if def == "" {
				def = commit
			}
		}
	}
	if release != "" {
		offer(release, "latest release")
		if def == "" && sha != release {
			def = release
		}
	}
	offer(master, "head of the main branch")
	if baseSHA == "" {
		baseSHA = def
	}
	if baseSHA == "none" {
		baseSHA = ""
	}
	var base []sample
	var baseJobs []*job
	if baseSHA != "" {
		base, baseJobs, _ = w.db.commitRuns(sub.Name, baseSHA, harness)
	}
	results := compareCommits(head, base)
	if results == nil {
		results = []result{}
	}
	have := map[string]bool{}
	for _, r := range results {
		have[joinKey(r.Engine, r.Object, r.Op)] = true
	}
	for _, r := range w.otherResults(sub.Name) {
		if !have[joinKey(r.Engine, r.Object, r.Op)] {
			results = append(results, r)
		}
	}
	desc := ""
	for _, j := range jobs {
		if j.HeadSHA == sha {
			desc = j.HeadDesc
		}
	}
	if baseJobs == nil {
		baseJobs = []*job{}
	}
	return &commitPage{sub.Name, sha, desc, harness, repoURL(sub.Name), baseSHA, len(baseJobs), baseSHA != "" && len(base) == 0, bases, jobs, baseJobs, results, w.noiseFloor()}, nil
}
