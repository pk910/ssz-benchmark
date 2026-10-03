package main

import (
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
		if err := db.startJob(j.ID, "go1.27", "deadbeef"); err != nil {
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
		if err := db.startJob(j.ID, "go1.27.0 linux/amd64", "deadbeefcafef00d"); err != nil {
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
	_ = db.startJob(j.ID, "go1.27.0 linux/amd64", "deadbeefcafef00d")
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
