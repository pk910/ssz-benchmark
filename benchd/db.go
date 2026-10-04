package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Job kinds.
const (
	kindCommit  = "commit"  // a new commit against its base
	kindRelease = "release" // the main branch against the latest release
	kindNoise   = "noise"   // the main branch against itself
	// kindBaseline measures the reference libraries (other SSZ
	// implementations) on the same payload, on their own schedule.
	kindBaseline = "baseline"
)

// Job states.
const (
	stateQueued  = "queued"
	stateRunning = "running"
	stateDone    = "done"
	stateFailed  = "failed"
	// stateSkipped is a queued commit job a newer commit of the same branch
	// made pointless before it ran.
	stateSkipped = "skipped"
)

type job struct {
	ID        int64
	Created   time.Time
	Started   *time.Time
	Finished  *time.Time
	State     string
	Kind      string
	Branch    string
	HeadSHA   string
	HeadDesc  string
	BaseSHA   string
	BaseDesc  string
	BaseRef   string
	PR        int
	Passes    int
	Error     string
	Note      string
	Seconds   float64
	GoVersion string
	Priority  int
	Harness   string // hash of the harness source the job ran
	Runner    string // machine that ran (or is to run) the job
	// Seeds are the layout seeds this run measures under, set when the job
	// is handed out; empty means the runner's configured seeds.
	Seeds []string
}

// leaf identifies one benchmark: BenchmarkReal/<Engine>/<Object>/<Op> of
// one harness package.
type leaf struct {
	Pkg      string // harness package (fork) whose binary has the benchmark
	Engine   string
	Object   string
	Op       string
	Baseline bool // library reference, measured on the head binary only
}

func (l leaf) name() string { return "BenchmarkReal/" + l.Engine + "/" + l.Object + "/" + l.Op }
func (l leaf) key() string  { return l.Engine + "/" + l.Object + "/" + l.Op }

type sample struct {
	JobID  int64
	Side   string // "base" or "head"
	Engine string
	Object string
	Op     string
	Seed   string
	Pass   int
	Iters  int
	Ns     float64 // ns/op
	Bytes  float64 // B/op
	Allocs float64 // allocs/op
	Cycles float64 // cpu cycles/op of the measured thread (0 without a PMU)
	Instrs float64 // retired instructions/op (0 without a PMU)
	Steal  int     // steal ticks on the benchmark cpu during the measurement
	// Extra holds what only some runs measure, per operation: the counter
	// pair of the pass ("br-miss", "l2-miss", "fe-stall", "l1d-miss"), the
	// threads counted when all were ("threads"), the memory figures of the
	// first pass ("retained", "stack"), and the counters of a diagnostic
	// run, which is marked "diag" and takes no part in the statistics.
	Extra map[string]float64 `json:",omitempty"`
}

// isDiag reports whether the sample is a diagnostic run.
func (sm sample) isDiag() bool { return sm.Extra["diag"] > 0 }

// extraStat is the median of an extra value on each side over the runs
// that measured it.
type extraStat struct {
	Base, Head float64
	N          int // runs per side
}

// metric is one measured quantity of a leaf compared between the sides.
type metric struct {
	Base, Head       float64 // means
	MedBase, MedHead float64
	CVBase, CVHead   float64 // coefficient of variation in percent
	Delta            float64 // (head-base)/base in percent
	MedDelta         float64 // (median head - median base)/median base in percent
	Lo, Hi           float64 // 95% interval of Delta in percent
	P                float64 // Welch p-value
	DMin, DMax       float64 // smallest and largest delta of a single run (base and head of the same pass), percent
	HMin, HMax       float64 // smallest and largest head value of a single run
	// The comparison by pass: base and head of one pass are linked with the
	// same seed and measured minutes apart. PMed is the median of the
	// per-pass deltas (percent), PSpread their spread (a standard deviation
	// estimated from the median absolute deviation), PAgree
	// how many of the PN passes point the way of the median.
	PMed, PSpread float64
	PAgree, PN    float64
	// The middle half of the per-pass deltas (PQ1..PQ3, percent) and of the
	// head values (HQ1..HQ3): the body of the chart candles.
	PQ1, PQ3, HQ1, HQ3 float64
}

type result struct {
	JobID    int64
	Engine   string
	Object   string
	Op       string
	Baseline bool
	N        int // measurements per side
	Iters    int
	Ns       metric
	Bytes    metric
	Allocs   metric
	Cycles   metric
	Instrs   metric
	Steal    int // steal ticks seen over all measurements of the leaf
	// Extra is computed from the samples for the job page and not stored.
	Extra map[string]extraStat `json:",omitempty"`
}

// runnerInfo is a benchmark machine known to the controller.
type runnerInfo struct {
	Name         string
	State        string
	FirstSeen    time.Time
	LastSeen     time.Time
	Note         string
	Checks       int
	NoiseMedian  float64
	NoiseP95     float64
	RatioGeomean float64 // worker time / controller time, geomean over leaves
	RatioSpread  float64 // CV of the per-leaf ratios, percent
	ReferenceJob int64
	QualifyJob   int64
}

type store struct {
	db      *sql.DB
	dataDir string

	// The noise floor, kept until another noise job finishes.
	nfMu  sync.Mutex
	nfKey string
	nf    noiseFloor
}

func openDB(path string) (*store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created INTEGER NOT NULL,
			started INTEGER,
			finished INTEGER,
			state TEXT NOT NULL,
			kind TEXT NOT NULL,
			branch TEXT NOT NULL DEFAULT '',
			head_sha TEXT NOT NULL,
			head_desc TEXT NOT NULL DEFAULT '',
			base_sha TEXT NOT NULL,
			base_desc TEXT NOT NULL DEFAULT '',
			base_ref TEXT NOT NULL DEFAULT '',
			pr INTEGER NOT NULL DEFAULT 0,
			passes INTEGER NOT NULL DEFAULT 0,
			error TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			seconds REAL NOT NULL DEFAULT 0,
			go_version TEXT NOT NULL DEFAULT '',
			priority INTEGER NOT NULL DEFAULT 0,
			harness TEXT NOT NULL DEFAULT '',
			runner TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS jobs_state ON jobs(state, priority, id)`,
		`CREATE INDEX IF NOT EXISTS jobs_head ON jobs(head_sha)`,
		`CREATE INDEX IF NOT EXISTS jobs_base ON jobs(base_sha)`,
		`CREATE TABLE IF NOT EXISTS samples (
			job_id INTEGER NOT NULL,
			side TEXT NOT NULL,
			engine TEXT NOT NULL,
			object TEXT NOT NULL,
			op TEXT NOT NULL,
			seed TEXT NOT NULL,
			pass INTEGER NOT NULL,
			iters INTEGER NOT NULL,
			ns REAL NOT NULL,
			bytes REAL NOT NULL,
			allocs REAL NOT NULL,
			steal INTEGER NOT NULL DEFAULT 0,
			cycles REAL NOT NULL DEFAULT 0,
			instrs REAL NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS samples_job ON samples(job_id, engine, object, op)`,
		`CREATE TABLE IF NOT EXISTS builds (
			job_id INTEGER NOT NULL,
			side TEXT NOT NULL,
			sha TEXT NOT NULL,
			pkg TEXT NOT NULL,
			bin_bytes INTEGER NOT NULL,
			text_bytes INTEGER NOT NULL,
			gen_bytes INTEGER NOT NULL,
			build_seconds REAL NOT NULL,
			PRIMARY KEY (job_id, side, pkg)
		)`,
		`CREATE TABLE IF NOT EXISTS job_samples (
			job_id INTEGER PRIMARY KEY,
			data BLOB NOT NULL
		)`,
		resultsSchema("results"),
		`CREATE INDEX IF NOT EXISTS results_leaf ON results(object, op, engine, job_id)`,
		`CREATE TABLE IF NOT EXISTS job_checks (
			job_id INTEGER PRIMARY KEY,
			check_id INTEGER NOT NULL,
			digest TEXT NOT NULL,
			final INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS pr_approvals (
			pr INTEGER NOT NULL,
			head_sha TEXT NOT NULL,
			by TEXT NOT NULL,
			at INTEGER NOT NULL,
			PRIMARY KEY (pr, head_sha)
		)`,
		`CREATE TABLE IF NOT EXISTS seen_refs (
			ref TEXT PRIMARY KEY,
			sha TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS bench_iters (
			engine TEXT NOT NULL,
			object TEXT NOT NULL,
			op TEXT NOT NULL,
			iters INTEGER NOT NULL,
			PRIMARY KEY (engine, object, op)
		)`,
		`CREATE TABLE IF NOT EXISTS runners (
			name TEXT PRIMARY KEY,
			state TEXT NOT NULL,
			first_seen INTEGER NOT NULL,
			last_seen INTEGER NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			checks INTEGER NOT NULL DEFAULT 0,
			noise_median REAL NOT NULL DEFAULT 0,
			noise_p95 REAL NOT NULL DEFAULT 0,
			ratio_geomean REAL NOT NULL DEFAULT 0,
			ratio_spread REAL NOT NULL DEFAULT 0,
			reference_job INTEGER NOT NULL DEFAULT 0,
			qualify_job INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS kv (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS harness_leaves (
			harness TEXT NOT NULL,
			pkg TEXT NOT NULL,
			engine TEXT NOT NULL,
			object TEXT NOT NULL,
			op TEXT NOT NULL,
			baseline INTEGER NOT NULL,
			PRIMARY KEY (harness, engine, object, op)
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("schema: %w", err)
		}
	}
	migrations := []string{
		`ALTER TABLE jobs ADD COLUMN runner TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE samples ADD COLUMN cycles REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE samples ADD COLUMN instrs REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE samples ADD COLUMN extra TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range migrations {
		_, _ = db.Exec(stmt) // fails when the column exists
	}
	s := &store{db: db, dataDir: filepath.Dir(path)}
	if err := s.refreshResults("results:robust-spread"); err != nil {
		return nil, err
	}
	return s, nil
}

// refreshResults recomputes the results of every finished job once per
// schema change named by key, pooled with the earlier runs of the pair as
// at the time the job finished, so columns added later are filled.
func (s *store) refreshResults(key string) error {
	if v, _ := s.getKV(key); v != "" {
		return nil
	}
	jobs, err := s.listJobs(1<<30, "")
	if err != nil {
		return err
	}
	for _, j := range jobs {
		if j.State != stateDone {
			continue
		}
		pooled, err := s.samplesFor(j.ID)
		if err != nil {
			return err
		}
		ids, _ := s.pairJobIDs(j.HeadSHA, j.BaseSHA, j.Harness, j.Runner)
		for _, id := range ids {
			if id >= j.ID {
				continue
			}
			prev, err := s.samplesFor(id)
			if err != nil {
				return err
			}
			pooled = append(pooled, prev...)
		}
		if len(pooled) == 0 {
			continue
		}
		if err := s.replaceResults(j.ID, summarize(pooled)); err != nil {
			return err
		}
	}
	return s.setKV(key, time.Now().UTC().Format(time.RFC3339))
}

// requeueRunning puts jobs a stopped local daemon left running back into
// the queue; jobs of remote workers stay with them.
func (s *store) requeueRunning(local string) error {
	_, err := s.db.Exec(`UPDATE jobs SET state = ?, started = NULL WHERE state = ? AND (runner = ? OR runner = '')`, stateQueued, stateRunning, local)
	return err
}

func (s *store) requeueJob(id int64) error {
	_, err := s.db.Exec(`UPDATE jobs SET state = ?, started = NULL WHERE id = ? AND state = ?`, stateQueued, id, stateRunning)
	return err
}

func (s *store) seenRef(ref string) (string, error) {
	var sha string
	err := s.db.QueryRow(`SELECT sha FROM seen_refs WHERE ref = ?`, ref).Scan(&sha)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return sha, err
}

func (s *store) seenCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM seen_refs`).Scan(&n)
	return n, err
}

func (s *store) markRef(ref, sha string) error {
	_, err := s.db.Exec(`INSERT INTO seen_refs(ref, sha) VALUES(?, ?) ON CONFLICT(ref) DO UPDATE SET sha = excluded.sha`, ref, sha)
	return err
}

// hasJobFor reports whether a commit job for the pair is queued, running or done.
func (s *store) hasJobFor(headSHA, baseSHA string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE kind = ? AND head_sha = ? AND base_sha = ? AND state != ?`, kindCommit, headSHA, baseSHA, stateFailed).Scan(&n)
	return n > 0, err
}

func (s *store) insertJob(j *job) error {
	res, err := s.db.Exec(`INSERT INTO jobs(created, state, kind, branch, head_sha, head_desc, base_sha, base_desc, base_ref, pr, note, priority, runner)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		time.Now().Unix(), stateQueued, j.Kind, j.Branch, j.HeadSHA, j.HeadDesc, j.BaseSHA, j.BaseDesc, j.BaseRef, j.PR, j.Note, j.Priority, j.Runner)
	if err != nil {
		return err
	}
	j.ID, _ = res.LastInsertId()
	return nil
}

const jobColumns = `id, created, started, finished, state, kind, branch, head_sha, head_desc, base_sha, base_desc, base_ref, pr, passes, error, note, seconds, go_version, priority, harness, runner`

func scanJob(row interface{ Scan(...any) error }) (*job, error) {
	j := &job{}
	var created int64
	var started, finished sql.NullInt64
	if err := row.Scan(&j.ID, &created, &started, &finished, &j.State, &j.Kind, &j.Branch, &j.HeadSHA, &j.HeadDesc, &j.BaseSHA, &j.BaseDesc, &j.BaseRef, &j.PR, &j.Passes, &j.Error, &j.Note, &j.Seconds, &j.GoVersion, &j.Priority, &j.Harness, &j.Runner); err != nil {
		return nil, err
	}
	j.Created = time.Unix(created, 0)
	if started.Valid {
		t := time.Unix(started.Int64, 0)
		j.Started = &t
	}
	if finished.Valid {
		t := time.Unix(finished.Int64, 0)
		j.Finished = &t
	}
	return j, nil
}

// nextQueued returns the queued job with the highest priority that the
// runner may take (unassigned, or assigned to it), commit jobs before idle
// jobs, oldest first.
func (s *store) nextQueued(runner string) (*job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE state = ? AND (runner = '' OR runner = ?) ORDER BY priority DESC, (kind = ?) DESC, id LIMIT 1`, stateQueued, runner, kindCommit))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

// claimJob marks a queued job running for the runner; nil when another
// runner took it first.
func (s *store) claimJob(j *job, runner string) (*job, error) {
	if j == nil {
		return nil, nil
	}
	res, err := s.db.Exec(`UPDATE jobs SET state = ?, started = ?, runner = ? WHERE id = ? AND state IN (?, ?) AND (runner = '' OR runner = ?)`,
		stateRunning, time.Now().Unix(), runner, j.ID, stateQueued, stateRunning, runner)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, nil
	}
	return s.getJob(j.ID)
}

// runningOrQueuedFor returns the job a runner holds or is assigned, if any.
func (s *store) runningOrQueuedFor(runner string) (*job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE runner = ? AND state IN (?, ?) ORDER BY (state = ?) DESC, id LIMIT 1`, runner, stateRunning, stateQueued, stateRunning))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

// latestNoiseJob is the newest finished self-comparison a runner made with
// a harness version: the reference for qualifying other machines.
func (s *store) latestNoiseJob(runner, harness string) (*job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE runner = ? AND kind = ? AND state = ? AND harness = ? ORDER BY id DESC LIMIT 1`, runner, kindNoise, stateDone, harness))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

func (s *store) getRunner(name string) (*runnerInfo, error) {
	r := &runnerInfo{}
	var first, last int64
	err := s.db.QueryRow(`SELECT name, state, first_seen, last_seen, note, checks, noise_median, noise_p95, ratio_geomean, ratio_spread, reference_job, qualify_job FROM runners WHERE name = ?`, name).
		Scan(&r.Name, &r.State, &first, &last, &r.Note, &r.Checks, &r.NoiseMedian, &r.NoiseP95, &r.RatioGeomean, &r.RatioSpread, &r.ReferenceJob, &r.QualifyJob)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	r.FirstSeen, r.LastSeen = time.Unix(first, 0), time.Unix(last, 0)
	return r, nil
}

func (s *store) putRunner(r *runnerInfo) error {
	_, err := s.db.Exec(`INSERT INTO runners(name, state, first_seen, last_seen, note, checks, noise_median, noise_p95, ratio_geomean, ratio_spread, reference_job, qualify_job)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET state = excluded.state, last_seen = excluded.last_seen, note = excluded.note, checks = excluded.checks,
		noise_median = excluded.noise_median, noise_p95 = excluded.noise_p95, ratio_geomean = excluded.ratio_geomean, ratio_spread = excluded.ratio_spread,
		reference_job = excluded.reference_job, qualify_job = excluded.qualify_job`,
		r.Name, r.State, r.FirstSeen.Unix(), r.LastSeen.Unix(), r.Note, r.Checks, r.NoiseMedian, r.NoiseP95, r.RatioGeomean, r.RatioSpread, r.ReferenceJob, r.QualifyJob)
	return err
}

func (s *store) listRunners() ([]*runnerInfo, error) {
	rows, err := s.db.Query(`SELECT name FROM runners ORDER BY first_seen`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	out := []*runnerInfo{}
	for _, n := range names {
		r, err := s.getRunner(n)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, nil
}

// setRunnerStatus stores a runner's live status under status:<name>.
func (s *store) setRunnerStatus(ls liveStatus) error {
	data, err := json.Marshal(ls)
	if err != nil {
		return err
	}
	return s.setKV("status:"+ls.Runner, string(data))
}

// runnerStatuses lists every published live status.
func (s *store) runnerStatuses() ([]liveStatus, error) {
	rows, err := s.db.Query(`SELECT value FROM kv WHERE key LIKE 'status:%' ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []liveStatus{}
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var ls liveStatus
		if json.Unmarshal([]byte(raw), &ls) == nil {
			out = append(out, ls)
		}
	}
	return out, rows.Err()
}

func (s *store) queuedCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE state = ?`, stateQueued).Scan(&n)
	return n, err
}

func (s *store) startJob(id int64, goVersion, harness string) error {
	_, err := s.db.Exec(`UPDATE jobs SET state = ?, started = ?, go_version = ?, harness = ? WHERE id = ?`, stateRunning, time.Now().Unix(), goVersion, harness, id)
	return err
}

func (s *store) finishJob(id int64, state string, passes int, errText string, seconds float64) error {
	_, err := s.db.Exec(`UPDATE jobs SET state = ?, finished = ?, passes = ?, error = ?, seconds = ? WHERE id = ?`,
		state, time.Now().Unix(), passes, errText, seconds, id)
	return err
}

func (s *store) getJob(id int64) (*job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

func (s *store) scanJobs(rows *sql.Rows, err error) ([]*job, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// queuedJobs lists the queue in execution order.
func (s *store) queuedJobs() ([]*job, error) {
	return s.scanJobs(s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE state IN (?, ?) ORDER BY (state = ?) DESC, priority DESC, (kind = ?) DESC, id`, stateQueued, stateRunning, stateRunning, kindCommit))
}

// finishedJobs lists done and failed jobs, most recently finished first.
func (s *store) finishedJobs(limit int) ([]*job, error) {
	return s.scanJobs(s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE state IN (?, ?) ORDER BY finished DESC, id DESC LIMIT ?`, stateDone, stateFailed, limit))
}

func (s *store) listJobs(limit int, kind string) ([]*job, error) {
	if kind != "" {
		return s.scanJobs(s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE kind = ? ORDER BY id DESC LIMIT ?`, kind, limit))
	}
	return s.scanJobs(s.db.Query(`SELECT `+jobColumns+` FROM jobs ORDER BY id DESC LIMIT ?`, limit))
}

// prJobs lists the commit jobs of a pull request, oldest first.
func (s *store) prJobs(pr int) ([]*job, error) {
	return s.scanJobs(s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE pr = ? AND kind = ? ORDER BY id`, pr, kindCommit))
}

// jobsWithCommit lists finished jobs that measured the commit on either side.
func (s *store) jobsWithCommit(sha string) ([]*job, error) {
	return s.scanJobs(s.db.Query(`SELECT `+jobColumns+` FROM jobs WHERE state = ? AND (head_sha = ? OR base_sha = ?) ORDER BY id DESC`, stateDone, sha, sha))
}

// pairJobIDs lists the finished jobs that measured the same pair with the
// same harness, oldest first.
// pairBuildFailed reports whether a job of this pair already failed to
// build (the harness does not compile against that side), in which case
// the pair is not worth another attempt until one of the sides changes.
func (s *store) pairBuildFailed(headSHA, baseSHA string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE state = ? AND head_sha = ? AND base_sha = ? AND error LIKE 'build %'`, stateFailed, headSHA, baseSHA).Scan(&n)
	return n > 0, err
}

func (s *store) pairJobIDs(headSHA, baseSHA, harness, runner string) ([]int64, error) {
	rows, err := s.db.Query(`SELECT id FROM jobs WHERE state = ? AND head_sha = ? AND base_sha = ? AND harness = ? AND runner = ? ORDER BY id`, stateDone, headSHA, baseSHA, harness, runner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// approvePR records that measuring this head of a pull request was
// approved, and by whom.
func (s *store) approvePR(pr int, headSHA, by string) error {
	_, err := s.db.Exec(`INSERT INTO pr_approvals(pr, head_sha, by, at) VALUES(?, ?, ?, ?)
		ON CONFLICT(pr, head_sha) DO UPDATE SET by = excluded.by, at = excluded.at`, pr, headSHA, by, time.Now().Unix())
	return err
}

// prApproval returns who approved this head of the pull request, or "".
func (s *store) prApproval(pr int, headSHA string) (string, error) {
	var by string
	err := s.db.QueryRow(`SELECT by FROM pr_approvals WHERE pr = ? AND head_sha = ?`, pr, headSHA).Scan(&by)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return by, err
}

// jobCheck returns the check run of a job: its id on GitHub, the digest of
// the payload last sent and whether that was the final one.
func (s *store) jobCheck(jobID int64) (checkID int64, digest string, final bool, err error) {
	err = s.db.QueryRow(`SELECT check_id, digest, final FROM job_checks WHERE job_id = ?`, jobID).Scan(&checkID, &digest, &final)
	if err == sql.ErrNoRows {
		return 0, "", false, nil
	}
	return checkID, digest, final, err
}

func (s *store) setJobCheck(jobID, checkID int64, digest string, final bool) error {
	_, err := s.db.Exec(`INSERT INTO job_checks(job_id, check_id, digest, final) VALUES(?, ?, ?, ?)
		ON CONFLICT(job_id) DO UPDATE SET check_id = excluded.check_id, digest = excluded.digest, final = excluded.final`, jobID, checkID, digest, final)
	return err
}

func (s *store) maxJobID() (int64, error) {
	var id sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(id) FROM jobs`).Scan(&id)
	return id.Int64, err
}

// pairRuns counts the finished runs of a pair on a machine.
func (s *store) pairRuns(headSHA, baseSHA, runner string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE state = ? AND head_sha = ? AND base_sha = ? AND runner = ?`, stateDone, headSHA, baseSHA, runner).Scan(&n)
	return n, err
}

// queuedAhead counts the queued jobs that run before j.
func (s *store) queuedAhead(j *job) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE state = ? AND (priority > ? OR (priority = ? AND id < ?))`, stateQueued, j.Priority, j.Priority, j.ID).Scan(&n)
	return n, err
}

// supersedeQueued marks the queued commit jobs of a branch whose head is
// not sha as skipped and returns their ids.
func (s *store) supersedeQueued(branch, sha string) ([]int64, error) {
	rows, err := s.db.Query(`SELECT id FROM jobs WHERE state = ? AND kind = ? AND branch = ? AND head_sha != ?`, stateQueued, kindCommit, branch, sha)
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
	for _, id := range ids {
		if _, err := s.db.Exec(`UPDATE jobs SET state = ?, finished = ?, note = ? WHERE id = ? AND state = ?`, stateSkipped, time.Now().Unix(), "superseded by "+shortSHA(sha), id, stateQueued); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func (s *store) setJobPR(id int64, pr int) error {
	_, err := s.db.Exec(`UPDATE jobs SET pr = ? WHERE id = ?`, pr, id)
	return err
}

func (s *store) setJobNote(id int64, note string) error {
	_, err := s.db.Exec(`UPDATE jobs SET note = ? WHERE id = ?`, note, id)
	return err
}

// leastRefinedPair returns the finished commit comparison with the fewest
// runs (newest among equals) as a template for a refinement run.
func (s *store) leastRefinedPair() (*job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE id = (
		SELECT MAX(id) FROM jobs WHERE state = ? AND kind = ? GROUP BY head_sha, base_sha, runner ORDER BY COUNT(*) ASC, MAX(id) DESC LIMIT 1)`, stateDone, kindCommit))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

// lastJobOfKind returns the newest job of a kind in any state.
func (s *store) lastJobOfKind(kind string) (*job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE kind = ? ORDER BY id DESC LIMIT 1`, kind))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

// lastDoneJobOfKind is the newest finished job of a kind.
func (s *store) lastDoneJobOfKind(kind string) (*job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE kind = ? AND state = ? ORDER BY id DESC LIMIT 1`, kind, stateDone))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

func (s *store) countJobs(kind, state string) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM jobs WHERE kind = ? AND state = ?`, kind, state).Scan(&n)
	return n, err
}

func (s *store) insertSamples(samples []sample) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO samples(job_id, side, engine, object, op, seed, pass, iters, ns, bytes, allocs, steal, cycles, instrs, extra) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, sm := range samples {
		extra := ""
		if len(sm.Extra) > 0 {
			data, _ := json.Marshal(sm.Extra)
			extra = string(data)
		}
		if _, err := stmt.Exec(sm.JobID, sm.Side, sm.Engine, sm.Object, sm.Op, sm.Seed, sm.Pass, sm.Iters, sm.Ns, sm.Bytes, sm.Allocs, sm.Steal, sm.Cycles, sm.Instrs, extra); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *store) deleteSamples(jobID int64) error {
	_, err := s.db.Exec(`DELETE FROM samples WHERE job_id = ?`, jobID)
	return err
}

// samplesFor returns the single runs of a job: its rows while it runs, its
// blob once it is packed.
func (s *store) samplesFor(jobID int64) ([]sample, error) {
	out, err := s.sampleRows(jobID)
	if err != nil || len(out) > 0 {
		return out, err
	}
	return s.sampleBlob(jobID)
}

func (s *store) sampleRows(jobID int64) ([]sample, error) {
	rows, err := s.db.Query(`SELECT job_id, side, engine, object, op, seed, pass, iters, ns, bytes, allocs, steal, cycles, instrs, extra FROM samples WHERE job_id = ? ORDER BY object, op, engine, pass, seed, side`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []sample
	for rows.Next() {
		var sm sample
		var extra string
		if err := rows.Scan(&sm.JobID, &sm.Side, &sm.Engine, &sm.Object, &sm.Op, &sm.Seed, &sm.Pass, &sm.Iters, &sm.Ns, &sm.Bytes, &sm.Allocs, &sm.Steal, &sm.Cycles, &sm.Instrs, &extra); err != nil {
			return nil, err
		}
		if extra != "" {
			_ = json.Unmarshal([]byte(extra), &sm.Extra)
		}
		out = append(out, sm)
	}
	return out, rows.Err()
}

// The results table holds, per metric, the values the pages across jobs
// read. The remaining statistics of a metric are computed from the job's
// samples when its page is opened (fullResults).
var (
	metricPrefixes = []string{"ns_", "b_", "a_", "c_", "i_"}
	metricColumns  = []string{"base", "head", "cv_base", "cv_head", "delta", "dmin", "dmax", "pmed", "pspread", "pagree", "pn"}
	resultColumns  = func() string {
		cols := []string{"job_id", "engine", "object", "op", "baseline", "n", "iters", "steal"}
		for _, p := range metricPrefixes {
			for _, c := range metricColumns {
				cols = append(cols, p+c)
			}
		}
		return strings.Join(cols, ", ")
	}()
)

func resultsSchema(table string) string {
	var b strings.Builder
	b.WriteString(`CREATE TABLE IF NOT EXISTS ` + table + ` (
			job_id INTEGER NOT NULL,
			engine TEXT NOT NULL,
			object TEXT NOT NULL,
			op TEXT NOT NULL,
			baseline INTEGER NOT NULL,
			n INTEGER NOT NULL,
			iters INTEGER NOT NULL,
			steal INTEGER NOT NULL,`)
	for _, p := range metricPrefixes {
		for _, c := range metricColumns {
			b.WriteString("\n\t\t\t" + p + c + " REAL NOT NULL DEFAULT 0,")
		}
	}
	b.WriteString("\n\t\t\tPRIMARY KEY (job_id, engine, object, op)\n\t\t)")
	return b.String()
}

func metricArgs(m *metric) []any {
	return []any{&m.Base, &m.Head, &m.CVBase, &m.CVHead, &m.Delta, &m.DMin, &m.DMax, &m.PMed, &m.PSpread, &m.PAgree, &m.PN}
}

func metricValues(m metric) []any {
	return []any{m.Base, m.Head, m.CVBase, m.CVHead, m.Delta, m.DMin, m.DMax, m.PMed, m.PSpread, m.PAgree, m.PN}
}

func (s *store) replaceResults(jobID int64, results []result) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM results WHERE job_id = ?`, jobID); err != nil {
		tx.Rollback()
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO results(` + resultColumns + `) VALUES(` + strings.TrimSuffix(strings.Repeat("?,", 8+len(metricPrefixes)*len(metricColumns)), ",") + `)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	for _, r := range results {
		args := []any{jobID, r.Engine, r.Object, r.Op, r.Baseline, r.N, r.Iters, r.Steal}
		args = append(args, metricValues(r.Ns)...)
		args = append(args, metricValues(r.Bytes)...)
		args = append(args, metricValues(r.Allocs)...)
		args = append(args, metricValues(r.Cycles)...)
		args = append(args, metricValues(r.Instrs)...)
		if _, err := stmt.Exec(args...); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func scanResults(rows *sql.Rows, err error) ([]result, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []result
	for rows.Next() {
		var r result
		args := []any{&r.JobID, &r.Engine, &r.Object, &r.Op, &r.Baseline, &r.N, &r.Iters, &r.Steal}
		args = append(args, metricArgs(&r.Ns)...)
		args = append(args, metricArgs(&r.Bytes)...)
		args = append(args, metricArgs(&r.Allocs)...)
		args = append(args, metricArgs(&r.Cycles)...)
		args = append(args, metricArgs(&r.Instrs)...)
		if err := rows.Scan(args...); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// resultsFor returns the stored results of a job: the values kept per
// metric, without the statistics that fullResults computes.
func (s *store) resultsFor(jobID int64) ([]result, error) {
	return scanResults(s.db.Query(`SELECT `+resultColumns+` FROM results WHERE job_id = ? ORDER BY object, op, engine`, jobID))
}

func (s *store) benchIters() (map[string]int, error) {
	rows, err := s.db.Query(`SELECT engine, object, op, iters FROM bench_iters`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var l leaf
		var n int
		if err := rows.Scan(&l.Engine, &l.Object, &l.Op, &n); err != nil {
			return nil, err
		}
		out[l.key()] = n
	}
	return out, rows.Err()
}

func (s *store) setBenchIters(l leaf, n int) error {
	_, err := s.db.Exec(`INSERT INTO bench_iters(engine, object, op, iters) VALUES(?, ?, ?, ?) ON CONFLICT(engine, object, op) DO UPDATE SET iters = excluded.iters`, l.Engine, l.Object, l.Op, n)
	return err
}

func (s *store) harnessLeaves(hash string) ([]leaf, error) {
	rows, err := s.db.Query(`SELECT pkg, engine, object, op, baseline FROM harness_leaves WHERE harness = ?`, hash)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []leaf
	for rows.Next() {
		var l leaf
		if err := rows.Scan(&l.Pkg, &l.Engine, &l.Object, &l.Op, &l.Baseline); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *store) setHarnessLeaves(hash string, leaves []leaf) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM harness_leaves WHERE harness = ?`, hash); err != nil {
		tx.Rollback()
		return err
	}
	for _, l := range leaves {
		if _, err := tx.Exec(`INSERT INTO harness_leaves(harness, pkg, engine, object, op, baseline) VALUES(?, ?, ?, ?, ?, ?)`, hash, l.Pkg, l.Engine, l.Object, l.Op, l.Baseline); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *store) setKV(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO kv(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (s *store) getKV(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

// appendJobLog appends one line to a job's log file.
func appendJobLog(dataDir string, id int64, line string) {
	dir := filepath.Join(dataDir, "jobs", fmt.Sprint(id))
	_ = os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "job.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), line)
	_ = f.Close()
}

func jobReportPath(dataDir string, id int64) string {
	return filepath.Join(dataDir, "jobs", fmt.Sprint(id), "report.txt")
}
