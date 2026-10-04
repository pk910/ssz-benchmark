package main

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
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
// against the commit before it.
type repoEngine struct {
	Engine         string
	Geomean        float64 // percent, over cycles (time for the async engines and without counters)
	N              int
	Faster, Slower int // operations that moved by more than stepMoved
}

type repoCommit struct {
	SHA, Desc string
	Committed int64    // when the commit was made; zero when the history does not have it
	Tags      []string // tags at the commit
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
}

type repoTarget struct{ Name, Label, SHA string }

type repoView struct {
	Name, Repo string
	Branch     string // of the master target; empty without one
	Targets    []repoTarget
	Total      int // commits of the main branch
	PageSize   int
	Measured   int // of them measured
	Commits    []repoCommit
	Leaves     []leaf `json:",omitempty"`
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
	rows, err = w.db.db.Query(`SELECT head_sha, head_desc, max(id), min(id), count(*), coalesce(max(finished), 0), sum(state = ?), sum(state = ?) FROM jobs
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
		if err := rows.Scan(&c.SHA, &c.Desc, &newest, &oldest, &c.Jobs, &c.Measured, &done, &failed); err != nil {
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
	history, err := w.db.branchCommits(sub.Name)
	if err != nil {
		return nil, err
	}
	var commits []repoCommit
	for _, h := range history {
		c := measured[h.SHA]
		c.SHA, c.Desc, c.Committed, c.Tags = h.SHA, h.Title, h.Committed, h.Tags
		commits = append(commits, c)
	}
	if len(history) == 0 {
		sort.Slice(found, func(a, b int) bool { return first[found[a].SHA] > first[found[b].SHA] })
		commits = found
	}
	v.Total, v.PageSize = len(commits), repoPage

	// The pooled values of every commit, per harness version.
	type hv struct {
		job    int64
		values map[leaf]commitValue
	}
	values := map[string]map[string]*hv{}
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
			values[sha] = map[string]*hv{}
		}
		h := values[sha][harness]
		if h == nil {
			h = &hv{values: map[leaf]commitValue{}}
			values[sha][harness] = h
		}
		h.job = max(h.job, cv.JobID)
		h.values[leaf{Engine: cv.Engine, Object: cv.Object, Op: cv.Op}] = cv
	}
	rows.Close()

	seen := map[leaf]bool{}
	type step struct{ ns, cycles float64 }
	perCommit := make([]map[leaf]step, len(commits))
	for i := range commits {
		c := &commits[i]
		cur := values[c.SHA]
		// The newest harness version the commit has values of gives its
		// number of runs; the newest both have gives the comparison.
		var own *hv
		for _, h := range cur {
			if own == nil || h.job > own.job {
				own = h
			}
		}
		if own != nil {
			for _, cv := range own.values {
				c.Runs = max(c.Runs, cv.N)
			}
		}
		if own == nil {
			continue
		}
		v.Measured++
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
		prev := values[commits[before].SHA]
		var a, b *hv
		for harness, h := range cur {
			if p := prev[harness]; p != nil && (a == nil || h.job > a.job) {
				a, b = h, p
			}
		}
		if a == nil {
			continue
		}
		c.Compared = true
		perCommit[i] = map[leaf]step{}
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
			var st step
			if cv.Ns > 0 && pv.Ns > 0 {
				st.ns = cv.Ns / pv.Ns
			}
			if cv.Cycles > 0 && pv.Cycles > 0 {
				st.cycles = cv.Cycles / pv.Cycles
			}
			perCommit[i][l] = st
			seen[l] = true
			ratio := st.cycles
			if ratio == 0 || strings.HasSuffix(l.Engine, "Async") {
				ratio = st.ns
			}
			if ratio <= 0 {
				continue
			}
			e := engines[l.Engine]
			if e == nil {
				e = &acc{}
				engines[l.Engine] = e
			}
			e.sum += math.Log(ratio)
			e.n++
			if d := (ratio - 1) * 100; d <= -stepMoved {
				e.faster++
			} else if d >= stepMoved {
				e.slower++
			}
		}
		for name, e := range engines {
			c.Engines = append(c.Engines, repoEngine{Engine: name, Geomean: (math.Exp(e.sum/float64(e.n)) - 1) * 100, N: e.n, Faster: e.faster, Slower: e.slower})
		}
		sort.Slice(c.Engines, func(x, y int) bool { return leafLess("", "", c.Engines[x].Engine, "", "", c.Engines[y].Engine) })
	}
	if steps {
		for l := range seen {
			v.Leaves = append(v.Leaves, l)
		}
		sort.Slice(v.Leaves, func(a, b int) bool {
			return leafLess(v.Leaves[a].Object, v.Leaves[a].Op, v.Leaves[a].Engine, v.Leaves[b].Object, v.Leaves[b].Op, v.Leaves[b].Engine)
		})
		for i := offset; i < min(len(commits), offset+limit); i++ {
			if perCommit[i] == nil {
				continue
			}
			commits[i].Steps = make([][2]float64, len(v.Leaves))
			for k, l := range v.Leaves {
				st := perCommit[i][l]
				commits[i].Steps[k] = [2]float64{st.ns, st.cycles}
			}
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
	stamp := w.db.doneStamp() + "|" + strconv.Itoa(w.queueStamp())
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
