package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// subjectDynSSZ is the library whose branches are measured. A series of
// measurements is identified by subject and branch; other libraries can
// become subjects of their own later.
const subjectDynSSZ = "dynamic-ssz"

// trendCommits is how many of the newest commits of a branch a trend is
// fitted over.
const trendCommits = 20

// branchInfo is a branch with finished measurements.
type branchInfo struct {
	Name    string
	PR      int
	Commits int
}

// branches lists the branches with finished commit jobs, the main branch
// first, then the newest measured.
func (s *store) branches(main string) ([]branchInfo, error) {
	rows, err := s.db.Query(`SELECT branch, max(pr), count(DISTINCT head_sha), max(id) FROM jobs
		WHERE kind = ? AND state = ? AND branch != '' AND branch != '-' GROUP BY branch ORDER BY (branch = ?) DESC, max(id) DESC`, kindCommit, stateDone, main)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []branchInfo{}
	for rows.Next() {
		var b branchInfo
		var last int64
		if err := rows.Scan(&b.Name, &b.PR, &b.Commits, &last); err != nil {
			return nil, err
		}
		if b.Name == main {
			b.PR = 0
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// branchSeries returns the measured commits of a branch, oldest first: per
// head commit its newest finished job (which carries the pooled reruns),
// in the order the commits were first queued, which is commit order. Only
// commits measured with the harness of the newest one are in the series,
// since values of different harness versions are not comparable; at most
// limit commits, the newest.
func (s *store) branchSeries(branch string, limit int) ([]*job, error) {
	rows, err := s.db.Query(`SELECT max(id) FROM jobs WHERE kind = ? AND state = ? AND branch = ? GROUP BY head_sha ORDER BY min(id)`, kindCommit, stateDone, branch)
	if err != nil {
		return nil, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var series []*job
	for _, id := range ids {
		j, err := s.getJob(id)
		if err != nil {
			return nil, err
		}
		series = append(series, j)
	}
	if n := len(series); n > 0 {
		harness := series[n-1].Harness
		kept := series[:0]
		for _, j := range series {
			if j.Harness == harness {
				kept = append(kept, j)
			}
		}
		series = kept
	}
	if len(series) > limit {
		series = series[len(series)-limit:]
	}
	return series, nil
}

// resultsOfJobs returns the stored results of the jobs, of one operation
// when object and op are given.
func (s *store) resultsOfJobs(ids []int64, object, op string) ([]result, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+2)
	for _, id := range ids {
		args = append(args, id)
	}
	cond := ""
	if object != "" {
		cond = " AND object = ? AND op = ?"
		args = append(args, object, op)
	}
	return scanResults(s.db.Query(`SELECT `+resultColumns+` FROM results WHERE job_id IN (`+marks+`)`+cond, args...))
}

// doneStamp changes whenever a job finishes.
func (s *store) doneStamp() string {
	var n, last int64
	_ = s.db.QueryRow(`SELECT count(*), coalesce(max(finished), 0) FROM jobs WHERE state = ?`, stateDone).Scan(&n, &last)
	return fmt.Sprintf("%d/%d", n, last)
}

// trend is the change along a series of values, oldest first: the change
// of the line fitted through them from the first to the last point, in
// percent of its start. Fewer than three points have no trend.
type trend struct {
	Pct float64
	N   int
}

func seriesTrend(values []float64) trend {
	n := len(values)
	if n < 3 {
		return trend{N: n}
	}
	xs := make([]float64, n)
	for i := range xs {
		xs[i] = float64(i)
	}
	mx, my := mean(xs), mean(values)
	sxx, sxy := 0.0, 0.0
	for i := range xs {
		sxx += (xs[i] - mx) * (xs[i] - mx)
		sxy += (xs[i] - mx) * (values[i] - my)
	}
	slope := sxy / sxx
	start := my - slope*mx
	if start <= 0 {
		return trend{N: n}
	}
	return trend{Pct: slope * float64(n-1) / start * 100, N: n}
}

// metricNames are the metrics of a result by the name the UI reads them
// under.
var metricNames = []string{"Ns", "Cycles", "Instrs", "Bytes", "Allocs"}

func metricsOf(r *result) map[string]*metric {
	return map[string]*metric{"Ns": &r.Ns, "Cycles": &r.Cycles, "Instrs": &r.Instrs, "Bytes": &r.Bytes, "Allocs": &r.Allocs}
}

// seriesTrends fits, per metric, the head values of one engine's results
// along a series (oldest first; nil where a commit has no result). With
// leadBase the base value of the first result opens the series: the state
// the branch started from.
func seriesTrends(rs []*result, leadBase bool) map[string]trend {
	out := map[string]trend{}
	for _, name := range metricNames {
		var vals []float64
		for _, r := range rs {
			if r == nil {
				continue
			}
			m := metricsOf(r)[name]
			if leadBase && len(vals) == 0 && m.Base > 0 {
				vals = append(vals, m.Base)
			}
			if m.Head > 0 {
				vals = append(vals, m.Head)
			}
		}
		out[name] = seriesTrend(vals)
	}
	return out
}

// opsMetric is what the operations page shows of a metric.
type opsMetric struct {
	Base, Head, Delta float64
	PMed, PSpread     float64
	PAgree, PN        float64
	DMin, DMax        float64
	Trend             float64 // change along the series, percent
	TrendN            int     // points the trend is fitted over (0: none)
	CVBase, CVHead    float64
}

// opsCell is one engine's or library's newest result of an operation.
type opsCell struct {
	JobID                             int64
	Engine, Object, Op                string
	Baseline                          bool
	Ns, Cycles, Instrs, Bytes, Allocs opsMetric
}

type opsRow struct {
	Object, Op string
	Cells      map[string]*opsCell
}

// opsPage is the operations page of one branch.
type opsPage struct {
	Subject  string
	Branch   string
	Main     bool // the main branch: values and trend; else also the change against the base
	PR       int
	Branches []branchInfo
	Commits  int  // commits of the branch in the series
	Head     *job // newest measured commit of the branch
	Engines  []string
	Rows     []opsRow
	Noise    map[string]float64 // engine/object/op -> noise floor
}

func newOpsCell(r *result, trends map[string]trend) *opsCell {
	c := &opsCell{JobID: r.JobID, Engine: r.Engine, Object: r.Object, Op: r.Op, Baseline: r.Baseline}
	fill := func(dst *opsMetric, m metric, t trend) {
		*dst = opsMetric{Base: m.Base, Head: m.Head, Delta: m.Delta, PMed: m.PMed, PSpread: m.PSpread, PAgree: m.PAgree, PN: m.PN,
			DMin: m.DMin, DMax: m.DMax, CVBase: m.CVBase, CVHead: m.CVHead}
		if t.N >= 3 {
			dst.Trend, dst.TrendN = t.Pct, t.N
		}
	}
	fill(&c.Ns, r.Ns, trends["Ns"])
	fill(&c.Cycles, r.Cycles, trends["Cycles"])
	fill(&c.Instrs, r.Instrs, trends["Instrs"])
	fill(&c.Bytes, r.Bytes, trends["Bytes"])
	fill(&c.Allocs, r.Allocs, trends["Allocs"])
	return c
}

// seriesIndex arranges the results of a series: leaf key -> engine ->
// result per commit of the series (nil where missing).
func seriesIndex(series []*job, results []result) map[string]map[string][]*result {
	pos := map[int64]int{}
	for i, j := range series {
		pos[j.ID] = i
	}
	idx := map[string]map[string][]*result{}
	for i := range results {
		r := &results[i]
		if r.Baseline {
			continue
		}
		leafKey := r.Object + "/" + r.Op
		if idx[leafKey] == nil {
			idx[leafKey] = map[string][]*result{}
		}
		if idx[leafKey][r.Engine] == nil {
			idx[leafKey][r.Engine] = make([]*result, len(series))
		}
		idx[leafKey][r.Engine][pos[r.JobID]] = r
	}
	return idx
}

func newest(rs []*result) *result {
	for i := len(rs) - 1; i >= 0; i-- {
		if rs[i] != nil {
			return rs[i]
		}
	}
	return nil
}

func (w *webServer) buildOps(branch string) (*opsPage, error) {
	main := w.cfg.mainBranch
	p := &opsPage{Subject: subjectDynSSZ, Branch: branch, Main: branch == main, Rows: []opsRow{}, Engines: []string{}}
	var err error
	if p.Branches, err = w.db.branches(main); err != nil {
		return nil, err
	}
	for _, b := range p.Branches {
		if b.Name == branch {
			p.PR = b.PR
		}
	}
	series, err := w.db.branchSeries(branch, trendCommits)
	if err != nil {
		return nil, err
	}
	p.Commits = len(series)
	ids := make([]int64, len(series))
	for i, j := range series {
		ids[i] = j.ID
	}
	if len(series) > 0 {
		p.Head = series[len(series)-1]
	}
	results, err := w.db.resultsOfJobs(ids, "", "")
	if err != nil {
		return nil, err
	}
	idx := seriesIndex(series, results)
	rows := map[string]*opsRow{}
	engines := map[string]bool{}
	row := func(object, op string) *opsRow {
		k := object + "/" + op
		if rows[k] == nil {
			rows[k] = &opsRow{Object: object, Op: op, Cells: map[string]*opsCell{}}
		}
		return rows[k]
	}
	for _, byEngine := range idx {
		for engine, rs := range byEngine {
			last := newest(rs)
			if last == nil {
				continue
			}
			engines[engine] = true
			row(last.Object, last.Op).Cells[engine] = newOpsCell(last, seriesTrends(rs, !p.Main))
		}
	}
	refs, _ := w.baselineResults(false)
	for i := range refs {
		r := &refs[i]
		engines[r.Engine] = true
		row(r.Object, r.Op).Cells[r.Engine] = newOpsCell(r, nil)
	}
	p.Engines = sortedBy(engines, engineOrder)
	for _, r := range rows {
		p.Rows = append(p.Rows, *r)
	}
	sort.Slice(p.Rows, func(a, b int) bool {
		return leafLess(p.Rows[a].Object, p.Rows[a].Op, "", p.Rows[b].Object, p.Rows[b].Op, "")
	})
	p.Noise = w.noiseFloor().PerLeaf
	return p, nil
}

// pageCache keeps built pages until a job finishes.
type pageCache struct {
	mu    sync.Mutex
	stamp string
	pages map[string]any
}

func (c *pageCache) get(stamp, key string, build func() (any, error)) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stamp != stamp || c.pages == nil {
		c.stamp, c.pages = stamp, map[string]any{}
	}
	if v, ok := c.pages[key]; ok {
		return v, nil
	}
	v, err := build()
	if err != nil {
		return nil, err
	}
	c.pages[key] = v
	return v, nil
}

func (w *webServer) apiOps(rw http.ResponseWriter, req *http.Request) {
	branch := req.URL.Query().Get("branch")
	if branch == "" {
		branch = w.cfg.mainBranch
	}
	page, err := w.pages.get(w.db.doneStamp(), "ops|"+subjectDynSSZ+"|"+branch, func() (any, error) { return w.buildOps(branch) })
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	w.writeJSON(rw, page)
}
