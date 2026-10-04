package main

import (
	"context"
	"math"
	"net/http"
	"os/exec"
	"path/filepath"
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
// against the commit before it.
type repoEngine struct {
	Engine         string
	Geomean        float64 // percent, over cycles (time for the async engines and without counters)
	N              int
	Faster, Slower int // operations that moved by more than stepMoved
}

type repoCommit struct {
	SHA, Desc string
	Jobs      int   // jobs that measured it as the head of the main branch
	Runs      int   // runs pooled into its values
	Measured  int64 // when its newest job finished
	State     string
	// Against the commit before it (the next in the list). Compared is
	// false when the two have no values of one harness version.
	Compared bool
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
	Total      int // measured commits of the main branch
	Commits    []repoCommit
	Leaves     []leaf `json:",omitempty"`
}

// mainOrder returns the commits of the mirrored library's main branch,
// newest first.
func mainOrder(dataDir, branch string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", "-C", filepath.Join(dataDir, "repo.git"), "rev-list", "--first-parent", "-n", "2000", branch).Output()
	if err != nil {
		return nil
	}
	return strings.Fields(string(out))
}

func (w *webServer) repoView(sub *subject, limit int, steps bool) (*repoView, error) {
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
	var commits []repoCommit
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
		commits = append(commits, c)
	}
	rows.Close()
	// Newest first: in the order of the branch where the mirror has it,
	// else in the order the commits were found.
	pos := map[string]int{}
	if sub.mirrored() && branch != "" {
		for i, sha := range mainOrder(w.cfg.dataDir, branch) {
			pos[sha] = i + 1
		}
	}
	sort.Slice(commits, func(a, b int) bool {
		pa, pb := pos[commits[a].SHA], pos[commits[b].SHA]
		if pa != pb && pa > 0 && pb > 0 {
			return pa < pb
		}
		return first[commits[a].SHA] > first[commits[b].SHA]
	})
	v.Total = len(commits)

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

	if limit > 0 && len(commits) > limit+1 {
		commits = commits[:limit+1] // one more: the last shown is compared with it
	}
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
		if i+1 >= len(commits) {
			continue
		}
		prev := values[commits[i+1].SHA]
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
		for i := range commits {
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
	if limit > 0 && len(commits) > limit {
		commits = commits[:limit]
	}
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
				if v, err := w.repoView(&subjects[i], 5, false); err == nil {
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
	w.writeJSON(rw, w.pages.get(stamp, "repo|"+name, func() any {
		v, err := w.repoView(sub, 200, true)
		if err != nil {
			return map[string]string{"Error": err.Error()}
		}
		return v
	}))
}
