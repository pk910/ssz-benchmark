package main

import (
	"net/http"
	"sort"
	"strings"
)

// subjectValues are the pooled values a subject shows in a mode.
type subjectValues struct {
	Name   string
	Target string // the target shown: master, release or fixed
	Label  string // branch or tag
	SHA    string // commit the values belong to
	// Wanted is the commit the target points to when the values are of an
	// older one: the newer commit is not measured yet or does not build.
	Wanted string
	JobID  int64
	Repo   string
	values []commitValue
}

// valuesOf picks what a subject shows in a mode: its fixed target, else
// the target of the mode; when that commit has no values, the newest
// measured commit of the same target.
func (w *webServer) valuesOf(name, mode string, targets []targetState) *subjectValues {
	var pick *targetState
	for i := range targets {
		t := &targets[i]
		if t.Subject == name && (t.Name == mode || t.Name == targetFixed) {
			pick = t
		}
	}
	if pick == nil {
		return nil
	}
	sv := &subjectValues{Name: name, Target: pick.Name, Label: pick.Label, SHA: pick.SHA, Repo: repoURL(w.cfg, name)}
	sv.values, _ = w.db.commitValues(name, pick.SHA)
	if len(sv.values) == 0 {
		// The newest finished job of this target, whatever commit it had.
		var sha string
		var err error
		if name == subjectDynSSZ {
			if mode != targetMaster {
				return sv
			}
			err = w.db.db.QueryRow(`SELECT head_sha FROM jobs WHERE state = ? AND subject = ? AND kind = ? AND branch = ? ORDER BY id DESC LIMIT 1`,
				stateDone, name, kindCommit, w.cfg.mainBranch).Scan(&sha)
		} else {
			err = w.db.db.QueryRow(`SELECT head_sha FROM jobs WHERE state = ? AND subject = ? AND kind = ? AND ('+' || branch || '+') LIKE ? ORDER BY id DESC LIMIT 1`,
				stateDone, name, kindLib, "%+"+pick.Name+"+%").Scan(&sha)
		}
		if err != nil || sha == "" {
			return sv
		}
		sv.Wanted, sv.SHA = pick.SHA, sha
		sv.values, _ = w.db.commitValues(name, sha)
	}
	for _, v := range sv.values {
		sv.JobID = max(sv.JobID, v.JobID)
	}
	return sv
}

func subjectNames() []string {
	names := []string{subjectDynSSZ}
	for i := range librarySubjects {
		names = append(names, librarySubjects[i].Name)
	}
	return names
}

// libraryResults returns the other libraries' values as results, for the
// pages that show them next to a dynamic-ssz job: each library at its
// release (or fixed) target, at its master when it has no release value.
func (w *webServer) libraryResults() []result {
	targets, _ := w.db.targets()
	var out []result
	for i := range librarySubjects {
		sv := w.valuesOf(librarySubjects[i].Name, targetRelease, targets)
		if sv == nil || len(sv.values) == 0 {
			sv = w.valuesOf(librarySubjects[i].Name, targetMaster, targets)
		}
		if sv == nil {
			continue
		}
		for _, v := range sv.values {
			out = append(out, result{JobID: v.JobID, Engine: v.Engine, Object: v.Object, Op: v.Op, Baseline: true, N: v.N,
				Ns: metric{Head: v.Ns, CVHead: v.CVNs}, Cycles: metric{Head: v.Cycles, CVHead: v.CVCycles}, Instrs: metric{Head: v.Instrs},
				Bytes: metric{Head: v.Bytes}, Allocs: metric{Head: v.Allocs}})
		}
	}
	return out
}

// opsCell is the pooled value of one engine for one operation, and in
// master mode its change against the same library's release, per metric,
// in percent.
type opsCell struct {
	Subject                           string
	JobID                             int64
	N                                 int
	Ns, Cycles, Instrs, Bytes, Allocs float64
	CVNs, CVCycles                    float64
	Rel                               map[string]float64 `json:",omitempty"`
}

type opsRow struct {
	Object, Op string
	Cells      map[string]*opsCell
}

// opsPage is the operations page in one mode.
type opsPage struct {
	Mode     string
	Subjects []*subjectValues
	Engines  []string
	Rows     []opsRow
}

func (w *webServer) buildOps(mode string) any {
	targets, _ := w.db.targets()
	p := &opsPage{Mode: mode, Subjects: []*subjectValues{}, Rows: []opsRow{}}
	rows := map[string]*opsRow{}
	engines := map[string]bool{}
	for _, name := range subjectNames() {
		sv := w.valuesOf(name, mode, targets)
		if sv == nil {
			continue
		}
		p.Subjects = append(p.Subjects, sv)
		// The release values of the same subject, for the change against
		// them; only values of the same harness version compare.
		release := map[string]commitValue{}
		if mode == targetMaster && sv.Target == targetMaster {
			if rv := w.valuesOf(name, targetRelease, targets); rv != nil && rv.SHA != sv.SHA {
				for _, v := range rv.values {
					release[joinKey(v.Engine, v.Object, v.Op)] = v
				}
			}
		}
		for _, v := range sv.values {
			k := v.Object + "/" + v.Op
			if rows[k] == nil {
				rows[k] = &opsRow{Object: v.Object, Op: v.Op, Cells: map[string]*opsCell{}}
			}
			engines[v.Engine] = true
			c := &opsCell{Subject: name, JobID: v.JobID, N: v.N, Ns: v.Ns, Cycles: v.Cycles, Instrs: v.Instrs, Bytes: v.Bytes, Allocs: v.Allocs, CVNs: v.CVNs, CVCycles: v.CVCycles}
			if r, ok := release[joinKey(v.Engine, v.Object, v.Op)]; ok && r.Harness == v.Harness {
				c.Rel = map[string]float64{}
				for metric, pair := range map[string][2]float64{"Ns": {v.Ns, r.Ns}, "Cycles": {v.Cycles, r.Cycles}, "Instrs": {v.Instrs, r.Instrs}, "Bytes": {v.Bytes, r.Bytes}, "Allocs": {v.Allocs, r.Allocs}} {
					if pair[1] > 0 {
						c.Rel[metric] = (pair[0] - pair[1]) / pair[1] * 100
					}
				}
			}
			rows[k].Cells[v.Engine] = c
		}
	}
	p.Engines = sortedBy(engines, engineOrder)
	for _, r := range rows {
		p.Rows = append(p.Rows, *r)
	}
	sort.Slice(p.Rows, func(a, b int) bool {
		return leafLess(p.Rows[a].Object, p.Rows[a].Op, "", p.Rows[b].Object, p.Rows[b].Op, "")
	})
	return p
}

// targetStamp changes when a target moves to another commit.
func (s *store) targetStamp() string {
	targets, _ := s.targets()
	var b strings.Builder
	for _, t := range targets {
		b.WriteString(t.SHA)
	}
	return b.String()
}

func (w *webServer) apiOps(rw http.ResponseWriter, req *http.Request) {
	mode := req.URL.Query().Get("mode")
	if mode != targetRelease {
		mode = targetMaster
	}
	page := w.pages.get(w.db.doneStamp()+"|"+w.db.targetStamp(), "ops|"+mode, func() any { return w.buildOps(mode) })
	w.writeJSON(rw, page)
}
