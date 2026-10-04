package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// runner claims jobs one at a time, builds the harness against both
// checkouts and measures them interleaved on the benchmark cpus. It talks
// to the controller through a jobStore: the controller's own database when
// it runs there, HTTP when it runs on a worker machine.
type runner struct {
	seeds []string // layout seeds of the job being run
	cfg   *config
	store jobStore
	git   *gitRepo
	name  string
	sched *scheduler // on the controller: the fetch status to publish

	mu              sync.Mutex
	current         *job
	phase           string
	started         time.Time
	progress        progress
	goVersionCached string

	// What the passes of the running job measured, for the diagnostic runs.
	// onResult is called for every result line a benchmark process prints.
	onResult func()

	jobSamples []sample
	jobIters   map[string]int
	jobLeaves  map[string]leaf
}

// progress is where the running job stands.
type progress struct {
	Pass, Round   int
	Seed          string
	Leaf, Leaves  int
	Passes        int // passes completed
	Measured      time.Duration
	LastPass      time.Duration
	PlannedPasses int // passes the budget allows, estimated from the last pass
}

// liveStatus is what a runner publishes about itself through the
// controller, so the web process can show it without sharing memory.
type liveStatus struct {
	Runner    string
	JobID     int64
	Phase     string
	Progress  progress
	Started   time.Time
	Updated   time.Time
	LastFetch time.Time
	FetchErr  string
	Host      string
	GoVersion string
}

func (r *runner) loop(ctx context.Context) {
	r.goVersionCached = r.goVersion()
	r.publish()
	for ctx.Err() == nil {
		j, err := r.store.claim(r.name)
		if err != nil {
			log.Printf("claim: %v", err)
			r.setPhase(nil, "")
			sleepCtx(ctx, 30*time.Second)
			continue
		}
		if j == nil {
			// Nothing for this runner (a rejected worker, or nothing to do).
			r.setPhase(nil, "")
			sleepCtx(ctx, time.Minute)
			continue
		}
		ok := r.runJob(ctx, j)
		if ctx.Err() == nil {
			r.prune()
			if err := compressJobLogs(filepath.Join(r.cfg.dataDir, "jobs", strconv.FormatInt(j.ID, 10))); err != nil {
				log.Printf("job %d: compress logs: %v", j.ID, err)
			}
		}
		if !ok {
			// A failing environment must not spin through jobs.
			sleepCtx(ctx, time.Minute)
		}
	}
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func (r *runner) setPhase(j *job, phase string) {
	r.mu.Lock()
	r.current, r.phase = j, phase
	if j == nil {
		r.started = time.Time{}
		r.progress = progress{}
	} else if r.started.IsZero() {
		r.started = time.Now()
	}
	r.mu.Unlock()
	r.publish()
}

func (r *runner) setProgress(p progress) {
	r.mu.Lock()
	r.progress = p
	r.mu.Unlock()
	r.publish()
}

// publish sends the live status to the controller.
func (r *runner) publish() {
	r.mu.Lock()
	ls := liveStatus{Runner: r.name, Phase: r.phase, Progress: r.progress, Started: r.started, Updated: time.Now(), GoVersion: r.goVersionCached}
	if r.current != nil {
		ls.JobID = r.current.ID
	}
	r.mu.Unlock()
	ls.Host, _ = os.Hostname()
	if r.sched != nil {
		ls.LastFetch, ls.FetchErr = r.sched.status()
	}
	if err := r.store.publish(ls); err != nil {
		log.Printf("status: %v", err)
	}
}

func (r *runner) goVersion() string {
	out, err := exec.Command(r.cfg.goBin, "version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimPrefix(strings.TrimSpace(string(out)), "go version ")
}

// harnessHash identifies the harness the dynamic-ssz engines are measured
// with: its sources without the baseline libraries, so a change to those
// does not separate the runs of a commit pair.
func harnessHash(dir string) (string, error) {
	return hashTree(dir, func(rel string) bool { return !strings.HasPrefix(rel, "baselines/") })
}

// hashTree hashes the Go sources, module files and generation recipes
// under dir that keep accepts (generated gen_* files follow from the rest).
func hashTree(dir string, keep func(rel string) bool) (string, error) {
	h := sha256.New()
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		name := info.Name()
		rel, _ := filepath.Rel(dir, path)
		if !keep(filepath.ToSlash(rel)) {
			return nil
		}
		recipe := name == "generate.sh" || name == "order.txt" || strings.HasSuffix(name, ".yaml")
		if (strings.HasSuffix(name, ".go") && !strings.HasPrefix(name, "gen_")) || name == "go.mod" || name == "go.sum" || recipe {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		rel, _ := filepath.Rel(dir, f)
		fmt.Fprintf(h, "%s\n%d\n", rel, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// side is one checkout of the pair with the harness built against it.
type side struct {
	name string
	sha  string
	dir  string                       // worktree
	hdir string                       // harness build directory
	bins map[string]map[string]string // package -> seed -> test binary
}

func (s *side) bin(pkg, seed string) string {
	if s.bins[pkg] == nil {
		return ""
	}
	return s.bins[pkg][seed]
}

// runJob runs one job and reports whether it finished.
func (r *runner) runJob(ctx context.Context, j *job) bool {
	start := time.Now()
	r.mu.Lock()
	r.started = time.Time{}
	r.mu.Unlock()
	r.setPhase(j, "preparing")
	// A rerun of a pair measures under other layouts than the runs before
	// it; the controller hands the seeds out with the job.
	r.seeds = r.cfg.seeds
	if len(j.Seeds) > 0 {
		r.seeds = j.Seeds
	}
	lib := subjectByName(j.Subject)
	if lib == nil {
		log.Printf("job %d: unknown library %q", j.ID, j.Subject)
		return false
	}
	hash, err := subjectHash(r.cfg.harnessDir, lib)
	if err != nil {
		log.Printf("job %d: harness: %v", j.ID, err)
		return false
	}
	if err := r.store.started(j.ID, r.goVersionCached, hash); err != nil {
		log.Printf("job %d: %v", j.ID, err)
	}
	j.Harness = hash
	jobDir := filepath.Join(r.cfg.dataDir, "jobs", strconv.FormatInt(j.ID, 10))
	_ = os.RemoveAll(jobDir)
	_ = os.MkdirAll(jobDir, 0o755)
	logw := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		log.Printf("job %d: %s", j.ID, line)
		r.store.log(j.ID, line)
	}
	fail := func(err error) bool {
		if ctx.Err() != nil {
			// Shutting down: the job runs again after the restart.
			logw("interrupted, queued again")
			_ = r.store.interrupted(j.ID)
		} else {
			logw("failed: %v", err)
			_ = r.store.finish(j.ID, 0, err.Error(), time.Since(start).Seconds())
		}
		r.setPhase(nil, "")
		return false
	}
	// A rerun after an interruption starts from scratch.
	if err := r.store.resetSamples(j.ID); err != nil {
		return fail(err)
	}
	head := &side{name: "head", sha: j.HeadSHA}
	base := &side{name: "base", sha: j.BaseSHA}
	if lib.mirrored() {
		if err := r.git.fetch(); err != nil {
			logw("fetch before checkout: %v", err)
		}
	}
	// A job measures its head, and its base when it has one.
	built := []*side{head}
	if base.sha != "" {
		built = append(built, base)
	}
	for _, s := range built {
		r.setPhase(j, "building "+s.name)
		if lib.mirrored() {
			// The harness packages against a checkout from the mirror.
			s.dir = filepath.Join(r.cfg.dataDir, "wt", s.sha)
			s.hdir = filepath.Join(r.cfg.dataDir, "hb", hash+"-"+s.sha)
			if err := r.git.worktree(s.dir, s.sha); err != nil {
				return fail(fmt.Errorf("checkout %s: %w", s.name, err))
			}
			if err := r.own(s.dir); err != nil {
				return fail(fmt.Errorf("checkout %s: %w", s.name, err))
			}
			if err := r.buildHarness(ctx, s, logw); err != nil {
				return fail(fmt.Errorf("build %s: %w", s.name, err))
			}
			used(s.dir)
		} else {
			// The library's adapter; its recipe fetches the module at the
			// commit, so there is no checkout.
			short := s.sha
			if len(short) > 12 {
				short = short[:12]
			}
			s.hdir = filepath.Join(r.cfg.dataDir, "hb", hash+"-"+lib.Name+"-"+short)
			if err := r.buildAdapter(ctx, s, lib, logw); err != nil {
				return fail(fmt.Errorf("build %s: %w", s.name, err))
			}
		}
		used(s.hdir)
		if err := r.store.addBuilds(buildFacts(j.ID, s)); err != nil {
			logw("%s: build facts: %v", s.name, err)
		}
	}

	passes, err := r.measure(ctx, j, head, base, jobDir, logw)
	if err != nil {
		return fail(fmt.Errorf("measure: %w", err))
	}
	r.diagnose(ctx, j, head, base, jobDir, logw)
	logw("measured %d passes in %s", passes, time.Since(start).Round(time.Second))
	if err := r.store.finish(j.ID, passes, "", time.Since(start).Seconds()); err != nil {
		logw("finish: %v", err)
	}
	r.setPhase(nil, "")
	return true
}

// building wraps a build command so that it runs on the build cpus: no
// benchmark runs while a job builds, so its builds can have the benchmark
// cores and leave the shared ones alone. Without build cpus the command
// stays on the daemon's own.
func (r *runner) building(name string, args ...string) (string, []string, []string) {
	if r.cfg.buildCPUs == "" {
		return name, args, nil
	}
	return "taskset", append([]string{"-c", r.cfg.buildCPUs, name}, args...), []string{"GOMAXPROCS=" + strconv.Itoa(cpuCount(r.cfg.buildCPUs))}
}

// cpuCount counts the cpus of a list like "0-4" or "0,1,3".
func cpuCount(list string) int {
	n := 0
	for _, part := range strings.Split(list, ",") {
		lo, hi, isRange := strings.Cut(strings.TrimSpace(part), "-")
		a, err := strconv.Atoi(lo)
		if err != nil {
			continue
		}
		b := a
		if isRange {
			if b, err = strconv.Atoi(hi); err != nil || b < a {
				continue
			}
		}
		n += b - a + 1
	}
	return max(n, 1)
}

func (r *runner) goCmd(ctx context.Context, dir string, args ...string) *exec.Cmd {
	name, args, env := r.building(r.cfg.goBin, args...)
	cmd := r.sandboxed(ctx, true, name, args...)
	cmd.Env = append(cmd.Env, env...)
	cmd.Dir = dir
	// No version-control stamping: the checkout belongs to the daemon's git,
	// which the sandbox user may not query, and the stamp is of no use here.
	cmd.Env = append(cmd.Env, "GOFLAGS=-mod=mod -buildvcs=false", "GOTOOLCHAIN=local")
	return cmd
}

// sandboxed builds a command that runs as the sandbox user when one is
// configured: without privileges, and without any network unless network
// is set (builds download modules; a benchmark needs nothing). The code of
// a checkout is never run as the daemon's own user then.
func (r *runner) sandboxed(ctx context.Context, network bool, name string, args ...string) *exec.Cmd {
	u := r.cfg.sandboxUser
	if u == "" {
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = os.Environ()
		return cmd
	}
	wrap := []string{"setpriv", "--reuid=" + u, "--regid=" + u, "--init-groups", "--no-new-privs", "--", name}
	if !network {
		wrap = append([]string{"unshare", "--net", "--"}, wrap...)
	}
	cmd := exec.CommandContext(ctx, wrap[0], append(wrap[1:], args...)...)
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HOME=") && !strings.HasPrefix(kv, "RUNNER_TOKEN=") && !strings.HasPrefix(kv, "GITHUB_TOKEN=") && !strings.HasPrefix(kv, "GH_") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "HOME="+filepath.Join("/home", u))
	return cmd
}

// own hands a directory tree to the sandbox user.
func (r *runner) own(path string) error {
	u := r.cfg.sandboxUser
	if u == "" {
		return nil
	}
	return runLogged(exec.Command("chown", "-R", u+":"+u, path))
}

func runLogged(cmd *exec.Cmd) error {
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", filepath.Base(cmd.Path)+" "+strings.Join(cmd.Args[1:], " "), err, trimOut(out))
	}
	return nil
}

// buildHarness copies the harness next to the checkout, points it at the
// checkout, generates the types of every package with the checkout's
// generator and compiles one test binary per package and layout seed. A
// package the checkout cannot generate or build is left out (an old commit
// without the newer type features still measures the others); no package
// at all is an error. The result is cached per harness and commit.
func (r *runner) buildHarness(ctx context.Context, s *side, logw func(string, ...any)) error {
	s.bins = map[string]map[string]string{}
	if data, err := os.ReadFile(filepath.Join(s.hdir, "ok")); err == nil {
		for _, pkg := range strings.Fields(string(data)) {
			s.bins[pkg] = map[string]string{}
			for _, seed := range r.seeds {
				out := filepath.Join(s.hdir, "seed-"+seed+"-"+pkg+".test")
				if _, err := os.Stat(out); err != nil {
					// A layout this cached build has not been linked with yet.
					if err := r.compile(ctx, s, pkg, seed, out); err != nil {
						return fmt.Errorf("%s seed %s: %w", pkg, seed, err)
					}
				}
				s.bins[pkg][seed] = out
			}
		}
		if len(s.bins) > 0 {
			return nil
		}
	}
	start := time.Now()
	_ = os.RemoveAll(s.hdir)
	if err := copyTree(r.cfg.harnessDir, s.hdir); err != nil {
		return err
	}
	if err := r.own(s.hdir); err != nil {
		return err
	}
	for _, stale := range []string{"payload", "dynssz-gen"} {
		_ = os.Remove(filepath.Join(s.hdir, stale))
	}
	for _, pkg := range r.cfg.packages {
		_ = os.Remove(filepath.Join(s.hdir, "types", pkg, "gen_ssz.go"))
	}
	gomod := filepath.Join(s.hdir, "go.mod")
	data, err := os.ReadFile(gomod)
	if err != nil {
		return err
	}
	re := regexp.MustCompile(`(?m)^replace github.com/pk910/dynamic-ssz => .*$`)
	if !re.Match(data) {
		return fmt.Errorf("harness go.mod has no replace directive for dynamic-ssz")
	}
	data = re.ReplaceAll(data, []byte("replace github.com/pk910/dynamic-ssz => "+s.dir))
	if err := os.WriteFile(gomod, data, 0o644); err != nil {
		return err
	}
	if err := runLogged(r.goCmd(ctx, s.hdir, "mod", "tidy")); err != nil {
		return err
	}
	gen := filepath.Join(s.hdir, "dynssz-gen")
	if err := runLogged(r.goCmd(ctx, s.dir, "build", "-o", gen, "./dynssz-gen")); err != nil {
		return fmt.Errorf("generator: %w", err)
	}
	var built []string
	for _, pkg := range r.cfg.packages {
		if err := r.buildPackage(ctx, s, pkg, gen); err != nil {
			logw("%s: package %s left out: %v", s.name, pkg, err)
			continue
		}
		built = append(built, pkg)
	}
	if err := ctx.Err(); err != nil {
		// Interrupted: a package may be missing only because its build was
		// cut off, so nothing is recorded as the result of this build.
		return err
	}
	if len(built) == 0 {
		return fmt.Errorf("no harness package builds against %s", s.sha[:12])
	}
	if err := os.WriteFile(filepath.Join(s.hdir, "ok"), []byte(strings.Join(built, " ")), 0o644); err != nil {
		return err
	}
	writeBuildNote(s.hdir, time.Since(start).Seconds())
	logw("%s: harness built against %s in %s (packages %s)", s.name, s.sha[:12], time.Since(start).Round(time.Second), strings.Join(built, ", "))
	return nil
}

// buildAdapter builds the adapter of a library against the commit of the
// side: a copy of the harness in which the adapter's recipe fetches the
// library at that commit and generates its code with that commit's
// generator, then one test binary per fork and layout seed.
func (r *runner) buildAdapter(ctx context.Context, s *side, lib *subject, logw func(string, ...any)) error {
	s.bins = map[string]map[string]string{}
	if data, err := os.ReadFile(filepath.Join(s.hdir, "ok")); err == nil {
		for _, pkg := range strings.Fields(string(data)) {
			s.bins[pkg] = map[string]string{}
			for _, seed := range r.seeds {
				out := filepath.Join(s.hdir, "seed-"+seed+"-"+pkg+".test")
				if _, err := os.Stat(out); err != nil {
					// A layout this cached build has not been linked with yet.
					if err := r.compile(ctx, s, pkg, seed, out); err != nil {
						return fmt.Errorf("%s seed %s: %w", pkg, seed, err)
					}
				}
				s.bins[pkg][seed] = out
			}
		}
		if len(s.bins) > 0 {
			return nil
		}
	}
	start := time.Now()
	_ = os.RemoveAll(s.hdir)
	if err := copyTree(r.cfg.harnessDir, s.hdir); err != nil {
		return err
	}
	if err := r.own(s.hdir); err != nil {
		return err
	}
	libDir := filepath.Join(s.hdir, "baselines", lib.Adapter)
	// The recipe picks the Go toolchain its generator needs itself.
	name, args, env := r.building("bash", "generate.sh", s.sha)
	gen := r.sandboxed(ctx, true, name, args...)
	gen.Dir = libDir
	gen.Env = append(append(gen.Env, env...), "GOTOOLCHAIN=auto")
	if err := runLogged(gen); err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	if err := runLogged(r.goCmd(ctx, libDir, "mod", "tidy")); err != nil {
		return err
	}
	var built []string
	forks, _ := os.ReadDir(libDir)
	for _, fork := range forks {
		if _, err := os.Stat(filepath.Join(libDir, fork.Name(), "bench_test.go")); err != nil {
			continue
		}
		pkg := lib.Adapter + "-" + fork.Name()
		bins := map[string]string{}
		var err error
		for _, seed := range r.seeds {
			out := filepath.Join(s.hdir, "seed-"+seed+"-"+pkg+".test")
			if err = r.compile(ctx, s, pkg, seed, out); err != nil {
				break
			}
			bins[seed] = out
		}
		if err != nil {
			logw("%s left out: %v", pkg, err)
			continue
		}
		s.bins[pkg] = bins
		built = append(built, pkg)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(built) == 0 {
		return fmt.Errorf("no package of the adapter builds")
	}
	if err := os.WriteFile(filepath.Join(s.hdir, "ok"), []byte(strings.Join(built, " ")), 0o644); err != nil {
		return err
	}
	writeBuildNote(s.hdir, time.Since(start).Seconds())
	logw("%s built at %s in %s (%s)", lib.Name, s.sha, time.Since(start).Round(time.Second), strings.Join(built, ", "))
	return nil
}

// compile links the test binary of a package under one layout seed. A
// baseline package is named <lib>-<fork> and lives in its library's module.
func (r *runner) compile(ctx context.Context, s *side, pkg, seed, out string) error {
	dir, target := s.hdir, "./"+pkg
	if lib, fork, ok := strings.Cut(pkg, "-"); ok {
		if _, err := os.Stat(filepath.Join(s.hdir, "baselines", lib, "go.mod")); err == nil {
			dir, target = filepath.Join(s.hdir, "baselines", lib), "./"+fork
		}
	}
	return runLogged(r.goCmd(ctx, dir, "test", "-c", "-o", out, "-ldflags", "-funcalign=64 -randlayout="+seed, target))
}

// buildPackage generates types/<pkg> and compiles <pkg> for every seed.
func (r *runner) buildPackage(ctx context.Context, s *side, pkg, gen string) error {
	typesDir := filepath.Join("types", pkg)
	typesSrc, err := os.ReadFile(filepath.Join(s.hdir, typesDir, "types.go"))
	if err != nil {
		return err
	}
	var names []string
	for _, m := range regexp.MustCompile(`(?m)^type ([A-Z][A-Za-z0-9]*) struct`).FindAllStringSubmatch(string(typesSrc), -1) {
		names = append(names, m[1])
	}
	genCmd := r.sandboxed(ctx, false, "prlimit", "--as="+strconv.Itoa(8<<30), gen, "-package", "./"+typesDir, "-with-streaming", "-legacy", "-types", strings.Join(names, ","), "-output", filepath.Join(typesDir, "gen_ssz.go"))
	genCmd.Dir = s.hdir
	if err := runLogged(genCmd); err != nil {
		return fmt.Errorf("generate: %w", err)
	}
	if _, err := os.Stat(filepath.Join(s.hdir, typesDir, "gen_ssz.go")); err != nil {
		return fmt.Errorf("generate: no output")
	}
	bins := map[string]string{}
	for _, seed := range r.seeds {
		out := filepath.Join(s.hdir, "seed-"+seed+"-"+pkg+".test")
		if err := runLogged(r.goCmd(ctx, s.hdir, "test", "-c", "-o", out, "-ldflags", "-funcalign=64 -randlayout="+seed, "./"+pkg)); err != nil {
			return fmt.Errorf("seed %s: %w", seed, err)
		}
		bins[seed] = out
	}
	s.bins[pkg] = bins
	return nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func trimOut(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 1500 {
		s = s[:1500] + "..."
	}
	return s
}

// benchRegex matches exactly one benchmark: the name is split on "/" the
// way the testing package splits its -bench pattern.
func benchRegex(name string) string {
	parts := strings.Split(name, "/")
	for i, p := range parts {
		parts[i] = "^" + regexp.QuoteMeta(p) + "$"
	}
	return strings.Join(parts, "/")
}

// parseLeaf reads BenchmarkReal/<Engine>/<Object>/<Op>.
func (r *runner) parseLeaf(name string) (leaf, bool) {
	parts := strings.Split(name, "/")
	if len(parts) != 4 || parts[0] != "BenchmarkReal" {
		return leaf{}, false
	}
	l := leaf{Engine: parts[1], Object: parts[2], Op: parts[3]}
	for _, b := range r.cfg.baselines {
		if b == l.Engine {
			l.Baseline = true
		}
	}
	return l, true
}

// discover lists the leaves of the harness by running every benchmark of
// every package for one iteration, once per harness version.
func (r *runner) discover(ctx context.Context, s *side, seed, jobDir string, logw func(string, ...any)) ([]leaf, error) {
	var leaves []leaf
	pkgs := make([]string, 0, len(s.bins))
	for pkg := range s.bins {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	for _, pkg := range pkgs {
		if s.bin(pkg, seed) == "" {
			continue
		}
		out, _, err := r.runBinary(ctx, s, pkg, seed, ".", "1x", "", filepath.Join(jobDir, "discover-"+pkg+".txt"))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", pkg, err)
		}
		for _, bl := range parseBenchOutput(out) {
			l, ok := r.parseLeaf(bl.name)
			if !ok {
				continue
			}
			l.Pkg = pkg
			leaves = append(leaves, l)
		}
	}
	logw("discovered %d leaves", len(leaves))
	return leaves, nil
}

// calibrate fixes the iteration count of a leaf that has none yet: the
// count Go settles on for the configured benchtime, at least minIters,
// measured on the base side.
func (r *runner) calibrate(ctx context.Context, l leaf, s *side, seed, jobDir string) (int, error) {
	out, _, err := r.runBinary(ctx, s, l.Pkg, seed, benchRegex(l.name()), r.cfg.benchTime, "", filepath.Join(jobDir, "calibrate.txt"))
	if err != nil {
		return 0, err
	}
	for _, bl := range parseBenchOutput(out) {
		if bl.name == l.name() {
			return max(bl.iters, r.cfg.minIters), nil
		}
	}
	return 0, fmt.Errorf("%s produced no result", l.name())
}

// measure runs every leaf on both sides back to back under one layout
// seed after another, the side order alternating per pass, each with the
// fixed iteration count of the leaf. Baseline leaves run on the head binary
// only. Passes over the seeds, then further rounds of them, continue until
// the measurement budget is spent; at least two passes are made.
func (r *runner) measure(ctx context.Context, j *job, head, base *side, jobDir string, logw func(string, ...any)) (int, error) {
	seed0 := r.seeds[0]
	leaves, err := r.store.harnessLeaves(j.Harness)
	if err != nil {
		return 0, err
	}
	if len(leaves) == 0 {
		r.setPhase(j, "discovering benchmarks")
		leaves, err = r.discover(ctx, head, seed0, jobDir, logw)
		if err != nil {
			return 0, fmt.Errorf("discover: %w", err)
		}
		if len(leaves) == 0 {
			return 0, fmt.Errorf("no benchmarks found")
		}
		if err := r.store.setHarnessLeaves(j.Harness, leaves); err != nil {
			return 0, err
		}
	}
	sortLeafList(leaves)
	// Leaves of a package one side could not build are left out of the job.
	// A job without a base measures its head only.
	for i := range leaves {
		leaves[i].Baseline = base.sha == ""
	}
	var measurable []leaf
	skipped := map[string]bool{}
	for _, l := range leaves {
		if head.bin(l.Pkg, seed0) == "" || (!l.Baseline && base.bin(l.Pkg, seed0) == "") {
			skipped[l.Pkg] = true
			continue
		}
		measurable = append(measurable, l)
	}
	for pkg := range skipped {
		logw("package %s is not built on both sides, its benchmarks are left out", pkg)
	}
	leaves = measurable
	if len(leaves) == 0 {
		return 0, fmt.Errorf("no benchmark package builds on both sides")
	}
	iters, err := r.store.benchIters()
	if err != nil {
		return 0, err
	}
	for _, l := range leaves {
		if iters[l.key()] > 0 {
			continue
		}
		r.setPhase(j, "calibrating "+l.key())
		calSide := base
		if l.Baseline {
			calSide = head
		}
		n, err := r.calibrate(ctx, l, calSide, seed0, jobDir)
		if err != nil {
			return 0, fmt.Errorf("calibrate %s: %w", l.key(), err)
		}
		logw("calibrated %s: %d iterations", l.key(), n)
		if err := r.store.setBenchIters(l, n); err != nil {
			return 0, err
		}
		iters[l.key()] = n
	}

	// One process per package, engine and object runs every operation of
	// that group with the group's iteration counts; a group is the unit of
	// base/head interleaving.
	type group struct {
		pkg, engine, object string
		leaves              []leaf
	}
	var groups []*group
	byKey := map[string]*group{}
	for _, l := range leaves {
		k := joinKey(l.Pkg, l.Engine, l.Object)
		g := byKey[k]
		if g == nil {
			g = &group{pkg: l.Pkg, engine: l.Engine, object: l.Object}
			byKey[k] = g
			groups = append(groups, g)
		}
		g.leaves = append(g.leaves, l)
	}

	var measured, lastPass time.Duration
	passes := 0
	budget := r.cfg.target + r.cfg.target/4
	minPasses := r.cfg.minPasses
	if minPasses <= 0 {
		minPasses = len(r.seeds)
	}
	planned := func() int {
		if lastPass == 0 {
			return minPasses
		}
		return max(minPasses, passes+int((budget-measured)/lastPass))
	}
	r.jobSamples, r.jobIters, r.jobLeaves = nil, iters, map[string]leaf{}
	for _, l := range leaves {
		r.jobLeaves[l.key()] = l
	}
	// The earlier runs of the base commit a one-sided job may use.
	var stored map[string]sample
	if r.cfg.oneSided && base.sha != "" {
		j.GoVersion = r.goVersionCached
		if stored, err = r.store.baseRuns(j, r.seeds); err != nil {
			return 0, err
		}
		if stored == nil {
			logw("one-sided: this runner measures both sides")
		} else {
			logw("one-sided: %d earlier runs of the base %s can stand in for measuring it", len(stored), base.sha[:12])
		}
	}
	seen := map[string][]float64{} // leaf+side+seed -> cycles (or ns) of the kept measurements
	for round := 0; ; round++ {
		for i, seed := range r.seeds {
			if passes >= minPasses && measured+lastPass > budget {
				return passes, nil
			}
			passStart := time.Now()
			order := []*side{base, head}
			if passes%2 == 1 {
				order = []*side{head, base}
			}
			done := 0
			for _, g := range groups {
				// The position inside the group advances with every result
				// line the benchmark process prints; a group with a base
				// runs once per side.
				lines := 0
				report := func() {
					at := done + 1 + lines/max(1, len(order))
					if g.leaves[0].Baseline {
						at = done + 1 + lines
					}
					at = min(at, done+len(g.leaves))
					r.setPhase(j, fmt.Sprintf("pass %d (round %d, seed %s): %s/%s %d/%d", passes+1, round+1, seed, g.engine, g.object, at, len(leaves)))
					r.setProgress(progress{Pass: passes + 1, Round: round + 1, Seed: seed, Leaf: at, Leaves: len(leaves), Passes: passes, Measured: measured + time.Since(passStart), LastPass: lastPass, PlannedPasses: planned()})
				}
				r.onResult = func() { lines++; report() }
				report()
				sides := order
				if g.leaves[0].Baseline {
					sides = []*side{head}
				}
				bySide := map[*side][]sample{}
				if stored != nil && len(sides) == 2 {
					// One-sided: the head, then the base only where needed.
					hs, err := r.measureGroup(ctx, j, head, seed, g.pkg, g.engine, g.object, g.leaves, iters, passes, seen, jobDir, logw)
					if err != nil {
						return passes, fmt.Errorf("head %s/%s seed %s: %w", g.engine, g.object, seed, err)
					}
					bs, err := r.baseFor(ctx, j, base, head, seed, g.pkg, g.engine, g.object, g.leaves, hs, stored, iters, passes, seen, jobDir, logw)
					if err != nil {
						return passes, fmt.Errorf("base %s/%s seed %s: %w", g.engine, g.object, seed, err)
					}
					bySide[head], bySide[base] = hs, bs
				} else {
					for _, s := range sides {
						samples, err := r.measureGroup(ctx, j, s, seed, g.pkg, g.engine, g.object, g.leaves, iters, passes, seen, jobDir, logw)
						if err != nil {
							return passes, fmt.Errorf("%s %s/%s seed %s: %w", s.name, g.engine, g.object, seed, err)
						}
						bySide[s] = samples
					}
				}
				if stored == nil && len(sides) == 2 {
					if err := r.reconcile(ctx, j, base, head, seed, g.leaves, iters, passes, bySide[base], bySide[head], seen, jobDir, logw); err != nil {
						return passes, fmt.Errorf("%s/%s seed %s: %w", g.engine, g.object, seed, err)
					}
				}
				for _, s := range sides {
					if err := r.store.addSamples(bySide[s]); err != nil {
						return passes, err
					}
					r.jobSamples = append(r.jobSamples, bySide[s]...)
				}
				r.onResult = nil
				done += len(g.leaves)
				if ctx.Err() != nil {
					return passes, ctx.Err()
				}
			}
			passes++
			lastPass = time.Since(passStart)
			measured += lastPass
			logw("pass %d (round %d, seed %s, %d/%d) measured in %s", passes, round+1, seed, i+1, len(r.seeds), lastPass.Round(time.Second))
		}
	}
}

// itersSpec writes the BENCH_ITERS value for a set of leaves.
func itersSpec(leaves []leaf, iters map[string]int) string {
	var parts []string
	for _, l := range leaves {
		parts = append(parts, l.Op+"="+strconv.Itoa(iters[l.key()]))
	}
	return strings.Join(parts, ",")
}

// measureGroup runs every operation of one engine and object in one
// process on one side, with the fixed iteration counts, and returns a
// sample per operation. A run during which the measured cpu lost more than
// stealShare of the wall time to the hypervisor is repeated once as a
// whole. An operation whose cycles (time, without counters) deviate from
// the median of this side's earlier runs under the same seed by more than
// the outlier threshold is repeated once on its own, and that run is kept
// whatever it shows.
func (r *runner) measureGroup(ctx context.Context, j *job, s *side, seed, pkg, engine, object string, leaves []leaf, iters map[string]int, pass int, seen map[string][]float64, jobDir string, logw func(string, ...any)) ([]sample, error) {
	logName := fmt.Sprintf("%s-p%d-%s-%s.txt", s.name, pass, seed, pkg)
	pattern := benchRegex("BenchmarkReal/" + engine + "/" + object)
	var got map[string]sample
	// A run is repeated once when the hypervisor took too much of it, and
	// up to three times when a thread started during an all-thread count.
	repeatedSteal := false
	for attempt := 0; ; attempt++ {
		t0 := time.Now()
		out, steal, err := r.runBinary(ctx, s, pkg, seed, pattern, "1x", itersSpec(leaves, iters), filepath.Join(jobDir, logName), r.passEnv(engine, pass, seed)...)
		if err != nil {
			return nil, err
		}
		got = parseSamples(j, s, seed, pass, leaves, out, steal)
		drift := false
		for _, sm := range got {
			drift = drift || sm.Extra["thread-drift"] > 0
		}
		if drift && attempt < 3 {
			logw("%s %s/%s seed %s: a thread started during the count, repeating", s.name, engine, object, seed)
			continue
		}
		if stolen := time.Duration(steal) * stealTick; float64(stolen) <= stealShare*float64(time.Since(t0)) || repeatedSteal {
			break
		}
		repeatedSteal = true
		logw("%s %s/%s seed %s: %d steal ticks during the run, repeating", s.name, engine, object, seed, steal)
	}
	var samples []sample
	for _, l := range leaves {
		sm, ok := got[l.name()]
		if !ok {
			return nil, fmt.Errorf("%s produced no result", l.name())
		}
		key := l.key() + "|" + s.name + "|" + seed
		if prior := seen[key]; len(prior) > 0 && r.cfg.outlierPct > 0 {
			med := median(prior)
			if dev := math.Abs(sampleKey(sm)-med) / med * 100; med > 0 && dev > r.cfg.outlierPct {
				logw("%s %s seed %s: %.1f%% off the side's median, repeating", s.name, l.key(), seed, dev)
				again, err := r.measureLeaf(ctx, j, s, seed, l, iters, pass, jobDir)
				if err != nil {
					return nil, err
				}
				sm = again
			}
		}
		seen[key] = append(seen[key], sampleKey(sm))
		samples = append(samples, sm)
	}
	return samples, nil
}

// stealTick is the length of one steal tick of /proc/stat; stealShare is
// the share of a run's wall time the measured cpu may lose before the run
// is repeated: the measured thread's own wake-up latencies after the
// collections amount to a few ticks per minute and are no disturbance.
const (
	stealTick  = 10 * time.Millisecond
	stealShare = 0.01
)

// Diagnostic runs: an operation whose two sides executed the same
// instructions in clearly different cycles under one layout is run once
// more per side with the counters outside the rotation, to show where the
// cycles went. The runs are untimed extras, marked "diag", and take no
// part in the statistics.
const (
	diagCycles = 5.0 // percent the cycles of the sides differ by at least
	diagMax    = 6   // operations diagnosed per job at most
)

var diagSets = []string{"D1", "D2"}

func (r *runner) diagnose(ctx context.Context, j *job, head, base *side, jobDir string, logw func(string, ...any)) {
	if !r.cfg.counterPairs || base.sha == "" {
		return
	}
	type cand struct {
		l     leaf
		seed  string
		pass  int
		delta float64
	}
	bases := map[string]sample{}
	for _, sm := range r.jobSamples {
		if sm.Side == "base" {
			bases[sm.Engine+"/"+sm.Object+"/"+sm.Op+"|"+sm.Seed] = sm
		}
	}
	best := map[string]cand{}
	for _, h := range r.jobSamples {
		key := h.Engine + "/" + h.Object + "/" + h.Op
		b, ok := bases[key+"|"+h.Seed]
		if h.Side != "head" || !ok || strings.HasSuffix(h.Engine, "Async") || b.Cycles <= 0 || h.Cycles <= 0 || b.Instrs <= 0 || h.Instrs <= 0 {
			continue
		}
		dc := math.Abs(h.Cycles/b.Cycles-1) * 100
		di := math.Abs(h.Instrs/b.Instrs-1) * 100
		if dc < diagCycles || di >= instrMoved || dc <= best[key].delta {
			continue
		}
		best[key] = cand{l: r.jobLeaves[key], seed: h.Seed, pass: h.Pass, delta: dc}
	}
	cands := make([]cand, 0, len(best))
	for _, c := range best {
		cands = append(cands, c)
	}
	sort.Slice(cands, func(a, b int) bool { return cands[a].delta > cands[b].delta })
	if len(cands) > diagMax {
		cands = cands[:diagMax]
	}
	for i, c := range cands {
		r.setPhase(j, fmt.Sprintf("diagnostic run %d/%d: %s seed %s", i+1, len(cands), c.l.key(), c.seed))
		logw("%s seed %s: same instructions, cycles differ by %.1f%%: diagnostic runs", c.l.key(), c.seed, c.delta)
		for _, set := range diagSets {
			for _, s := range []*side{base, head} {
				out, steal, err := r.runBinary(ctx, s, c.l.Pkg, c.seed, benchRegex(c.l.name()), "1x", itersSpec([]leaf{c.l}, r.jobIters),
					filepath.Join(jobDir, fmt.Sprintf("%s-diag-%s-%s.txt", s.name, c.seed, c.l.Pkg)), "BENCH_COUNTERS="+set)
				if err != nil {
					logw("diagnostic run: %v", err)
					return
				}
				sm, ok := parseSamples(j, s, c.seed, c.pass, []leaf{c.l}, out, steal)[c.l.name()]
				if !ok {
					continue
				}
				if sm.Extra == nil {
					sm.Extra = map[string]float64{}
				}
				sm.Extra["diag"] = 1
				if err := r.store.addSamples([]sample{sm}); err != nil {
					logw("diagnostic run: %v", err)
					return
				}
			}
		}
	}
}

// resultLines collects the output of a benchmark process and calls fn for
// every result line as it arrives.
type resultLines struct {
	buf  *bytes.Buffer
	fn   func()
	line []byte
}

func (w *resultLines) Write(p []byte) (int, error) {
	w.buf.Write(p)
	if w.fn == nil {
		return len(p), nil
	}
	for _, c := range p {
		if c != '\n' {
			w.line = append(w.line, c)
			continue
		}
		if bytes.HasPrefix(w.line, []byte("Benchmark")) && bytes.Contains(w.line, []byte("ns/op")) {
			w.fn()
		}
		w.line = w.line[:0]
	}
	return len(p), nil
}

// counterPairs are the names of the two pairs of further counters the
// passes alternate between.
var counterPairs = []string{"A", "B"}

// passEnv is the environment of a measurement of the pass: the counter
// pair (alternating by pass, and starting with the other pair in a rerun,
// whose seeds are a thousand higher, so that a pair is not tied to the
// same layouts), all threads counted for an async engine, and the memory
// figures in the first pass.
func (r *runner) passEnv(engine string, pass int, seed string) []string {
	var env []string
	if r.cfg.counterPairs {
		n, _ := strconv.Atoi(seed)
		env = append(env, "BENCH_COUNTERS="+counterPairs[(pass+n/1000)%len(counterPairs)])
	}
	if strings.HasSuffix(engine, "Async") {
		env = append(env, "BENCH_ALL_THREADS=1")
	}
	if pass == 0 {
		env = append(env, "BENCH_MEMORY=1")
	}
	return env
}

// measureLeaf runs one operation alone on one side and returns its sample.
func (r *runner) measureLeaf(ctx context.Context, j *job, s *side, seed string, l leaf, iters map[string]int, pass int, jobDir string) (sample, error) {
	logName := fmt.Sprintf("%s-p%d-%s-%s.txt", s.name, pass, seed, l.Pkg)
	out, steal, err := r.runBinary(ctx, s, l.Pkg, seed, benchRegex(l.name()), "1x", itersSpec([]leaf{l}, iters), filepath.Join(jobDir, logName), r.passEnv(l.Engine, pass, seed)...)
	if err != nil {
		return sample{}, err
	}
	sm, ok := parseSamples(j, s, seed, pass, []leaf{l}, out, steal)[l.name()]
	if !ok {
		return sample{}, fmt.Errorf("%s produced no result", l.name())
	}
	return sm, nil
}

// parseSamples turns a benchmark run's output into samples of the wanted
// leaves. The async hashers materialize worker buffers as scheduling
// demands, so their allocation counts are not a property of the code and
// are left out.
func parseSamples(j *job, s *side, seed string, pass int, leaves []leaf, out []byte, steal int) map[string]sample {
	want := map[string]leaf{}
	for _, l := range leaves {
		want[l.name()] = l
	}
	got := map[string]sample{}
	for _, bl := range parseBenchOutput(out) {
		l, ok := want[bl.name]
		if !ok {
			continue
		}
		sm := sample{JobID: j.ID, Side: s.name, Engine: l.Engine, Object: l.Object, Op: l.Op, Seed: seed, Pass: pass,
			Iters: bl.iters, Ns: bl.ns, Bytes: bl.bytes, Allocs: bl.allocs, Cycles: bl.cycles, Instrs: bl.instrs, Steal: steal, Extra: bl.extra}
		if strings.HasSuffix(l.Engine, "Async") {
			sm.Bytes, sm.Allocs = 0, 0
		}
		got[bl.name] = sm
	}
	return got
}

// reconcile compares the two sides of a group measured minutes apart under
// the same layout: an operation whose cycles (time, without counters)
// differ between the sides by more than the outlier threshold while the
// instruction counts agree ran into a disturbance on one side, since the
// same instructions cannot legitimately cost that much more; both sides
// are measured again once and those runs are kept whatever they show.
func (r *runner) reconcile(ctx context.Context, j *job, base, head *side, seed string, leaves []leaf, iters map[string]int, pass int, bs, hs []sample, seen map[string][]float64, jobDir string, logw func(string, ...any)) error {
	if r.cfg.outlierPct <= 0 {
		return nil
	}
	for i, l := range leaves {
		b, h := bs[i], hs[i]
		kb, kh := sampleKey(b), sampleKey(h)
		if kb <= 0 || kh <= 0 {
			continue
		}
		dev := math.Abs(kh-kb) / math.Min(kb, kh) * 100
		if dev <= r.cfg.outlierPct {
			continue
		}
		if b.Instrs > 0 && h.Instrs > 0 && math.Abs(h.Instrs-b.Instrs)/b.Instrs*100 > instrsAgreePct {
			continue
		}
		logw("%s seed %s: sides differ by %.1f%% with the same instructions, measuring both again", l.key(), seed, dev)
		nb, err := r.measureLeaf(ctx, j, base, seed, l, iters, pass, jobDir)
		if err != nil {
			return err
		}
		nh, err := r.measureLeaf(ctx, j, head, seed, l, iters, pass, jobDir)
		if err != nil {
			return err
		}
		bs[i], hs[i] = nb, nh
		for _, x := range []struct {
			s  *side
			sm sample
		}{{base, nb}, {head, nh}} {
			key := l.key() + "|" + x.s.name + "|" + seed
			if n := len(seen[key]); n > 0 {
				seen[key][n-1] = sampleKey(x.sm)
			}
		}
	}
	return nil
}

// instrsAgreePct is how far the instruction counts of two runs of the same
// binary may differ and still count as the same work.
const instrsAgreePct = 0.5

// sampleKey is the quantity the outlier rule judges: cycles when counted,
// else time.
func sampleKey(sm sample) float64 {
	if sm.Cycles > 0 {
		return sm.Cycles
	}
	return sm.Ns
}

// cpuSteal reads the steal ticks of the measured cpu: time its vcpu was
// runnable but not running, which is the hypervisor taking the core away
// from the measured thread. The other benchmark cpus run the collector
// and the async workers only between or around measurements; their wake-up
// latencies are not a disturbance of the measurement.
func (r *runner) cpuSteal() int {
	data, err := os.ReadFile("/proc/stat")
	if err != nil {
		return 0
	}
	want := "cpu" + r.mutatorCPU()
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 9 && f[0] == want {
			n, _ := strconv.Atoi(f[8])
			return n
		}
	}
	return 0
}

func (r *runner) mutatorCPU() string {
	first := r.cfg.cpus
	if i := strings.IndexAny(first, "-,"); i > 0 {
		first = first[:i]
	}
	return strings.TrimSpace(first)
}

// runBinary runs the harness binary of one side on the benchmark cpus: no
// address space randomization, one scheduler thread for the synchronous
// engines, output appended to the log file. It returns the output and the
// steal ticks seen meanwhile.
func (r *runner) runBinary(ctx context.Context, s *side, pkg, seed, pattern, benchTime, itersEnv, logPath string, env ...string) ([]byte, int, error) {
	bin := s.bin(pkg, seed)
	if bin == "" {
		return nil, 0, fmt.Errorf("no %s binary for seed %s", pkg, seed)
	}
	procs := r.cfg.gomaxprocs
	if strings.Contains(pattern, "Async") {
		procs = r.cfg.asyncGomaxprocs
	}
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x86_64"
	}
	args := []string{arch, "-R", "taskset", "-c", r.cfg.cpus, bin, "-test.run", "^$", "-test.bench", pattern, "-test.benchmem", "-test.benchtime", benchTime, "-test.count", "1", "-test.timeout", "60m"}
	cmd := r.sandboxed(ctx, false, "setarch", args...)
	cmd.Dir = s.hdir
	// GOGC=off: the harness collects between iterations with the timer
	// stopped; the memory limit is a safety net for one-off giant iterations.
	cmd.Env = append(cmd.Env, "GOMAXPROCS="+procs, "GOGC=off", "GOMEMLIMIT="+r.cfg.memLimit, "REAL_DATA="+r.cfg.payloadDir, "BENCH_MUTATOR_CPU="+r.mutatorCPU())
	if r.cfg.godebug != "" {
		cmd.Env = append(cmd.Env, "GODEBUG="+r.cfg.godebug)
	}
	if itersEnv != "" {
		cmd.Env = append(cmd.Env, "BENCH_ITERS="+itersEnv)
	}
	cmd.Env = append(cmd.Env, env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &resultLines{buf: &out, fn: r.onResult}, &errb
	before := r.cpuSteal()
	err := cmd.Run()
	steal := r.cpuSteal() - before
	if f, ferr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644); ferr == nil {
		fmt.Fprintf(f, "# %s steal=%d\n", pattern, steal)
		_, _ = f.Write(out.Bytes())
		_, _ = f.Write(errb.Bytes())
		_ = f.Close()
	}
	if err != nil {
		return nil, steal, fmt.Errorf("%v: %s", err, trimOut(append(out.Bytes(), errb.Bytes()...)))
	}
	return out.Bytes(), steal, nil
}

// benchLine is one parsed Go benchmark result line.
type benchLine struct {
	name   string
	iters  int
	ns     float64
	bytes  float64
	allocs float64
	cycles float64
	instrs float64
	extra  map[string]float64
}

// extraUnits maps the units of the harness's further figures to the keys
// they are stored under.
var extraUnits = map[string]string{
	"br-miss/op": "br-miss", "l2-miss/op": "l2-miss", "fe-stall/op": "fe-stall", "l1d-miss/op": "l1d-miss",
	"dec-uops/op": "dec-uops", "l1i-miss/op": "l1i-miss", "dtlb-miss/op": "dtlb-miss", "itlb-miss/op": "itlb-miss",
	"threads": "threads", "thread-drift": "thread-drift", "retained-B/op": "retained", "stack-B/op": "stack",
	"faults/op": "faults", "sys-ns/op": "sys-ns",
}

func parseBenchOutput(out []byte) []benchLine {
	var lines []benchLine
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "Benchmark") {
			continue
		}
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		name := f[0]
		if i := strings.LastIndex(name, "-"); i > 0 {
			if _, err := strconv.Atoi(name[i+1:]); err == nil {
				name = name[:i]
			}
		}
		bl := benchLine{name: name}
		bl.iters, _ = strconv.Atoi(f[1])
		ok := false
		for k := 2; k+1 < len(f); k += 2 {
			v, err := strconv.ParseFloat(f[k], 64)
			if err != nil {
				continue
			}
			switch f[k+1] {
			case "ns/op":
				bl.ns = v
				ok = true
			case "B/op":
				bl.bytes = v
			case "allocs/op":
				bl.allocs = v
			case "cycles/op":
				bl.cycles = v
			case "instrs/op":
				bl.instrs = v
			case "iters":
				bl.iters = int(v)
			default:
				if key, known := extraUnits[f[k+1]]; known {
					if bl.extra == nil {
						bl.extra = map[string]float64{}
					}
					bl.extra[key] = v
				}
			}
		}
		if ok {
			lines = append(lines, bl)
		}
	}
	return lines
}

// prune clears the work directories after a job: harness builds made with
// a harness that is no longer the current one are of no use to any future
// job and go at once; of the rest, the sixteen most recently used
// worktrees and builds stay (a directory is marked used whenever a job
// takes it).
func (r *runner) prune() {
	const keep = 16
	current := map[string]bool{}
	for i := range subjects {
		if h, err := subjectHash(r.cfg.harnessDir, &subjects[i]); err == nil {
			current[h] = true
		}
	}
	for _, sub := range []string{"wt", "hb"} {
		dir := filepath.Join(r.cfg.dataDir, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		type ent struct {
			name string
			mod  time.Time
		}
		var ents []ent
		for _, e := range entries {
			info, err := e.Info()
			if err != nil {
				continue
			}
			if hash, _, ok := strings.Cut(e.Name(), "-"); sub == "hb" && ok && len(current) > 0 && !current[hash] {
				_ = os.RemoveAll(filepath.Join(dir, e.Name()))
				continue
			}
			ents = append(ents, ent{e.Name(), info.ModTime()})
		}
		sort.Slice(ents, func(i, k int) bool { return ents[i].mod.After(ents[k].mod) })
		for i := keep; i < len(ents); i++ {
			path := filepath.Join(dir, ents[i].name)
			if sub == "wt" {
				r.git.removeWorktree(path)
			} else {
				_ = os.RemoveAll(path)
			}
		}
	}
}

// used marks a work directory as just used, for prune.
func used(dir string) {
	now := time.Now()
	_ = os.Chtimes(dir, now, now)
}

// writeReport writes a plain-text table next to the raw logs.
func writeReport(path string, j *job, results []result) error {
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	var b strings.Builder
	fmt.Fprintf(&b, "job %d  %s  runner %s\nhead: %s %s\nbase: %s %s\n\n", j.ID, j.Kind, j.Runner, j.HeadSHA, j.HeadDesc, j.BaseSHA, j.BaseDesc)
	fmt.Fprintf(&b, "%-16s %-14s %-24s %13s %13s %9s %19s %7s %12s %9s %10s %9s %3s %6s\n", "engine", "object", "op", "base", "head", "time", "95% CI", "p", "B/op", "delta", "allocs/op", "delta", "n", "iters")
	for _, res := range results {
		if res.Baseline {
			fmt.Fprintf(&b, "%-16s %-14s %-24s %13s %13s %9s %19s %7s %12.0f %9s %10.0f %9s %3d %6d\n", res.Engine, res.Object, res.Op, "", fmtNs(res.Ns.Head), "", "", "", res.Bytes.Head, "", res.Allocs.Head, "", res.N, res.Iters)
			continue
		}
		fmt.Fprintf(&b, "%-16s %-14s %-24s %13s %13s %+8.2f%% [%+7.2f%% %+7.2f%%] %7.3f %12.0f %+8.2f%% %10.0f %+8.2f%% %3d %6d\n", res.Engine, res.Object, res.Op, fmtNs(res.Ns.Base), fmtNs(res.Ns.Head), res.Ns.Delta, res.Ns.Lo, res.Ns.Hi, res.Ns.P, res.Bytes.Head, res.Bytes.Delta, res.Allocs.Head, res.Allocs.Delta, res.N, res.Iters)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
