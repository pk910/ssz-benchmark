package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTDistribution(t *testing.T) {
	if v := tCDF(1.96, 1e6); math.Abs(v-0.975) > 1e-3 {
		t.Fatalf("tCDF(1.96, inf) = %v", v)
	}
	if q := tQuantile(0.975, 10); math.Abs(q-2.228) > 1e-3 {
		t.Fatalf("tQuantile(0.975, 10) = %v", q)
	}
	if q := tQuantile(0.975, 3); math.Abs(q-3.182) > 1e-3 {
		t.Fatalf("tQuantile(0.975, 3) = %v", q)
	}
}

func TestWelch(t *testing.T) {
	a := []float64{100, 101, 99, 100, 102, 98}
	b := []float64{110, 111, 109, 110, 112, 108}
	diff, lo, hi, p := welch(a, b)
	if math.Abs(diff-10) > 1e-9 || lo > 10 || hi < 10 || p > 1e-6 {
		t.Fatalf("welch: diff %v [%v %v] p %v", diff, lo, hi, p)
	}
	_, lo, hi, p = welch(a, a)
	if lo > 0 || hi < 0 || p < 0.99 {
		t.Fatalf("welch same: [%v %v] p %v", lo, hi, p)
	}
}

func TestParseBenchOutput(t *testing.T) {
	out := []byte("goos: linux\nBenchmarkReal/Codegen/State/Unmarshal-1   \t 3\t 120106849 ns/op\t2898.31 MB/s\t463417792 B/op\t 5014460 allocs/op\nPASS\n")
	lines := parseBenchOutput(out)
	if len(lines) != 1 || lines[0].name != "BenchmarkReal/Codegen/State/Unmarshal" || lines[0].iters != 3 || lines[0].ns != 120106849 || lines[0].allocs != 5014460 || lines[0].bytes != 463417792 {
		t.Fatalf("parsed %+v", lines)
	}
}

func TestBenchRegex(t *testing.T) {
	if got := benchRegex("BenchmarkReal/Codegen/State/Unmarshal"); got != "^BenchmarkReal$/^Codegen$/^State$/^Unmarshal$" {
		t.Fatal(got)
	}
}

func syntheticSamples(jobID int64) []sample {
	var samples []sample
	for pass := 0; pass < 4; pass++ {
		for _, obj := range objectOrder {
			for _, op := range opOrder {
				for _, eng := range engineOrder {
					base := 1000.0 + float64(pass)*3
					head := base * 0.9
					if !ownEngine(eng) {
						samples = append(samples, sample{JobID: jobID, Side: "head", Engine: eng, Object: obj, Op: op, Seed: "101", Pass: pass, Iters: 3, Ns: 800 + float64(pass), Bytes: 64, Allocs: 2})
						continue
					}
					samples = append(samples,
						sample{JobID: jobID, Side: "base", Engine: eng, Object: obj, Op: op, Seed: "101", Pass: pass, Iters: 3, Ns: base, Bytes: 128, Allocs: 4},
						sample{JobID: jobID, Side: "head", Engine: eng, Object: obj, Op: op, Seed: "101", Pass: pass, Iters: 3, Ns: head, Bytes: 96, Allocs: 3, Steal: pass})
				}
			}
		}
	}
	return samples
}

// ownEngine reports whether the engine is one of dynamic-ssz's own, as
// opposed to a reference library measured on the head side only.
func ownEngine(name string) bool {
	switch name {
	case "Codegen", "Reflection", "CodegenAsync", "ReflectionAsync":
		return true
	}
	return false
}

func TestSummarize(t *testing.T) {
	results := summarize(syntheticSamples(1))
	if len(results) != len(objectOrder)*len(opOrder)*len(engineOrder) {
		t.Fatalf("%d results", len(results))
	}
	for _, r := range results {
		if !ownEngine(r.Engine) {
			if !r.Baseline || r.N != 4 || r.Ns.Base != 0 {
				t.Fatalf("baseline result %+v", r)
			}
			continue
		}
		if r.Baseline || r.N != 4 || r.Iters != 3 || math.Abs(r.Ns.Delta+10) > 1e-6 || r.Ns.Hi >= 0 || r.Steal != 6 {
			t.Fatalf("result %+v", r)
		}
		if math.Abs(r.Bytes.Delta+25) > 1e-6 || math.Abs(r.Allocs.Delta+25) > 1e-6 {
			t.Fatalf("memory deltas %+v %+v", r.Bytes, r.Allocs)
		}
	}
	if gm := geomeanDelta(results); math.Abs(gm+10) > 1e-6 {
		t.Fatalf("geomean %v", gm)
	}
	// Ordering: State first, operations in order, engines in order.
	if results[0].Object != "FuluState" || results[0].Op != "Unmarshal" || results[0].Engine != "Codegen" || results[1].Engine != "Reflection" || results[len(engineOrder)-1].Engine != engineOrder[len(engineOrder)-1] {
		t.Fatalf("order %+v %+v %+v", results[0], results[1], results[2])
	}
}

// TestPagesRender stores a synthetic job and renders every page and API
// endpoint against it.
func TestPagesRender(t *testing.T) {
	dir := t.TempDir()
	db, err := openDB(filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config{dataDir: dir, name: "ctl", packages: []string{"fulu", "gloas"}, seeds: []string{"101"}, target: time.Minute, benchTime: "300ms", minIters: 2, cpus: "2", gomaxprocs: "1", github: "x/y", mainBranch: "master", baselines: []string{"FastSSZ"}}
	git := &gitRepo{dir: filepath.Join(dir, "none.git")}
	sched := &scheduler{cfg: cfg, db: db, git: git, prs: newPRCache("x/y", ""), ready: make(chan struct{})}
	local := &localStore{db: db, sched: sched, local: "ctl", rule: qualifyRule{maxP95: 1.5, maxRatio: 0.15, maxSpread: 8, maxRetries: 3}}
	rn := &runner{cfg: cfg, store: local, git: git, sched: sched, name: "ctl"}
	rn.setPhase(nil, "")
	w := &webServer{cfg: cfg, db: db, sched: sched, runners: &runnerAPI{store: local, token: "t"}}

	for i, kind := range []string{kindCommit, kindNoise, kindRelease, kindCommit} {
		j := &job{Kind: kind, Branch: "master", HeadSHA: strings.Repeat("a", 39) + string(rune('0'+i)), BaseSHA: strings.Repeat("b", 40), BaseRef: "master^", HeadDesc: "head", BaseDesc: "base"}
		if kind == kindNoise {
			j.BaseSHA = j.HeadSHA
		}
		if err := db.insertJob(j); err != nil {
			t.Fatal(err)
		}
		if _, err := db.claimJob(j, "ctl"); err != nil {
			t.Fatal(err)
		}
		if err := db.startJob(j.ID, "go1.27", "deadbeef", ""); err != nil {
			t.Fatal(err)
		}
		samples := syntheticSamples(j.ID)
		if err := db.insertSamples(samples); err != nil {
			t.Fatal(err)
		}
		if err := db.replaceResults(j.ID, summarize(samples)); err != nil {
			t.Fatal(err)
		}
		if err := db.finishJob(j.ID, stateDone, 4, "", 600); err != nil {
			t.Fatal(err)
		}
	}
	h, err := w.handler()
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"/", "/ui/app.js", "/ui/app.css", "/api/status", "/api/dashboard", "/api/jobs", "/api/jobs?kind=noise", "/api/job/1", "/api/job/2", "/api/job/1/samples",
		"/api/job/1/leaf/Codegen/FuluState/Unmarshal", "/api/job/1/leaf/FastSSZ/FuluBlock/Marshal", "/api/ops", "/api/op/FuluState/Unmarshal", "/api/op/GloasBlocks/GetTree",
		"/api/pr/7", "/api/compare", "/api/compare?a=" + strings.Repeat("b", 40) + "&b=" + strings.Repeat("a", 39) + "0", "/api/noise", "/api/runners"}
	for _, p := range paths {
		req := httptest.NewRequest("GET", p, nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", p, rec.Code, rec.Body.String())
		}
		if strings.HasPrefix(p, "/api") {
			var v any
			if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
				t.Fatalf("%s: not json: %v", p, err)
			}
		}
	}
	nf := w.noiseFloor()
	own := 0
	for _, e := range engineOrder {
		if ownEngine(e) {
			own++
		}
	}
	if nf.Jobs != 1 || len(nf.Rows) != own*len(objectOrder)*len(opOrder) {
		t.Fatalf("noise floor %+v", nf.Jobs)
	}
}

// TestWriteDemoDB writes a database with synthetic but realistically scaled
// jobs for UI work when BENCHD_DEMO_DB names the path.
func TestWriteDemoDB(t *testing.T) {
	path := os.Getenv("BENCHD_DEMO_DB")
	if path == "" {
		t.Skip("BENCHD_DEMO_DB not set")
	}
	_ = os.Remove(path)
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(7, 11))
	scale := map[string]float64{"FuluState": 3e8, "FuluBlock": 1.5e6, "FuluBlocks": 2e6, "GloasState": 3.2e8, "GloasBlock": 1.6e6, "GloasBlocks": 1.2e6, "GloasEnvelope": 8e5}
	opScale := map[string]float64{"Unmarshal": 1, "UnmarshalReader": 1.1, "UnmarshalReaderUnknown": 1.5, "SizeSSZ": 0.0001, "Marshal": 0.4, "MarshalTo": 0.35, "MarshalWriter": 0.5, "HashTreeRoot": 3, "GetTree": 10}
	engScale := map[string]float64{"Codegen": 1, "Reflection": 2.6, "CodegenAsync": 0.4, "ReflectionAsync": 0.55, "FastSSZ": 0.8}
	kinds := []string{kindNoise, kindCommit, kindCommit, kindRelease, kindCommit, kindNoise, kindCommit}
	base := time.Now().Add(-6 * 24 * time.Hour)
	for i, kind := range kinds {
		head := fmt.Sprintf("%040x", rng.Uint64())
		baseSHA := fmt.Sprintf("%040x", rng.Uint64())
		branch := "master"
		pr := 0
		if kind == kindNoise {
			baseSHA = head
		}
		if i == 2 || i == 6 {
			branch, pr = "pk910/fix-something", 240+i
		}
		j := &job{Kind: kind, Branch: branch, HeadSHA: head, HeadDesc: head[:7] + " fix: a change (#" + strconv.Itoa(240+i) + ")", BaseSHA: baseSHA, BaseDesc: baseSHA[:7] + " earlier commit", BaseRef: "master^", PR: pr}
		if err := db.insertJob(j); err != nil {
			t.Fatal(err)
		}
		if _, err := db.claimJob(j, "pk-devbench"); err != nil {
			t.Fatal(err)
		}
		if err := db.startJob(j.ID, "go1.27.0 linux/amd64", "deadbeefcafef00d", ""); err != nil {
			t.Fatal(err)
		}
		var samples []sample
		effect := map[string]float64{}
		for pass := 0; pass < 4; pass++ {
			for _, obj := range objectOrder {
				for _, op := range opOrder {
					for _, eng := range engineOrder {
						if strings.HasSuffix(eng, "Async") && op != "HashTreeRoot" {
							continue
						}
						if eng == "FastSSZ" && (strings.HasPrefix(obj, "Gloas") || strings.Contains(op, "Reader") || op == "MarshalWriter") {
							continue
						}
						k := eng + "/" + obj + "/" + op
						if _, ok := effect[k]; !ok {
							effect[k] = 0
							if kind != kindNoise && rng.Float64() < 0.2 {
								effect[k] = (rng.Float64() - 0.6) * 12
							}
						}
						mean := scale[obj] * opScale[op] * engScale[eng]
						noise := func() float64 { return 1 + (rng.Float64()-0.5)*0.012 }
						bytes := mean * 1.3
						allocs := math.Round(mean / 60)
						iters := max(2, int(5e8/mean))
						if eng == "FastSSZ" {
							samples = append(samples, sample{JobID: j.ID, Side: "head", Engine: eng, Object: obj, Op: op, Seed: "101", Pass: pass, Iters: iters, Ns: mean * noise(), Bytes: bytes, Allocs: allocs, Cycles: mean * 3.5, Instrs: mean * 7})
							continue
						}
						samples = append(samples,
							sample{JobID: j.ID, Side: "base", Engine: eng, Object: obj, Op: op, Seed: "101", Pass: pass, Iters: iters, Ns: mean * noise(), Bytes: bytes, Allocs: allocs, Cycles: mean * 3.5 * noise(), Instrs: mean * 7},
							sample{JobID: j.ID, Side: "head", Engine: eng, Object: obj, Op: op, Seed: "101", Pass: pass, Iters: iters, Ns: mean * (1 + effect[k]/100) * noise(), Bytes: bytes * (1 + effect[k]/300), Allocs: math.Round(allocs * (1 + effect[k]/400)), Cycles: mean * 3.5 * (1 + effect[k]/100) * noise(), Instrs: mean * 7 * (1 + effect[k]/100), Steal: map[bool]int{true: 1, false: 0}[rng.Float64() < 0.02]})
					}
				}
			}
		}
		if err := db.insertSamples(samples); err != nil {
			t.Fatal(err)
		}
		if err := db.replaceResults(j.ID, summarize(samples)); err != nil {
			t.Fatal(err)
		}
		if _, err := db.db.Exec(`UPDATE jobs SET state = ?, finished = ?, passes = 4, seconds = 790 WHERE id = ?`, stateDone, base.Add(time.Duration(i)*20*time.Hour).Unix(), j.ID); err != nil {
			t.Fatal(err)
		}
	}
	// A running job with a few samples and two queued ones.
	j := &job{Kind: kindCommit, Branch: "master", HeadSHA: strings.Repeat("c", 40), HeadDesc: "ccccccc running change (#251)", BaseSHA: strings.Repeat("d", 40), BaseDesc: "ddddddd base", BaseRef: "master^", PR: 251}
	_ = db.insertJob(j)
	_, _ = db.claimJob(j, "pk-devbench")
	_ = db.startJob(j.ID, "go1.27.0 linux/amd64", "deadbeefcafef00d", "")
	_ = db.setRunnerStatus(liveStatus{Runner: "pk-devbench", JobID: j.ID, Phase: "pass 2 (round 1, seed 202): Codegen/FuluState/Marshal 5/158", Progress: progress{Pass: 2, Round: 1, Seed: "202", Leaf: 5, Leaves: 158, Passes: 1, Measured: 4 * time.Minute, LastPass: 3 * time.Minute, PlannedPasses: 4}, Started: time.Now().Add(-5 * time.Minute), Updated: time.Now(), LastFetch: time.Now(), Host: "pk-devbench", GoVersion: "go1.27.0"})
	_ = db.putRunner(&runnerInfo{Name: "worker-2", State: runnerQualifying, FirstSeen: time.Now().Add(-time.Hour), LastSeen: time.Now(), Note: "check 1 failed (job 3): noise p95 2.10% above 1.50%", Checks: 1, NoiseMedian: 0.4, NoiseP95: 2.1, RatioGeomean: 1.02, RatioSpread: 3.1})
	_ = db.setRunnerStatus(liveStatus{Runner: "worker-2", Updated: time.Now(), Host: "worker-2", GoVersion: "go1.27.0"})
	_ = db.insertSamples([]sample{{JobID: j.ID, Side: "base", Engine: "Codegen", Object: "FuluState", Op: "Unmarshal", Seed: "101", Iters: 4, Ns: 3e8, Bytes: 4e8, Allocs: 5e6}, {JobID: j.ID, Side: "head", Engine: "Codegen", Object: "FuluState", Op: "Unmarshal", Seed: "101", Iters: 4, Ns: 2.9e8, Bytes: 4e8, Allocs: 5e6}})
	for k := 0; k < 2; k++ {
		_ = db.insertJob(&job{Kind: kindCommit, Branch: "feature/x", HeadSHA: fmt.Sprintf("%040x", rng.Uint64()), HeadDesc: "queued change", BaseSHA: strings.Repeat("d", 40), BaseDesc: "ddddddd base", BaseRef: "master"})
	}
}

// One pass under a layout that makes the base run at twice its cost must
// not turn into a change: the median over the passes ignores it, and the
// band carries the disagreement.
func TestMedianOverPasses(t *testing.T) {
	base := []float64{100, 220, 100, 100}
	head := []float64{100.2, 100.1, 99.9, 100.3}
	m := compareMetric(base, head)
	if m.Delta > -20 {
		t.Fatalf("the mean should be pulled far down by the outlier, got %.2f", m.Delta)
	}
	if math.Abs(m.PMed) > 0.3 || m.PN != 4 {
		t.Fatalf("median over the passes %.3f over %v passes", m.PMed, m.PN)
	}
	if m.changed(0.5) {
		t.Fatalf("an outlier pass counts as a change: median %.2f band %.2f", m.PMed, m.band(0.5))
	}
	if b := m.band(0.5); b > 1 {
		t.Fatalf("an outlier pass widens the band to %.2f%%", b)
	}
	if m.PQ1 < -0.5 || m.PQ3 > 0.5 || m.DMin > -50 {
		t.Fatalf("the middle half [%.2f, %.2f] should leave the outlier pass (%.2f) out", m.PQ1, m.PQ3, m.DMin)
	}
	// A real change: every pass agrees, by more than the layouts scatter.
	m = compareMetric([]float64{100, 103, 98, 101}, []float64{105.1, 108, 102.8, 106.2})
	if !m.changed(0.5) || math.Abs(m.PMed-5) > 0.3 || m.PAgree != 4 {
		t.Fatalf("a consistent +5%%: median %.2f agree %v band %.2f", m.PMed, m.PAgree, m.band(0.5))
	}
	// Passes that disagree in direction are no change, however large.
	m = compareMetric([]float64{100, 100, 100, 100}, []float64{108, 93, 107, 94})
	if m.changed(0.5) {
		t.Fatalf("passes pointing both ways count as a change: median %.2f agree %v", m.PMed, m.PAgree)
	}
	// The geomean of an engine uses the medians.
	rs := []result{{Engine: "Codegen", Ns: compareMetric(base, head)}}
	if gm := geomeanDelta(rs); math.Abs(gm) > 0.3 {
		t.Fatalf("geomean follows the outlier: %.2f", gm)
	}
}

// Reruns of a pair rotate through sets of layout seeds.
func TestSeedRotation(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config{seeds: []string{"101", "202", "303", "404"}, name: "ctl"}
	st := &localStore{db: db, sched: &scheduler{cfg: cfg}, local: "ctl"}
	j := &job{Kind: kindCommit, Branch: "b", HeadSHA: strings.Repeat("a", 40), BaseSHA: strings.Repeat("b", 40), Runner: "ctl"}
	want := [][]string{nil, {"1101", "1202", "1303", "1404"}, {"2101", "2202", "2303", "2404"}, {"3101", "3202", "3303", "3404"}, nil}
	for run, w := range want {
		if got := st.seedsFor(j); strings.Join(got, ",") != strings.Join(w, ",") {
			t.Fatalf("run %d: seeds %v, want %v", run, got, w)
		}
		done := &job{Kind: kindCommit, Branch: "b", HeadSHA: j.HeadSHA, BaseSHA: j.BaseSHA}
		if err := db.insertJob(done); err != nil {
			t.Fatal(err)
		}
		if _, err := db.claimJob(done, "ctl"); err != nil {
			t.Fatal(err)
		}
		if err := db.finishJob(done.ID, stateDone, 4, "", 1); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJobLogCompression(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "job.log")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := compressJobLogs(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("the plain file is still there")
	}
	// A line logged after the compression joins the compressed file.
	if err := os.WriteFile(path, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := compressJobLogs(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("third\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if files := jobFiles(dir); len(files) != 1 || files[0] != "job.log" {
		t.Fatalf("files %v", files)
	}
	rec := httptest.NewRecorder()
	serveJobFile(rec, httptest.NewRequest("GET", "/raw/1/job.log", nil), path)
	if got := rec.Body.String(); got != "first\nsecond\nthird\n" {
		t.Fatalf("served %q", got)
	}
}

func TestSampleBlobAndMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "t.db")
	db, err := openDB(path)
	if err != nil {
		t.Fatal(err)
	}
	j := &job{Kind: kindCommit, State: stateQueued, HeadSHA: "h", BaseSHA: "b"}
	if err := db.insertJob(j); err != nil {
		t.Fatal(err)
	}
	var samples []sample
	for pass := 0; pass < 4; pass++ {
		for _, side := range []string{"base", "head"} {
			samples = append(samples, sample{JobID: j.ID, Side: side, Engine: "Codegen", Object: "Block", Op: "Marshal", Seed: fmt.Sprint(101 + pass), Pass: pass, Iters: 10, Ns: 100 + float64(pass), Cycles: 400, Instrs: 900})
		}
	}
	if err := db.insertSamples(samples); err != nil {
		t.Fatal(err)
	}
	full := summarize(samples)
	if err := db.replaceResults(j.ID, full); err != nil {
		t.Fatal(err)
	}
	if err := db.finishJob(j.ID, stateDone, 4, "", 1); err != nil {
		t.Fatal(err)
	}
	// A database from before the layout change has the wide results table
	// and the samples of finished jobs as rows.
	if _, err := db.db.Exec(`ALTER TABLE results ADD COLUMN ns_lo REAL NOT NULL DEFAULT 0`); err != nil {
		t.Fatal(err)
	}
	if err := db.migrateStorage(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".before-blobs"); err != nil {
		t.Fatalf("no copy of the database: %v", err)
	}
	var n int
	if err := db.db.QueryRow(`SELECT count(*) FROM pragma_table_info('results') WHERE name = 'ns_lo'`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("results still wide: %d, %v", n, err)
	}
	if rows, _ := db.sampleRows(j.ID); len(rows) != 0 {
		t.Fatalf("%d sample rows left", len(rows))
	}
	got, err := db.samplesFor(j.ID)
	if err != nil || len(got) != len(samples) {
		t.Fatalf("samples from the blob: %d, %v", len(got), err)
	}
	stored, err := db.resultsFor(j.ID)
	if err != nil || len(stored) != 1 || stored[0].Ns.PMed != full[0].Ns.PMed || stored[0].Ns.Head != full[0].Ns.Head {
		t.Fatalf("stored results %+v, %v", stored, err)
	}
	if stored[0].Ns.HQ3 != 0 {
		t.Fatal("stored results carry job-page statistics")
	}
	again := summarize(got)
	if len(again) != 1 || again[0].Ns != full[0].Ns || again[0].Cycles != full[0].Cycles {
		t.Fatalf("full results differ: %+v vs %+v (%v)", again, full, err)
	}
	// Migrating again changes nothing.
	if err := db.migrateStorage(path); err != nil {
		t.Fatal(err)
	}
}

func TestBuildFacts(t *testing.T) {
	hdir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hdir, "types", "fulu"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hdir, "types", "fulu", "gen_ssz.go"), make([]byte, 1234), 0o644); err != nil {
		t.Fatal(err)
	}
	writeBuildNote(hdir, 42)
	// The test binary stands in for a harness binary.
	s := &side{name: "head", sha: "abc", hdir: hdir, bins: map[string]map[string]string{"fulu": {"101": os.Args[0]}}}
	facts := buildFacts(7, s)
	if len(facts) != 1 || facts[0].TextBytes <= 0 || facts[0].BinBytes < facts[0].TextBytes || facts[0].GenBytes != 1234 || facts[0].BuildSeconds != 42 {
		t.Fatalf("facts %+v", facts)
	}
	// An adapter's generated files, in one or in several files.
	for name, size := range map[string]int{"baselines/lib/fulu/encoding_gen.go": 100, "baselines/kar/fulu/gen_A_ssz.go": 30, "baselines/kar/fulu/gen_B_ssz.go": 40, "baselines/kar/fulu/types.go": 999} {
		if err := os.MkdirAll(filepath.Join(hdir, filepath.Dir(name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(hdir, name), make([]byte, size), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if a, b := generatedBytes(hdir, "lib-fulu"), generatedBytes(hdir, "kar-fulu"); a != 100 || b != 70 {
		t.Fatalf("generated bytes of the adapters: %d, %d", a, b)
	}
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.insertBuilds(facts); err != nil {
		t.Fatal(err)
	}
	got, err := db.buildsFor(7)
	if err != nil || len(got) != 1 || got[0] != facts[0] {
		t.Fatalf("stored %+v, %v", got, err)
	}
}

func TestExtraFigures(t *testing.T) {
	out := []byte("BenchmarkReal/Codegen/Block/Marshal \t 1\t 100 ns/op\t 400 cycles/op\t 900 instrs/op\t 12 br-miss/op\t 3 l2-miss/op\t 4096 stack-B/op\t 512 retained-B/op\t 1.5 faults/op\t 2300 sys-ns/op\t 10 iters\n")
	lines := parseBenchOutput(out)
	if len(lines) != 1 || lines[0].extra["br-miss"] != 12 || lines[0].extra["l2-miss"] != 3 || lines[0].extra["stack"] != 4096 || lines[0].extra["retained"] != 512 || lines[0].extra["faults"] != 1.5 || lines[0].extra["sys-ns"] != 2300 {
		t.Fatalf("parsed %+v", lines)
	}
	var samples []sample
	for pass := 0; pass < 4; pass++ {
		for _, side := range []string{"base", "head"} {
			sm := sample{Side: side, Engine: "Codegen", Object: "Block", Op: "Marshal", Seed: fmt.Sprint(101 + pass), Pass: pass, Iters: 10, Ns: 100, Cycles: 400, Instrs: 900}
			if pass%2 == 0 {
				sm.Extra = map[string]float64{"br-miss": 10 + float64(pass)}
			} else {
				sm.Extra = map[string]float64{"fe-stall": 50}
			}
			samples = append(samples, sm)
		}
	}
	// A diagnostic run takes no part in the statistics.
	samples = append(samples, sample{Side: "head", Engine: "Codegen", Object: "Block", Op: "Marshal", Seed: "101", Ns: 9999, Cycles: 9999, Instrs: 900, Extra: map[string]float64{"diag": 1, "dec-uops": 7}})
	rs := summarize(samples)
	if len(rs) != 1 || rs[0].N != 4 || rs[0].Ns.Head != 100 {
		t.Fatalf("results %+v", rs)
	}
	if st := rs[0].Extra["br-miss"]; st.N != 2 || st.Head != 11 || st.Base != 11 {
		t.Fatalf("br-miss %+v", st)
	}
	if _, ok := rs[0].Extra["dec-uops"]; ok {
		t.Fatal("a diagnostic counter entered the statistics")
	}
	r := &runner{cfg: &config{counterPairs: true}}
	pair := func(pass int, seed string) string { return r.passEnv("Codegen", pass, seed)[0] }
	if pair(0, "101") == pair(1, "202") || pair(0, "101") != pair(2, "303") || pair(0, "101") == pair(0, "1101") {
		t.Fatalf("pairs %s %s %s %s", pair(0, "101"), pair(1, "202"), pair(2, "303"), pair(0, "1101"))
	}
	env := strings.Join(r.passEnv("CodegenAsync", 0, "101"), " ")
	if !strings.Contains(env, "BENCH_ALL_THREADS=1") || !strings.Contains(env, "BENCH_MEMORY=1") {
		t.Fatalf("env %s", env)
	}
}

func TestLibraryTargetsAndPooledValues(t *testing.T) {
	tags := map[string]string{"v1.0.0": "a", "v2.0.0": "b", "v2.1.0": "c", "v2.10.1": "d", "v2.9.9": "e", "v3.0.0-rc1": "f"}
	if name, ok := highestTag(tags, `^v2\.\d+\.\d+$`); !ok || name != "v2.10.1" {
		t.Fatalf("highest 2.x tag %q", name)
	}
	if m := pseudoVersion.FindStringSubmatch("v0.0.0-20260703104215-9be4f5c6a334"); m == nil || m[1] != "9be4f5c6a334" {
		t.Fatalf("pseudo-version commit %v", m)
	}
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	// A commit measured as the head of one job and the base of another:
	// its value pools both.
	run := func(j *job, side string, ns ...float64) {
		if err := db.insertJob(j); err != nil {
			t.Fatal(err)
		}
		var samples []sample
		for i, v := range ns {
			samples = append(samples, sample{JobID: j.ID, Side: side, Engine: "Codegen", Object: "Block", Op: "Marshal", Seed: fmt.Sprint(i), Pass: i, Iters: 1, Ns: v, Cycles: v * 3, Instrs: v * 5})
		}
		if err := db.insertSamples(samples); err != nil {
			t.Fatal(err)
		}
		if _, err := db.db.Exec(`UPDATE jobs SET state = ?, harness = 'h', finished = 1 WHERE id = ?`, stateDone, j.ID); err != nil {
			t.Fatal(err)
		}
		done, _ := db.getJob(j.ID)
		if err := db.updateCommitValues(done); err != nil {
			t.Fatal(err)
		}
	}
	run(&job{Kind: kindCommit, Branch: "master", HeadSHA: "c1", BaseSHA: "c0"}, "head", 100, 102)
	run(&job{Kind: kindCommit, Branch: "master", HeadSHA: "c2", BaseSHA: "c1"}, "base", 104, 106)
	vals, err := db.commitValues(subjectDynSSZ, "c1")
	if err != nil || len(vals) != 1 || vals[0].N != 4 || vals[0].Ns != 103 || vals[0].Cycles != 309 {
		t.Fatalf("pooled value of c1: %+v, %v", vals, err)
	}
	if vals, _ := db.commitValues(subjectDynSSZ, "c2"); len(vals) != 0 {
		t.Fatalf("c2 has head samples of nobody: %+v", vals)
	}
	// A library: its release has a value, its master commit none yet, so
	// the page shows the release value under both modes' fallbacks.
	lib := subjectByName("fastssz")
	run(targetJob(lib, "r1", []string{targetRelease}, []string{"v2.0.0"}, "", ""), "head", 50, 52)
	if err := db.setTarget(targetState{Subject: lib.Name, Name: targetRelease, SHA: "r1", Label: "v2.0.0"}); err != nil {
		t.Fatal(err)
	}
	if err := db.setTarget(targetState{Subject: lib.Name, Name: targetMaster, SHA: "m9", Label: "main"}); err != nil {
		t.Fatal(err)
	}
	w := &webServer{cfg: &config{mainBranch: "master", github: "o/r"}, db: db}
	targets, _ := db.targets()
	if sv := w.valuesOf(lib.Name, targetRelease, targets); sv == nil || sv.SHA != "r1" || sv.Wanted != "" || len(sv.values) != 1 || sv.values[0].Ns != 51 {
		t.Fatalf("release values %+v", sv)
	}
	if sv := w.valuesOf(lib.Name, targetMaster, targets); sv == nil || len(sv.values) != 0 {
		t.Fatalf("master has values without a job: %+v", sv)
	}
	if rs := w.otherResults(subjectDynSSZ); len(rs) != 1 || !rs[0].Other || rs[0].Ns.Head != 51 {
		t.Fatalf("library results %+v", rs)
	}
	// The same commit is not queued twice for one adapter version; a new
	// adapter version measures it again.
	if ok, _ := db.targetJobExists(lib.Name, "r1", "h"); !ok {
		t.Fatal("the finished job of r1 is not seen")
	}
	if ok, _ := db.targetJobExists(lib.Name, "r1", "h2"); ok {
		t.Fatal("a job of another adapter version counts")
	}
}

func TestResolveTargetsLive(t *testing.T) {
	if os.Getenv("BENCH_LIVE") == "" {
		t.Skip("needs the network")
	}
	for i := range subjects {
		if subjects[i].mirrored() {
			continue
		}
		states, err := resolveTargets(context.Background(), &subjects[i])
		t.Logf("%s: %+v %v", subjects[i].Name, states, err)
		if err != nil {
			t.Fail()
		}
	}
}

func TestStoredBaseRuns(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	boot := bootID()
	if boot == "" {
		t.Skip("no boot id on this system")
	}
	// An earlier job measured commit b1 as its head, on four seeds; seed
	// 303 of one operation ran on a pathological layout.
	prev := &job{Kind: kindCommit, Branch: "master", HeadSHA: "b1", BaseSHA: "b0", Runner: "box"}
	if err := db.insertJob(prev); err != nil {
		t.Fatal(err)
	}
	var samples []sample
	for i, seed := range []string{"101", "202", "303", "404"} {
		cycles := 1000.0
		if seed == "303" {
			cycles = 2800
		}
		samples = append(samples,
			sample{JobID: prev.ID, Side: "head", Engine: "Codegen", Object: "Block", Op: "MarshalTo", Seed: seed, Pass: i, Iters: 1, Ns: cycles / 3, Cycles: cycles, Instrs: 5000},
			sample{JobID: prev.ID, Side: "head", Engine: "Codegen", Object: "Block", Op: "Marshal", Seed: seed, Pass: i, Iters: 1, Ns: 700, Cycles: 2000, Instrs: 9000},
			sample{JobID: prev.ID, Side: "base", Engine: "Codegen", Object: "Block", Op: "Marshal", Seed: seed, Pass: i, Iters: 1, Ns: 1, Cycles: 1, Instrs: 1})
	}
	if err := db.insertSamples(samples); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE jobs SET state = ?, harness = 'h', go_version = 'go1', boot_id = ?, finished = ? WHERE id = ?`, stateDone, boot, time.Now().Unix(), prev.ID); err != nil {
		t.Fatal(err)
	}
	j := &job{ID: 99, Kind: kindCommit, Subject: subjectDynSSZ, HeadSHA: "h1", BaseSHA: "b1", Harness: "h", GoVersion: "go1", Runner: "box"}
	runs, err := db.storedBaseRuns(j, []string{"101", "202", "303", "404"})
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 7 {
		t.Fatalf("%d runs, want 7 (eight of b1 as head, minus the pathological layout)", len(runs))
	}
	if _, ok := runs["Codegen/Block/MarshalTo|303"]; ok {
		t.Fatal("the pathological layout serves as a base")
	}
	if sm := runs["Codegen/Block/Marshal|101"]; sm.Cycles != 2000 {
		t.Fatalf("the base side of the earlier job was taken for b1: %+v", sm)
	}
	// Another harness version, Go version or boot has no usable runs.
	for _, other := range []*job{
		{ID: 99, Subject: subjectDynSSZ, BaseSHA: "b1", Harness: "h2", GoVersion: "go1", Runner: "box"},
		{ID: 99, Subject: subjectDynSSZ, BaseSHA: "b1", Harness: "h", GoVersion: "go2", Runner: "box"},
	} {
		if runs, _ := db.storedBaseRuns(other, []string{"101"}); len(runs) != 0 {
			t.Fatalf("runs of another environment are used: %d", len(runs))
		}
	}
	if _, err := db.db.Exec(`UPDATE jobs SET boot_id = 'earlier-boot' WHERE id = ?`, prev.ID); err != nil {
		t.Fatal(err)
	}
	if runs, _ := db.storedBaseRuns(j, []string{"101"}); len(runs) != 0 {
		t.Fatalf("runs of an earlier boot are used: %d", len(runs))
	}
	// Runs taken from another job are not counted again as measurements.
	note := baseNote([]sample{
		{Side: "base", Extra: map[string]float64{"from": 7}}, {Side: "base", Extra: map[string]float64{"from": 7}}, {Side: "base"}, {Side: "head"}})
	if !strings.Contains(note, "2 of 3 base runs") || !strings.Contains(note, "#7") {
		t.Fatalf("note %q", note)
	}
}

func TestResultLines(t *testing.T) {
	var buf bytes.Buffer
	n := 0
	w := &resultLines{buf: &buf, fn: func() { n++ }}
	for _, chunk := range []string{"goos: linux\nBenchmarkReal/A/B/C \t 1\t 5 ns", "/op\nBenchmarkReal/A/B/D \t 1\t 7 ns/op\nPASS\n"} {
		_, _ = w.Write([]byte(chunk))
	}
	if n != 2 || !strings.Contains(buf.String(), "PASS") {
		t.Fatalf("%d result lines, output %q", n, buf.String())
	}
}

func TestCPUCount(t *testing.T) {
	for list, want := range map[string]int{"0-4": 5, "0,1,3": 3, "2": 1, "0-1, 4-5": 4, "": 1} {
		if got := cpuCount(list); got != want {
			t.Errorf("cpuCount(%q) = %d, want %d", list, got, want)
		}
	}
}

func TestIdleWeights(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config{harnessDir: t.TempDir(), mainBranch: "master"}
	s := &scheduler{cfg: cfg, db: db, prs: newPRCache("o/r", "")}
	own, lib := subjectByName(subjectDynSSZ), subjectByName("fastssz")
	ownHash, _ := subjectHash(cfg.harnessDir, own)
	libHash, _ := subjectHash(cfg.harnessDir, lib)
	now := time.Now()
	done := func(j *job, harness string, ago time.Duration) {
		if err := db.insertJob(j); err != nil {
			t.Fatal(err)
		}
		if _, err := db.db.Exec(`UPDATE jobs SET state = ?, harness = ?, finished = ? WHERE id = ?`, stateDone, harness, now.Add(-ago).Unix(), j.ID); err != nil {
			t.Fatal(err)
		}
	}
	// An open pull request with two measured heads, and a library with a
	// release and a main head.
	s.prs.byHead["feature"] = pullRequest{Number: 7, HeadRef: "feature", HeadSHA: "p2", BaseRef: "master"}
	done(&job{Kind: kindCommit, Branch: "feature", HeadSHA: "p1", BaseSHA: "m", PR: 7}, ownHash, 3*time.Hour)
	done(&job{Kind: kindCommit, Branch: "feature", HeadSHA: "p2", BaseSHA: "m", PR: 7}, ownHash, 2*time.Hour)
	done(targetJob(lib, "rel", []string{targetRelease}, []string{"v2.0.0"}, "", ""), libHash, 5*time.Hour)
	done(targetJob(lib, "main", []string{targetMaster}, []string{"main"}, "", ""), libHash, 4*time.Hour)
	_ = db.setTarget(targetState{Subject: lib.Name, Name: targetRelease, SHA: "rel", Label: "v2.0.0"})
	_ = db.setTarget(targetState{Subject: lib.Name, Name: targetMaster, SHA: "main", Label: "main"})
	// A target whose first job has not finished is no candidate.
	_ = db.setTarget(targetState{Subject: "karalabe-ssz", Name: targetMaster, SHA: "k", Label: "main"})

	// Every candidate has one run in the window: runs per weight are
	// 1/8 (head p2), 1/6 (release), 1/4 (earlier head p1 and main).
	var picked []string
	for i := 0; i < 5; i++ {
		j, err := s.idleJob()
		if err != nil || j == nil {
			t.Fatalf("idle job %d: %v, %v", i, j, err)
		}
		if !j.Refinement || j.Note != "refinement run" {
			t.Fatalf("not marked as a refinement: %+v", j)
		}
		picked = append(picked, j.HeadSHA)
		harness := libHash
		if j.Subject == own.Name {
			harness = ownHash
		}
		done(j, harness, time.Duration(5-i)*time.Minute)
	}
	// p2 (1/8), rel (1/6), then p2 at 2/8 ties with p1 and main at 1/4:
	// the library that ran longest ago goes first, and of it the commit
	// that ran longest ago.
	want := []string{"p2", "rel", "p1", "main", "p2"}
	if fmt.Sprint(picked) != fmt.Sprint(want) {
		t.Fatalf("picked %v, want %v", picked, want)
	}
	if j, _ := s.idleJob(); j == nil || j.HeadSHA == "k" {
		t.Fatalf("an unmeasured target was picked: %+v", j)
	}
}

func TestNoiseFromRepeatedRuns(t *testing.T) {
	db, err := openDB(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	run := func(sha string, seeds []string, ns float64) int64 {
		j := &job{Kind: kindCommit, Branch: "master", HeadSHA: sha, Runner: "box"}
		if err := db.insertJob(j); err != nil {
			t.Fatal(err)
		}
		var samples []sample
		for i, seed := range seeds {
			samples = append(samples, sample{JobID: j.ID, Side: "head", Engine: "Codegen", Object: "Block", Op: "Marshal", Seed: seed, Pass: i, Iters: 1, Ns: ns, Cycles: ns * 3, Instrs: 900})
		}
		if err := db.insertSamples(samples); err != nil {
			t.Fatal(err)
		}
		if _, err := db.db.Exec(`UPDATE jobs SET state = ?, harness = 'h', boot_id = 'b', finished = 1 WHERE id = ?`, stateDone, j.ID); err != nil {
			t.Fatal(err)
		}
		return j.ID
	}
	set0, set1 := []string{"101", "202", "303", "404"}, []string{"1101", "1202", "1303", "1404"}
	run("m", set0, 100)
	run("m", set1, 500) // another seed set: no pair with the first
	run("x", set0, 100) // another commit
	later := run("m", set0, 102)
	sets, err := noiseSets(db)
	if err != nil || len(sets) != 1 || sets[0].ID != later {
		t.Fatalf("sets %+v, %v", sets, err)
	}
	if rs := sets[0].results; len(rs) != 1 || rs[0].N != 4 || math.Abs(rs[0].Ns.Delta-2) > 1e-9 {
		t.Fatalf("results %+v", sets[0].results)
	}
	nf := noiseFloorOf(db, "box")
	if nf.Jobs != 1 || math.Abs(nf.PerLeaf["Codegen/Block/Marshal"]-2) > 1e-9 {
		t.Fatalf("noise floor %+v", nf)
	}
}

func TestCompareCommits(t *testing.T) {
	run := func(side, seed string, job int64, ns float64) sample {
		return sample{JobID: job, Side: side, Engine: "Codegen", Object: "Block", Op: "Marshal", Seed: seed, Iters: 1, Ns: ns, Cycles: ns * 3, Instrs: 900}
	}
	// The head was run under two seed sets, seed 101 twice; the base under
	// three, in other jobs and far more often.
	head := []sample{run("head", "101", 1, 110), run("head", "101", 4, 112), run("head", "202", 1, 220), run("head", "1101", 3, 330)}
	base := []sample{run("head", "101", 2, 100), run("base", "101", 1, 100), run("head", "202", 2, 200), run("head", "1101", 5, 300), run("head", "2101", 6, 999)}
	rs := compareCommits(head, base)
	if len(rs) != 1 || rs[0].Baseline || rs[0].Ns.PN != 3 {
		t.Fatalf("results %+v", rs)
	}
	// Per seed: 111 against 100, 220 against 200, 330 against 300.
	if math.Abs(rs[0].Ns.PMed-10) > 1e-9 || math.Abs(rs[0].Ns.DMax-11) > 1e-9 {
		t.Fatalf("median %v, largest %v", rs[0].Ns.PMed, rs[0].Ns.DMax)
	}
	// Without base runs the head's values stand alone.
	alone := compareCommits(head, nil)
	if len(alone) != 1 || !alone[0].Baseline || alone[0].N != 3 {
		t.Fatalf("alone %+v", alone)
	}
}

func TestCheckoutTags(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("options.go", "package ssz\nfunc WithNoFastSsz() {}\n")
	write("options_test.go", "package ssz\nfunc WithAsyncHashing(n int) {}\n") // a test file does not count
	s := &side{tags: checkoutTags(dir)}
	if fmt.Sprint(s.tags) != "[noasync]" || !s.lacks("CodegenAsync") || !s.lacks("ReflectionAsync") || s.lacks("Codegen") || s.lacks("Reflection") {
		t.Fatalf("tags %v", s.tags)
	}
	write("async.go", "package ssz\nfunc WithAsyncHashing(workers int) {}\n")
	if s := (&side{tags: checkoutTags(dir)}); len(s.tags) != 0 || s.lacks("ReflectionAsync") || s.tagArgs() != nil {
		t.Fatalf("tags %v", s.tags)
	}
}
