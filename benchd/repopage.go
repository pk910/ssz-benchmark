package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The repository pages list the libraries with the commits of their main
// branch that were measured, each compared with the one before it:
//
//	/api/repos            every library, with its newest commits
//	/api/repo/<library>   one library, with every commit and, per
//	                      operation, the change at each commit

// stepMoved is the change of an operation between two commits, in
// percent, from which a row counts it as faster or slower.
const stepMoved = 1.0

// repoEngine summarises the change of one engine's operations at a commit
// against the commit before it. The async variant of the engine counts as
// one of its operations.
type repoEngine struct {
	Engine         string
	Geomean        float64 // percent, over cycles (time for the async hashing and without counters)
	N              int
	Faster, Slower int // operations that moved by more than stepMoved
}

type repoCommit struct {
	SHA, Desc string
	Committed int64    // when the commit was made; zero when the history does not have it
	Tags      []string // tags at the commit
	PR        int      // pull request the commit came from: of its jobs, else from its subject "(#N)"
	Jobs      int      // jobs that measured it as the head of the main branch
	Runs      int      // runs pooled into its values
	Measured  int64    // when its newest job finished
	State     string   // of its jobs; empty for a commit that has none
	// Against the measured commit before it. Compared is false when there
	// is none, or the two have no values of one harness version. Skipped
	// counts the unmeasured commits between the two.
	Compared bool
	Against  string
	Skipped  int
	Engines  []repoEngine
	// Steps holds per leaf of the page the ratio to the commit before it,
	// of time and of cycles; zero where one of the two has no value.
	Steps [][2]float64 `json:",omitempty"`
	// Values holds per leaf of the page the commit's own time and cycles
	// per operation (ns, cycles), of the newest harness version it was
	// measured with; zero where it has none.
	Values [][2]float64 `json:",omitempty"`
}

type repoTarget struct{ Name, Label, SHA string }

// repoPR is an open pull request of the mirrored library with its head
// compared against the head of the main branch (from the pooled values of
// both, within one harness version).
type repoPR struct {
	repoCommit // the head: SHA, Desc is the title, Against the main head
	Number     int
	Branch     string // head branch; owner:branch for a fork
	BaseRef    string
	Fork       bool
}

type repoView struct {
	Name, Repo   string
	Branch       string // of the master target; empty without one
	Targets      []repoTarget
	Total        int // commits of the main branch
	PageSize     int
	Measured     int // of them measured
	Commits      []repoCommit
	PullRequests []repoPR `json:",omitempty"` // open, the mirrored library only
	Leaves       []leaf   `json:",omitempty"`
}

// commitStep is the change of one operation between two commits: the
// ratio of time and of cycles; zero where one of the two has no value.
type commitStep struct{ ns, cycles float64 }

// harnessValues are the pooled values of one commit under one harness
// version, with the newest job that measured it.
type harnessValues struct {
	job    int64
	values map[leaf]commitValue
}

// newestValues picks the harness version a commit has the newest values of.
func newestValues(cur map[string]*harnessValues) *harnessValues {
	var own *harnessValues
	for _, h := range cur {
		if own == nil || h.job > own.job {
			own = h
		}
	}
	return own
}

// compareValues compares a commit's values with another commit's under
// the newest harness version both have: per engine the geomean over its
// operations (the async hashing among them) and the counts of operations
// that moved, and per leaf the step. ok is false when the two share no
// harness version.
func compareValues(cur, prev map[string]*harnessValues) ([]repoEngine, map[leaf]commitStep, bool) {
	var a, b *harnessValues
	for harness, h := range cur {
		if p := prev[harness]; p != nil && (a == nil || h.job > a.job) {
			a, b = h, p
		}
	}
	if a == nil {
		return nil, nil, false
	}
	steps := make(map[leaf]commitStep, len(a.values))
	type acc struct {
		sum            float64
		n              int
		faster, slower int
	}
	engines := map[string]*acc{}
	for l, cv := range a.values {
		pv, ok := b.values[l]
		if !ok {
			continue
		}
		var st commitStep
		if cv.Ns > 0 && pv.Ns > 0 {
			st.ns = cv.Ns / pv.Ns
		}
		if cv.Cycles > 0 && pv.Cycles > 0 {
			st.cycles = cv.Cycles / pv.Cycles
		}
		steps[l] = st
		ratio := st.cycles
		if ratio == 0 || isAsync(l.Engine) {
			ratio = st.ns
		}
		if ratio <= 0 {
			continue
		}
		e := engines[baseEngine(l.Engine)]
		if e == nil {
			e = &acc{}
			engines[baseEngine(l.Engine)] = e
		}
		e.sum += math.Log(ratio)
		e.n++
		if d := (ratio - 1) * 100; d <= -stepMoved {
			e.faster++
		} else if d >= stepMoved {
			e.slower++
		}
	}
	out := make([]repoEngine, 0, len(engines))
	for name, e := range engines {
		out = append(out, repoEngine{Engine: name, Geomean: (math.Exp(e.sum/float64(e.n)) - 1) * 100, N: e.n, Faster: e.faster, Slower: e.slower})
	}
	sort.Slice(out, func(x, y int) bool { return leafLess("", "", out[x].Engine, "", "", out[y].Engine) })
	return out, steps, true
}

// fill writes a commit's own values and its steps per leaf of the page.
func (c *repoCommit) fill(leaves []leaf, own *harnessValues, steps map[leaf]commitStep) {
	if own != nil {
		c.Values = make([][2]float64, len(leaves))
		for k, l := range leaves {
			cv := own.values[l]
			c.Values[k] = [2]float64{cv.Ns, cv.Cycles}
		}
	}
	if steps != nil {
		c.Steps = make([][2]float64, len(leaves))
		for k, l := range leaves {
			st := steps[l]
			c.Steps[k] = [2]float64{st.ns, st.cycles}
		}
	}
}

// repoPage is how many commits a page of a library's history holds.
const repoPage = 200

// repoView builds the page of a library: limit commits from offset on,
// with the per-operation steps when steps is set.
func (w *webServer) repoView(sub *subject, offset, limit int, steps bool) (*repoView, error) {
	v := &repoView{Name: sub.Name, Repo: sub.Repo, Commits: []repoCommit{}}
	rows, err := w.db.db.Query(`SELECT name, label, sha FROM targets WHERE subject = ?`, sub.Name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var t repoTarget
		if err := rows.Scan(&t.Name, &t.Label, &t.SHA); err != nil {
			rows.Close()
			return nil, err
		}
		v.Targets = append(v.Targets, t)
		if t.Name == targetMaster {
			v.Branch = t.Label
		}
	}
	rows.Close()
	sort.Slice(v.Targets, func(a, b int) bool { return v.Targets[a].Name < v.Targets[b].Name })
	branch := ""
	for _, t := range sub.Targets {
		if t.Name == targetMaster {
			branch = t.Branch
		}
	}

	// The measured commits of the main branch.
	rows, err = w.db.db.Query(`SELECT head_sha, head_desc, max(id), min(id), count(*), coalesce(max(finished), 0), sum(state = ?), sum(state = ?), max(pr) FROM jobs
		WHERE subject = ? AND kind = ? AND (targets LIKE '%master%' OR (targets = '' AND branch = ? AND pr = 0)) GROUP BY head_sha`, stateDone, stateFailed, sub.Name, kindCommit, branch)
	if err != nil {
		return nil, err
	}
	first := map[string]int64{}
	measured := map[string]repoCommit{}
	var found []repoCommit
	for rows.Next() {
		var c repoCommit
		var newest, oldest int64
		var done, failed int
		if err := rows.Scan(&c.SHA, &c.Desc, &newest, &oldest, &c.Jobs, &c.Measured, &done, &failed, &c.PR); err != nil {
			rows.Close()
			return nil, err
		}
		switch {
		case done > 0:
			c.State = stateDone
		case failed == c.Jobs:
			c.State = stateFailed
		default:
			c.State = stateQueued
		}
		first[c.SHA] = oldest
		measured[c.SHA] = c
		found = append(found, c)
	}
	rows.Close()
	// The list is the history of the branch, newest first, with what was
	// measured of each commit. Without a stored history it is the measured
	// commits in the order they were found.
	var history []branchCommit
	if v.Branch != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		history, _ = branchLog(ctx, historyDir(w.cfg.dataDir, sub), "refs/heads/"+v.Branch)
		cancel()
	}
	var commits []repoCommit
	for _, h := range history {
		c := measured[h.SHA]
		c.SHA, c.Desc, c.Committed, c.Tags = h.SHA, h.Title, h.Committed, h.Tags
		if c.PR == 0 {
			c.PR = prNumber(h.Title)
		}
		commits = append(commits, c)
	}
	if len(history) == 0 {
		sort.Slice(found, func(a, b int) bool { return first[found[a].SHA] > first[found[b].SHA] })
		for i := range found {
			if found[i].PR == 0 {
				found[i].PR = prNumber(found[i].Desc)
			}
		}
		commits = found
	}
	v.Total, v.PageSize = len(commits), repoPage

	// The pooled values of every commit, per harness version.
	values := map[string]map[string]*harnessValues{}
	rows, err = w.db.db.Query(`SELECT sha, harness, engine, object, op, job_id, n, ns, cycles FROM commit_values WHERE subject = ?`, sub.Name)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var sha, harness string
		var cv commitValue
		if err := rows.Scan(&sha, &harness, &cv.Engine, &cv.Object, &cv.Op, &cv.JobID, &cv.N, &cv.Ns, &cv.Cycles); err != nil {
			rows.Close()
			return nil, err
		}
		if values[sha] == nil {
			values[sha] = map[string]*harnessValues{}
		}
		h := values[sha][harness]
		if h == nil {
			h = &harnessValues{values: map[leaf]commitValue{}}
			values[sha][harness] = h
		}
		h.job = max(h.job, cv.JobID)
		h.values[leaf{Engine: cv.Engine, Object: cv.Object, Op: cv.Op}] = cv
	}
	rows.Close()

	seen := map[leaf]bool{}
	perCommit := make([]map[leaf]commitStep, len(commits))
	owns := make([]*harnessValues, len(commits))
	for i := range commits {
		c := &commits[i]
		own := newestValues(values[c.SHA])
		if own == nil {
			continue
		}
		for _, cv := range own.values {
			c.Runs = max(c.Runs, cv.N)
		}
		v.Measured++
		owns[i] = own
		// Measured as something else than the head of the branch (a
		// release on it, the base of a job) counts as well.
		c.State = stateDone
		// The measured commit before it.
		before := -1
		for k := i + 1; k < len(commits); k++ {
			if len(values[commits[k].SHA]) > 0 {
				before = k
				break
			}
		}
		if before < 0 {
			continue
		}
		c.Against, c.Skipped = commits[before].SHA, before-i-1
		engines, steps, ok := compareValues(values[c.SHA], values[commits[before].SHA])
		if !ok {
			continue
		}
		c.Compared, c.Engines, perCommit[i] = true, engines, steps
		for l := range steps {
			seen[l] = true
		}
	}

	// The open pull requests of the mirrored library, each head against
	// the head of the main branch.
	var prOwns []*harnessValues
	var prSteps []map[leaf]commitStep
	if sub.mirrored() {
		var master string
		for _, t := range v.Targets {
			if t.Name == targetMaster {
				master = t.SHA
			}
		}
		prs, _ := w.db.openPullRequests()
		for _, pr := range prs {
			c := repoPR{Number: pr.Number, Branch: pr.Branch, BaseRef: pr.BaseRef, Fork: pr.Fork}
			c.SHA, c.Desc = pr.HeadSHA, pr.Title
			var newest int64
			var state string
			var done int
			_ = w.db.db.QueryRow(`SELECT coalesce(max(id), 0), count(*), coalesce(max(finished), 0), sum(state = ?) FROM jobs WHERE subject = ? AND head_sha = ?`,
				stateDone, sub.Name, pr.HeadSHA).Scan(&newest, &c.Jobs, &c.Measured, &done)
			if newest > 0 {
				_ = w.db.db.QueryRow(`SELECT state FROM jobs WHERE id = ?`, newest).Scan(&state)
			}
			switch {
			case done > 0:
				c.State = stateDone
			default:
				c.State = state
			}
			own := newestValues(values[pr.HeadSHA])
			var steps map[leaf]commitStep
			if own != nil {
				for _, cv := range own.values {
					c.Runs = max(c.Runs, cv.N)
				}
				c.State = stateDone
				if master != "" {
					c.Against = master
					var engines []repoEngine
					engines, steps, c.Compared = compareValues(values[pr.HeadSHA], values[master])
					c.Engines = engines
					for l := range steps {
						seen[l] = true
					}
				}
			}
			v.PullRequests = append(v.PullRequests, c)
			prOwns = append(prOwns, own)
			prSteps = append(prSteps, steps)
		}
	}
	if steps {
		for i := offset; i < min(len(commits), offset+limit); i++ {
			if owns[i] != nil {
				for l := range owns[i].values {
					seen[l] = true
				}
			}
		}
		for _, own := range prOwns {
			if own != nil {
				for l := range own.values {
					seen[l] = true
				}
			}
		}
		for l := range seen {
			v.Leaves = append(v.Leaves, l)
		}
		sort.Slice(v.Leaves, func(a, b int) bool {
			return leafLess(v.Leaves[a].Object, v.Leaves[a].Op, v.Leaves[a].Engine, v.Leaves[b].Object, v.Leaves[b].Op, v.Leaves[b].Engine)
		})
		for i := offset; i < min(len(commits), offset+limit); i++ {
			commits[i].fill(v.Leaves, owns[i], perCommit[i])
		}
		for i := range v.PullRequests {
			v.PullRequests[i].fill(v.Leaves, prOwns[i], prSteps[i])
		}
	}
	offset = min(offset, len(commits))
	commits = commits[offset:min(len(commits), offset+limit)]
	if commits != nil {
		v.Commits = commits
	}
	return v, nil
}

func (w *webServer) apiRepos(rw http.ResponseWriter, req *http.Request) {
	// The pages change with the jobs and with the histories.
	stamp := w.db.doneStamp() + "|" + strconv.Itoa(w.queueStamp()) + "|" + w.historyStamp()
	name := strings.TrimPrefix(strings.TrimPrefix(req.URL.Path, "/api/repos"), "/api/repo/")
	if name == "" {
		w.writeJSON(rw, w.pages.get(stamp, "repos", func() any {
			out := []*repoView{}
			for i := range subjects {
				if v, err := w.repoView(&subjects[i], 0, 5, false); err == nil {
					out = append(out, v)
				}
			}
			return out
		}))
		return
	}
	sub := subjectByName(name)
	if sub == nil {
		http.NotFound(rw, req)
		return
	}
	page, _ := strconv.Atoi(req.URL.Query().Get("page"))
	page = max(page, 1)
	w.writeJSON(rw, w.pages.get(stamp, fmt.Sprintf("repo|%s|%d", name, page), func() any {
		v, err := w.repoView(sub, (page-1)*repoPage, repoPage, true)
		if err != nil {
			return map[string]string{"Error": err.Error()}
		}
		return v
	}))
}

// historyStamp identifies the state of the histories the repository pages
// read: the head of every library's main branch, as the target poller
// stored it, and the tags of the local repositories.
func (w *webServer) historyStamp() string {
	var heads string
	_ = w.db.db.QueryRow(`SELECT coalesce(group_concat(sha, ','), '') FROM (SELECT sha FROM targets ORDER BY subject, name)`).Scan(&heads)
	for i := range subjects {
		out, _ := exec.Command("git", "-C", historyDir(w.cfg.dataDir, &subjects[i]), "for-each-ref", "--count=1", "--sort=-creatordate", "--format=%(refname)", "refs/tags").Output()
		heads += "|" + strings.TrimSpace(string(out))
	}
	return heads
}
