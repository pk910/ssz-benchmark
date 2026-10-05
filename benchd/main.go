// benchd benchmarks every new commit of dynamic-ssz against its base (the
// pull request base, or the main branch) with a fixed harness on a real,
// extended mainnet payload, on an isolated CPU, keeps every sample and
// serves the comparisons per operation. When no commit waits it compares
// the main branch against the latest release, with a self comparison every
// few rounds to measure the noise floor.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type config struct {
	repoURL          string
	github           string // owner/name for the pull request lookup
	dataDir          string
	toolchainDir     string // toolchains of the adapters of other languages, installed by their builds
	harnessDir       string
	packages         []string // harness packages (forks), each a benchmark binary
	payloadDir       string
	listen           string
	cpus             string
	gomaxprocs       string // synchronous engines: one P, GC serialized with the measured loop
	asyncGomaxprocs  string // engines named *Async: one P per core of the cpuset
	memLimit         string
	godebug          string
	seeds            []string
	benchTime        string
	minIters         int
	minPasses        int
	publicURL        string
	approveLabel     string
	webhookSecret    string // GH_APP_WEBHOOK_SECRET; verifies webhook deliveries
	sandboxUser      string
	ghAppID          string
	ghInstallationID string
	ghAppKey         string
	targetPoll       time.Duration
	buildCPUs        string  // cpus the builds of a job may use (empty: the daemon's own)
	counterPairs     bool    // count a rotating pair of further hardware counters in every run
	oneSided         bool    // measure the base of a commit job only where an earlier run of it does not agree with the head
	baseThreshold    float64 // percent a head run may differ from the earlier base run before the base is measured
	outlierPct       float64 // re-measure a sample this far off the leaf's running median
	target           time.Duration
	poll             time.Duration
	mainBranch       string
	baselines        []string
	backfill         int
	goBin            string
	token            string
	web              bool   // web UI and API only, from the database; no fetching, no jobs
	noWeb            bool   // daemon only: fetching and jobs, no HTTP
	name             string // this machine's runner name
	controller       string // worker: URL of the controller's web service
	runnerToken      string // shared secret of the runner API
	rule             qualifyRule
}

func main() {
	cfg := &config{}
	var seeds, baselines, packages string
	flag.StringVar(&cfg.repoURL, "repo", "https://github.com/pk910/dynamic-ssz.git", "repository to mirror")
	flag.StringVar(&cfg.github, "github", "pk910/dynamic-ssz", "GitHub owner/name for the pull request lookup")
	flag.StringVar(&cfg.dataDir, "data", "/srv/benchd", "data directory (mirror, worktrees, harness builds, job logs, database)")
	flag.StringVar(&cfg.harnessDir, "harness", "/srv/benchd/harness", "harness module source")
	flag.StringVar(&cfg.toolchainDir, "toolchains", "/srv/benchd/work/toolchains", "where the builds of the adapters of other languages install their toolchains")
	flag.StringVar(&cfg.payloadDir, "payload", "/srv/benchd/res/real", "payload directory (REAL_DATA of the harness)")
	flag.StringVar(&packages, "packages", "fulu,gloas", "harness packages to build and measure, each with a types/<package> package to generate")
	flag.StringVar(&cfg.listen, "listen", ":80", "web UI listen address")
	flag.StringVar(&cfg.cpus, "cpus", "2", "cpu the benchmarks run on (taskset syntax)")
	flag.StringVar(&cfg.gomaxprocs, "gomaxprocs", "1", "GOMAXPROCS for the synchronous engines (the collector then runs serialized with the measured loop, deterministically)")
	flag.StringVar(&cfg.asyncGomaxprocs, "async-gomaxprocs", "3", "GOMAXPROCS for engines named *Async (async hashing workers need their own cores)")
	flag.StringVar(&cfg.memLimit, "memlimit", "24GiB", "GOMEMLIMIT of the benchmark processes (GOGC is off; the limit is the backstop below the machine's memory)")
	flag.StringVar(&cfg.godebug, "godebug", "madvdontneed=0", "GODEBUG of the benchmark binaries (MADV_FREE keeps freed buffers mapped, so they are not re-faulted)")
	flag.StringVar(&seeds, "seeds", "101,202,303,404", "linker layout seeds, one build per seed and side")
	flag.StringVar(&cfg.benchTime, "benchtime", "300ms", "benchtime used once per leaf to fix its iteration count")
	flag.IntVar(&cfg.minIters, "min-iters", 2, "lowest fixed iteration count per measurement")
	flag.DurationVar(&cfg.targetPoll, "target-poll", 15*time.Minute, "how often the upstream repositories of the libraries without a mirror are checked for new commits and releases")
	flag.StringVar(&cfg.sandboxUser, "sandbox-user", os.Getenv("SANDBOX_USER"), "unprivileged user the builds and benchmark processes run as (env SANDBOX_USER); empty: as the daemon, and no fork is measured")
	flag.StringVar(&cfg.approveLabel, "approve-label", "benchmark", "label that approves measuring a fork pull request at the head it is applied to (delivered by the GitHub webhook)")
	flag.StringVar(&cfg.publicURL, "public-url", os.Getenv("PUBLIC_URL"), "public root of the web UI, used in links from GitHub (env PUBLIC_URL)")
	flag.StringVar(&cfg.ghAppID, "gh-app-id", os.Getenv("GH_APP_ID"), "GitHub App id for check runs (env GH_APP_ID); empty: no checks")
	flag.StringVar(&cfg.ghInstallationID, "gh-installation-id", os.Getenv("GH_INSTALLATION_ID"), "installation id of the App on the repository (env GH_INSTALLATION_ID)")
	flag.StringVar(&cfg.ghAppKey, "gh-app-key", os.Getenv("GH_APP_KEY"), "path of the App's private key (env GH_APP_KEY)")
	flag.IntVar(&cfg.minPasses, "min-passes", 0, "passes a job runs at least, regardless of the budget (0: one per layout seed)")
	flag.StringVar(&cfg.buildCPUs, "build-cpus", "", "cpus the builds of a job run on, e.g. the benchmark cores, which are idle while a job builds (empty: the daemon's own cpus)")
	flag.BoolVar(&cfg.oneSided, "one-sided", false, "a job measures its head and takes the base from earlier runs of the base commit; the base is measured in the job only where a head run differs from the earlier one by more than -base-threshold, or none exists")
	flag.Float64Var(&cfg.baseThreshold, "base-threshold", 1, "percent a head run may differ from the earlier run of the base before the base is measured in the job (with -one-sided)")
	flag.BoolVar(&cfg.counterPairs, "counter-pairs", true, "count two further hardware counters in every run, alternating between two pairs by pass")
	flag.Float64Var(&cfg.outlierPct, "outlier", 4, "a measurement whose cycles (or time without counters) deviate this many percent from the median of the leaf's earlier measurements on the same side is repeated once")
	flag.DurationVar(&cfg.target, "target", 12*time.Minute, "measurement budget per job (passes are fitted to it)")
	flag.DurationVar(&cfg.poll, "poll", 60*time.Second, "how often the mirror is fetched")
	flag.StringVar(&cfg.mainBranch, "main", "master", "main branch")
	flag.StringVar(&baselines, "baselines", "FastSSZ", "harness engines that are library references (measured on the head binary only)")
	flag.IntVar(&cfg.backfill, "backfill", 10, "main-branch commits queued on the first run, and the most that one push to it queues")
	flag.StringVar(&cfg.goBin, "go", "/usr/local/go/bin/go", "go binary")
	flag.BoolVar(&cfg.web, "web", false, "serve the web UI and API from the database only (no fetching, no jobs)")
	flag.BoolVar(&cfg.noWeb, "no-web", false, "run the daemon only (fetching and jobs), without HTTP")
	flag.StringVar(&cfg.name, "name", "", "runner name of this machine (default: hostname)")
	flag.StringVar(&cfg.controller, "controller", "", "worker mode: URL of the controller's web service, e.g. http://controller.example (RUNNER_TOKEN in the environment)")
	flag.Float64Var(&cfg.rule.maxP95, "qualify-p95", 1.5, "worker acceptance: highest p95 of |Δ time| over the leaves of its noise job, percent")
	flag.Float64Var(&cfg.rule.maxRatio, "qualify-ratio", 0.15, "worker acceptance: largest |geomean(worker/controller time) - 1|")
	flag.Float64Var(&cfg.rule.maxSpread, "qualify-spread", 8, "worker acceptance: largest spread (CV) of the per-leaf worker/controller ratios, percent")
	flag.IntVar(&cfg.rule.maxRetries, "qualify-retries", 3, "noise jobs a worker may fail before it is rejected")
	flag.Parse()
	cfg.seeds = strings.Split(seeds, ",")
	cfg.baselines = strings.Split(baselines, ",")
	cfg.packages = strings.Split(packages, ",")
	cfg.token = os.Getenv("GITHUB_TOKEN")
	cfg.webhookSecret = os.Getenv("GH_APP_WEBHOOK_SECRET")
	cfg.runnerToken = os.Getenv("RUNNER_TOKEN")
	if cfg.name == "" {
		cfg.name, _ = os.Hostname()
	}

	configureSubjects(cfg)
	for _, d := range []string{"wt", "hb", "jobs"} {
		if err := os.MkdirAll(filepath.Join(cfg.dataDir, d), 0o755); err != nil {
			log.Fatal(err)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	git := &gitRepo{dir: filepath.Join(cfg.dataDir, "repo.git"), url: cfg.repoURL}

	if cfg.controller != "" {
		// Worker: benchmarks only, everything else lives on the controller.
		if cfg.runnerToken == "" {
			log.Fatal("worker mode needs RUNNER_TOKEN")
		}
		for _, need := range []string{filepath.Join(cfg.harnessDir, "go.mod"), filepath.Join(cfg.payloadDir, cfg.packages[0], "state.ssz")} {
			if _, err := os.Stat(need); err != nil {
				log.Fatalf("missing %s", need)
			}
		}
		if err := git.ensureMirror(); err != nil {
			log.Fatal(err)
		}
		r := &runner{cfg: cfg, store: newRemoteStore(cfg.controller, cfg.runnerToken), git: git, name: cfg.name}
		log.Printf("benchd worker %s for %s, data in %s, benchmarks on cpus %s", cfg.name, cfg.controller, cfg.dataDir, cfg.cpus)
		r.loop(ctx)
		time.Sleep(2 * time.Second)
		return
	}

	db, err := openDB(filepath.Join(cfg.dataDir, "benchd.db"))
	if err != nil {
		log.Fatal(err)
	}
	sched := &scheduler{cfg: cfg, db: db, git: git, prs: newPRCache(cfg.github, cfg.token), ready: make(chan struct{})}
	sched.prs.store = db
	local := &localStore{db: db, sched: sched, local: cfg.name, rule: cfg.rule}
	if cfg.ghAppID != "" {
		app, err := loadGHApp(cfg.ghAppID, cfg.ghInstallationID, cfg.ghAppKey)
		if err != nil {
			log.Fatalf("GitHub App: %v", err)
		}
		checks, err := newCheckReporter(app, cfg, db)
		if err != nil {
			log.Fatalf("GitHub App: %v", err)
		}
		sched.checks, local.checks = checks, checks
		sched.prs.tokenFn = func(ctx context.Context) string {
			t, err := app.installationToken(ctx)
			if err != nil {
				log.Printf("GitHub App token: %v", err)
			}
			return t
		}
		log.Printf("check runs on %s as GitHub App %s (jobs after #%d)", cfg.github, cfg.ghAppID, checks.sinceJob)
	}
	web := &webServer{cfg: cfg, db: db, sched: sched, runners: &runnerAPI{store: local, token: cfg.runnerToken}}

	if !cfg.web {
		for _, need := range []string{filepath.Join(cfg.harnessDir, "go.mod"), filepath.Join(cfg.payloadDir, cfg.packages[0], "state.ssz")} {
			if _, err := os.Stat(need); err != nil {
				log.Fatalf("missing %s", need)
			}
		}
		if err := git.ensureMirror(); err != nil {
			log.Fatal(err)
		}
		// A job interrupted by a restart is run again from scratch.
		if err := db.requeueRunning(cfg.name); err != nil {
			log.Fatal(err)
		}
		if err := db.migrateStorage(filepath.Join(cfg.dataDir, "benchd.db")); err != nil {
			log.Fatalf("storage migration: %v", err)
		}
		// The reference job is gone: one that never ran is removed.
		if _, err := db.db.Exec(`DELETE FROM jobs WHERE kind = ? AND state IN (?, ?)`, kindBaseline, stateQueued, stateSkipped); err != nil {
			log.Fatal(err)
		}
		if err := db.migrateJobKinds(cfg.mainBranch); err != nil {
			log.Fatalf("job kinds: %v", err)
		}
		if err := db.backfillCommitValues(); err != nil {
			log.Fatalf("pooled values: %v", err)
		}
		compressAllJobLogs(cfg.dataDir)
		r := &runner{cfg: cfg, store: local, git: git, name: cfg.name, sched: sched}
		sched.runner = r
		go sched.loop(ctx)
		go r.loop(ctx)
	}
	if cfg.noWeb {
		log.Printf("benchd daemon %s, data in %s, benchmarks on cpus %s", cfg.name, cfg.dataDir, cfg.cpus)
		<-ctx.Done()
		// Let a running job mark itself as queued again before exiting.
		time.Sleep(2 * time.Second)
		return
	}
	log.Printf("benchd listening on %s, data in %s", cfg.listen, cfg.dataDir)
	if err := web.serve(ctx); err != nil {
		log.Fatal(err)
	}
}
