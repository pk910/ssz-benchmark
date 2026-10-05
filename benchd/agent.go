package main

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The pages as text, for an agent that is given a link and has to gather
// what was measured without running the page's script:
//
//	/llms.txt                      what the site is, how its links map to
//	                               text and JSON, how to read the numbers
//	/job/<id>.md                   a job: what it compared and every result
//	/commit/<library>/<sha>.md     everything measured of a commit against a
//	                               base (?base=<sha> or none)
//	/repo/<library>.md             the commits of a library's main branch
//	                               with the change at each (?page=N)

// origin is the address the request reached the site under.
func origin(req *http.Request) string {
	scheme := "https"
	host, _, err := net.SplitHostPort(req.Host)
	if err != nil {
		host = req.Host
	}
	if req.TLS == nil && req.Header.Get("X-Forwarded-Proto") != "https" && (host == "localhost" || net.ParseIP(host) != nil) {
		scheme = "http"
	}
	return scheme + "://" + req.Host
}

func writeText(rw http.ResponseWriter, text string) {
	rw.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	rw.Header().Set("Cache-Control", "no-cache")
	_, _ = rw.Write([]byte(text))
}

const agentGuide = `# dynamic-ssz benchmarks

Continuous benchmarks of SSZ libraries for Go (dynamic-ssz and others) on a
dedicated machine. Every commit of a library's main branch, every pull
request head and every release is measured on real mainnet payloads.

The pages of this site are drawn by a script; a link like
%[1]s/#/job/12 returns only the script's shell. Use the text and JSON
addresses below instead. Text is Markdown; JSON carries every number.

## From a link to its data

| link in the browser | text | JSON |
|---|---|---|
| /#/job/<id> | /job/<id>.md | /api/job/<id> |
| /#/commit/<library>/<sha> (optional ?base=<sha>) | /commit/<library>/<sha>.md | /api/commit/<library>/<sha> |
| /#/repo/<library> (optional ?page=N) | /repo/<library>.md | /api/repo/<library> |
| /#/repos | /repos.md | /api/repos |
| /#/job/<id>/leaf/<Engine>/<Object>/<Op> | - | /api/job/<id>/leaf/<Engine>/<Object>/<Op> |
| /#/jobs | - | /api/jobs?limit=100&summaries=1 |
| /#/pr/<number> | - | /api/pr/<number> |
| /#/op/<Object>/<Op> | - | /api/op/<Object>/<Op> |
| /#/noise | - | /api/noise |

More: /api/job/<id>/samples (every single run of a job), /raw/<id>/<file>
(a job's log files, named on its text page), /api/status.

## What is measured

- A benchmark is Engine / Object / Operation. Objects are payload types
  (FuluState, FuluBlock, FuluBlocks, GloasState, GloasBlock, GloasBlocks,
  GloasEnvelope); operations are Unmarshal, Marshal, HashTreeRoot and
  their variants. Engines of dynamic-ssz: Codegen (generated code) and
  Reflection. "HashTreeRoot (async)" is the hash tree root with background
  workers, an operation of each engine (the harness runs it as the engines
  CodegenAsync / ReflectionAsync, which is how the JSON names it).
- A job measures a head commit, usually against a base commit. Each side
  is built once per layout seed (the linker shuffles the functions), and a
  pass measures both sides built with the same seed. Numbers per call:
  time (ns), cycles and instructions of the measured thread (hardware
  counters, user mode), allocated bytes and allocations.
- The delta of an operation is the median over the passes of
  (head - base) / base. Cycles are the primary metric (time for the async
  hashing, whose work is spread over threads).

## Reading a result

- "changed" means: the delta lies beyond the band, and at least three
  quarters of the passes point the same way. The band is the larger of
  the operation's noise floor on this machine and twice the spread of the
  passes divided by the root of their number. Anything inside the band is
  not a finding.
- "work" tells what kind of change it is. "code": the instructions per
  call moved by 0.5%% or more, so the code does different work (look at the
  diff for more or fewer instructions on that path). "same work": the
  instructions did not move but the cycles did; that comes from how the
  same code executes (function placement, cache, branch prediction) and is
  often a layout effect and not a regression of the change itself.
- Allocations and bytes per call are exact; a change there is always real.
- A single job has four passes. The commit page pools every job that
  measured the commit and is the better source for a verdict.

## Finding the cause of a regression

1. /repo/<library>.md: find the commit where an engine or operation moved.
2. /commit/<library>/<sha>.md: the operations that changed against the
   commit's base, pooled over all its jobs, with the kind of change.
3. /job/<id>.md of a job that measured it: the same for one job, and its
   log files.
4. /api/job/<id>/leaf/<Engine>/<Object>/<Op>: every run of one operation
   on both sides, per seed.
5. The commit itself in the library's repository (linked on each page).
`

func (w *webServer) agentGuide(rw http.ResponseWriter, req *http.Request) {
	writeText(rw, fmt.Sprintf(agentGuide, origin(req)))
}

// mdDelta is the delta of a metric as the pages show it.
func mdDelta(m metric) float64 {
	if m.PN > 0 {
		return m.PMed
	}
	return m.Delta
}

func mdPct(v float64) string { return fmt.Sprintf("%+.2f%%", v) }

// mdResults writes the results of a comparison: a summary per engine, the
// operations that changed, then all of them; or the plain values where
// nothing was compared.
func mdResults(sb *strings.Builder, results []result, nf noiseFloor) {
	var own []result
	for _, r := range results {
		if !r.Other {
			own = append(own, r)
		}
	}
	sort.SliceStable(own, func(a, b int) bool {
		return leafLess(own[a].Object, own[a].Op, own[a].Engine, own[b].Object, own[b].Op, own[b].Engine)
	})
	var compared, plain []result
	for _, r := range own {
		if m, _, _ := checkMetric(r); m.Base > 0 && m.Head > 0 {
			compared = append(compared, r)
		} else if r.Ns.Head > 0 {
			plain = append(plain, r)
		}
	}
	floor := func(r result) float64 { return max(0.5, nf.PerLeaf[joinKey(r.Engine, r.Object, r.Op)]) }
	row := func(r result) string {
		m, name, fmtv := checkMetric(r)
		verdict := "unchanged"
		if m.changed(floor(r)) {
			verdict = "slower"
			if mdDelta(m) < 0 {
				verdict = "faster"
			}
		}
		work := "-"
		if changed, ok := r.workChanged(); ok {
			work = "same work"
			if changed {
				work = "code"
			}
		}
		passes := "-"
		if m.PN > 0 {
			passes = fmt.Sprintf("%.0f/%.0f", m.PAgree, m.PN)
		}
		instrs := "-"
		if r.Instrs.Base > 0 && r.Instrs.Head > 0 {
			instrs = mdPct(mdDelta(r.Instrs))
		}
		return fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s | ±%.2f%% | %s | %s | %s | %s | %s | %s | %s |\n",
			baseEngine(r.Engine), r.Object, opLabel(r.Engine, r.Op), name, fmtv(m.Base), fmtv(m.Head), mdPct(mdDelta(m)), m.band(floor(r)), passes, verdict,
			instrs, work, mdPct(mdDelta(r.Ns)), mdAbs(r.Bytes), mdAbs(r.Allocs))
	}
	const header = "| engine | object | operation | metric | base | head | delta | band | passes agreeing | verdict | instructions | work | time | bytes/call | allocs/call |\n|---|---|---|---|---:|---:|---:|---:|---:|---|---:|---|---:|---:|---:|\n"
	if len(compared) > 0 {
		type sum struct {
			logs           []float64
			slower, faster int
		}
		engines := map[string]*sum{}
		var names []string
		var changed []result
		for _, r := range compared {
			m, _, _ := checkMetric(r)
			e := engines[baseEngine(r.Engine)]
			if e == nil {
				e = &sum{}
				engines[baseEngine(r.Engine)] = e
				names = append(names, baseEngine(r.Engine))
			}
			e.logs = append(e.logs, math.Log(1+mdDelta(m)/100))
			if m.changed(floor(r)) {
				changed = append(changed, r)
				if mdDelta(m) > 0 {
					e.slower++
				} else {
					e.faster++
				}
			}
		}
		sort.Slice(names, func(a, b int) bool { return rank(engineOrder, names[a]) < rank(engineOrder, names[b]) })
		sb.WriteString("## Per engine\n\n| engine | geomean delta | slower | faster | operations |\n|---|---:|---:|---:|---:|\n")
		for _, name := range names {
			e := engines[name]
			fmt.Fprintf(sb, "| %s | %s | %d | %d | %d |\n", name, mdPct((math.Exp(mean(e.logs))-1)*100), e.slower, e.faster, len(e.logs))
		}
		sort.SliceStable(changed, func(a, b int) bool {
			ma, _, _ := checkMetric(changed[a])
			mb, _, _ := checkMetric(changed[b])
			return mdDelta(ma) > mdDelta(mb)
		})
		fmt.Fprintf(sb, "\n## Changed operations (%d of %d), slowest first\n\n", len(changed), len(compared))
		if len(changed) == 0 {
			sb.WriteString("None: every delta lies inside its band.\n")
		} else {
			sb.WriteString(header)
			for _, r := range changed {
				sb.WriteString(row(r))
			}
		}
		sb.WriteString("\n## All compared operations\n\n" + header)
		for _, r := range compared {
			sb.WriteString(row(r))
		}
		sb.WriteString("\nbytes/call and allocs/call are given as base → head where they differ.\n")
	}
	if len(plain) > 0 {
		sb.WriteString("\n## Values without a comparison\n\n| engine | object | operation | time | cycles | instructions | bytes/call | allocs/call |\n|---|---|---|---:|---:|---:|---:|---:|\n")
		for _, r := range plain {
			fmt.Fprintf(sb, "| %s | %s | %s | %s | %s | %s | %.0f | %.0f |\n", baseEngine(r.Engine), r.Object, opLabel(r.Engine, r.Op), fmtNs(r.Ns.Head), fmtCount(r.Cycles.Head), fmtCount(r.Instrs.Head), r.Bytes.Head, r.Allocs.Head)
		}
	}
	if len(compared) == 0 && len(plain) == 0 {
		sb.WriteString("\nNo results.\n")
	}
}

// mdAbs writes an exact count per call, with the base where it differs.
func mdAbs(m metric) string {
	if m.Base > 0 && math.Round(m.Base) != math.Round(m.Head) {
		return fmt.Sprintf("%.0f → %.0f", m.Base, m.Head)
	}
	return fmt.Sprintf("%.0f", m.Head)
}

func mdCommit(o, subject, sha string) string {
	if sha == "" {
		return "none"
	}
	link := fmt.Sprintf("[`%s`](%s/commit/%s/%s.md)", shortSHA(sha), o, subject, sha)
	if repo := repoURL(subject); repo != "" {
		link += fmt.Sprintf(" ([source](%s/commit/%s))", repo, sha)
	}
	return link
}

func mdTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return t.UTC().Format("2006-01-02 15:04 UTC")
}

func (w *webServer) agentJob(rw http.ResponseWriter, req *http.Request) {
	id, err := strconv.ParseInt(strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/job/"), ".md"), 10, 64)
	if err != nil {
		http.NotFound(rw, req)
		return
	}
	j, err := w.db.getJob(id)
	if err != nil || j == nil {
		http.NotFound(rw, req)
		return
	}
	o := origin(req)
	var sb strings.Builder
	fmt.Fprintf(&sb, "# Job #%d: %s %s, %s\n\n", j.ID, j.Subject, j.Kind, j.State)
	fmt.Fprintf(&sb, "- library: %s (%s)\n- head: %s %s\n- base: %s %s\n- branch: %s\n", j.Subject, repoURL(j.Subject), mdCommit(o, j.Subject, j.HeadSHA), j.HeadDesc, mdCommit(o, j.Subject, j.BaseSHA), j.BaseRef, j.Branch)
	if j.PR != 0 {
		fmt.Fprintf(&sb, "- pull request: #%d (%s/pull/%d), history %s/api/pr/%d\n", j.PR, repoURL(j.Subject), j.PR, o, j.PR)
	}
	fmt.Fprintf(&sb, "- passes: %d, took %.0f s, finished %s\n- harness version %s, %s, runner %s\n", j.Passes, j.Seconds, mdTime(j.Finished), j.Harness, j.GoVersion, j.Runner)
	if j.Note != "" {
		fmt.Fprintf(&sb, "- note: %s\n", j.Note)
	}
	if j.Error != "" {
		fmt.Fprintf(&sb, "- error:\n\n```\n%s\n```\n", j.Error)
	}
	sb.WriteString("\nHow to read the numbers: " + o + "/llms.txt. A job has few passes; the commit page of the head pools all its jobs.\n\n")
	var results []result
	if samples, _ := w.db.samplesFor(j.ID); len(samples) > 0 {
		results = summarize(samples)
	} else {
		results, _ = w.db.resultsFor(j.ID)
	}
	mdResults(&sb, results, w.noiseFloor())
	sb.WriteString("\n## More data\n\n")
	fmt.Fprintf(&sb, "- everything as JSON: %s/api/job/%d\n- every single run: %s/api/job/%d/samples\n- every run of one operation: %s/api/job/%d/leaf/<Engine>/<Object>/<Op>\n", o, j.ID, o, j.ID, o, j.ID)
	if files := jobFiles(filepath.Join(w.cfg.dataDir, "jobs", strconv.FormatInt(j.ID, 10))); len(files) > 0 {
		sb.WriteString("- log files (job.log is the runner's log, the others the raw benchmark output per side, pass, seed and package):\n")
		for _, f := range files {
			fmt.Fprintf(&sb, "  - %s/raw/%d/%s\n", o, j.ID, f)
		}
	}
	writeText(rw, sb.String())
}

func (w *webServer) agentCommit(rw http.ResponseWriter, req *http.Request) {
	parts := strings.SplitN(strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/commit/"), ".md"), "/", 2)
	if len(parts) != 2 || subjectByName(parts[0]) == nil || parts[1] == "" {
		http.NotFound(rw, req)
		return
	}
	p, err := w.commitView(subjectByName(parts[0]), parts[1], req.URL.Query().Get("base"))
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	o := origin(req)
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s commit %s\n\n%s\n\n", p.Subject, shortSHA(p.SHA), p.Desc)
	fmt.Fprintf(&sb, "- commit: `%s` (%s/commit/%s)\n- harness version %s\n", p.SHA, p.Repo, p.SHA, p.Harness)
	list := func(jobs []*job) string {
		var ids []string
		for _, j := range jobs {
			ids = append(ids, fmt.Sprintf("[#%d](%s/job/%d.md) %s", j.ID, o, j.ID, j.State))
		}
		if len(ids) == 0 {
			return "none"
		}
		return strings.Join(ids, ", ")
	}
	fmt.Fprintf(&sb, "- jobs that measured it: %s\n", list(p.Jobs))
	switch {
	case p.Base == "":
		sb.WriteString("- compared with: nothing\n")
	case p.NoBase:
		fmt.Fprintf(&sb, "- compared with: %s, which has no runs with this harness version (nothing to compare)\n", mdCommit(o, p.Subject, p.Base))
	default:
		fmt.Fprintf(&sb, "- compared with: %s, measured by %s\n", mdCommit(o, p.Subject, p.Base), list(p.BaseJobs))
	}
	if len(p.Bases) > 0 {
		sb.WriteString("- other bases (append ?base=<sha>, or ?base=none):\n")
		for _, b := range p.Bases {
			fmt.Fprintf(&sb, "  - `%s` %s, %d jobs\n", b.SHA, b.Label, b.Runs)
		}
	}
	sb.WriteString("\nThe values are pooled over every job that measured the commit; head and base runs are paired by layout seed. How to read the numbers: " + o + "/llms.txt.\n\n")
	mdResults(&sb, p.Results, p.Noise)
	fmt.Fprintf(&sb, "\n## More data\n\n- everything as JSON: %s/api/commit/%s/%s\n- the library's commits: %s/repo/%s.md\n", o, p.Subject, p.SHA, o, p.Subject)
	writeText(rw, sb.String())
}

func (w *webServer) agentRepo(rw http.ResponseWriter, req *http.Request) {
	name := strings.TrimSuffix(strings.TrimPrefix(req.URL.Path, "/repo/"), ".md")
	o := origin(req)
	var sb strings.Builder
	if req.URL.Path == "/repos.md" {
		sb.WriteString("# Measured libraries\n\n")
		for i := range subjects {
			sub := &subjects[i]
			fmt.Fprintf(&sb, "- [%s](%s/repo/%s.md): %s\n", sub.Name, o, sub.Name, sub.Repo)
		}
		writeText(rw, sb.String())
		return
	}
	sub := subjectByName(name)
	if sub == nil {
		http.NotFound(rw, req)
		return
	}
	page, _ := strconv.Atoi(req.URL.Query().Get("page"))
	page = max(page, 1)
	v, err := w.repoView(sub, (page-1)*repoPage, repoPage, false)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	fmt.Fprintf(&sb, "# %s\n\n- repository: %s\n", v.Name, v.Repo)
	for _, t := range v.Targets {
		fmt.Fprintf(&sb, "- %s: %s at %s\n", t.Name, t.Label, mdCommit(o, v.Name, t.SHA))
	}
	fmt.Fprintf(&sb, "- %d commits on %s, %d measured; page %d of %d (?page=N)\n\n", v.Total, v.Branch, v.Measured, page, max(1, (v.Total+repoPage-1)/repoPage))
	sb.WriteString("Newest first. The change of a measured commit is against the measured commit before it: per engine the geomean over its operations, the async hashing among them (cycles, time for the async hashing) and how many operations moved by 1% or more (faster/slower). Per-operation detail is on the commit's page.\n\n")
	sb.WriteString("| commit | committed | tags | description | change against the measured commit before |\n|---|---|---|---|---|\n")
	for _, c := range v.Commits {
		change := "not measured"
		switch {
		case c.Compared:
			var parts []string
			for _, e := range c.Engines {
				parts = append(parts, fmt.Sprintf("%s %s (%d faster, %d slower of %d)", e.Engine, mdPct(e.Geomean), e.Faster, e.Slower, e.N))
			}
			change = strings.Join(parts, "; ")
			if c.Skipped > 0 {
				change += fmt.Sprintf(" [against %s, %d unmeasured between]", shortSHA(c.Against), c.Skipped)
			}
		case c.Runs > 0:
			change = "measured, no measured commit before it to compare with"
		case c.State != "":
			change = c.State
		}
		committed := "-"
		if c.Committed > 0 {
			committed = time.Unix(c.Committed, 0).UTC().Format("2006-01-02")
		}
		fmt.Fprintf(&sb, "| %s | %s | %s | %s | %s |\n", mdCommit(o, v.Name, c.SHA), committed, strings.Join(c.Tags, " "), strings.ReplaceAll(c.Desc, "|", "\\|"), change)
	}
	fmt.Fprintf(&sb, "\nEverything as JSON, with the values per operation: %s/api/repo/%s?page=%d\n", o, v.Name, page)
	writeText(rw, sb.String())
}
