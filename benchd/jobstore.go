package main

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"
)

// jobStore is what a runner needs from the controller: a job to run, the
// fixed iteration counts and leaves of the harness, somewhere to put
// samples, progress and the outcome. The local implementation is the
// database of the controller itself; the remote one speaks HTTP to it.
type jobStore interface {
	claim(runner string) (*job, error)
	started(id int64, goVersion, harness string) error
	publish(ls liveStatus) error
	log(id int64, line string)
	harnessLeaves(hash string) ([]leaf, error)
	setHarnessLeaves(hash string, leaves []leaf) error
	benchIters() (map[string]int, error)
	setBenchIters(l leaf, n int) error
	resetSamples(id int64) error
	addSamples(samples []sample) error
	finish(id int64, passes int, errText string, seconds float64) error
	interrupted(id int64) error
}

// Runner (machine) states.
const (
	runnerQualifying = "qualifying" // only noise jobs until the check passes
	runnerOK         = "ok"
	runnerRejected   = "rejected"
)

// qualifyRule is the acceptance test for a worker machine.
type qualifyRule struct {
	maxP95     float64 // p95 of |Δ time| over the leaves of its noise job, percent
	maxRatio   float64 // |geomean(worker/controller) - 1|, fraction
	maxSpread  float64 // CV of the per-leaf ratios, percent
	maxRetries int
}

// localStore is the controller's own database behind the jobStore interface.
type localStore struct {
	db    *store
	sched *scheduler
	local string // name of the controller's own runner
	rule  qualifyRule

	checks *checkReporter // nil without a GitHub App
	passMu sync.Mutex
	passes map[int64]int // job -> passes last seen in a status report
}

// claim hands the runner its next job. The controller's own runner and
// qualified workers take the queue; a worker still qualifying gets a noise
// job of its own, a rejected one nothing. An empty queue yields an idle job.
func (s *localStore) claim(runner string) (*job, error) {
	if runner != s.local {
		r, err := s.db.getRunner(runner)
		if err != nil {
			return nil, err
		}
		if r == nil {
			r = &runnerInfo{Name: runner, State: runnerQualifying, FirstSeen: time.Now()}
			log.Printf("new worker %s: qualifying", runner)
		}
		r.LastSeen = time.Now()
		if err := s.db.putRunner(r); err != nil {
			return nil, err
		}
		switch r.State {
		case runnerRejected:
			return nil, nil
		case runnerQualifying:
			if j, err := s.db.runningOrQueuedFor(runner); err != nil || j != nil {
				return s.db.claimJob(j, runner)
			}
			j, err := s.sched.idleJobOfKind(kindNoise)
			if err != nil {
				return nil, err
			}
			j.Runner = runner
			j.Note = "qualification: noise floor of " + runner
			j.Priority = 1
			if err := s.db.insertJob(j); err != nil {
				return nil, err
			}
			return s.db.claimJob(j, runner)
		}
	}
	for {
		j, err := s.db.nextQueued(runner)
		if err != nil {
			return nil, err
		}
		if j == nil {
			// An empty queue before the scheduler's first pass means only
			// that it has not looked yet: wait for it instead of filling
			// the gap with an idle job.
			select {
			case <-s.sched.ready:
			default:
				<-s.sched.ready
				continue
			}
			idle, err := s.sched.idleJob()
			if err != nil {
				return nil, err
			}
			idle.Runner = runner
			if err := s.db.insertJob(idle); err != nil {
				return nil, err
			}
			j = idle
		}
		claimed, err := s.db.claimJob(j, runner)
		if err != nil {
			return nil, err
		}
		if claimed != nil {
			claimed.Seeds = s.seedsFor(claimed)
			return claimed, nil
		}
	}
}

func (s *localStore) started(id int64, goVersion, harness string) error {
	err := s.db.startJob(id, goVersion, harness)
	s.checks.syncJob(id)
	return err
}

// publish stores the runner's live status; a finished pass refreshes the
// job's check with the provisional results.
func (s *localStore) publish(ls liveStatus) error {
	if ls.JobID != 0 {
		s.passMu.Lock()
		if s.passes == nil {
			s.passes = map[int64]int{}
		}
		changed := s.passes[ls.JobID] != ls.Progress.Passes
		s.passes[ls.JobID] = ls.Progress.Passes
		s.passMu.Unlock()
		if changed {
			s.checks.syncJob(ls.JobID)
		}
	}
	return s.db.setRunnerStatus(ls)
}

func (s *localStore) log(id int64, line string) { appendJobLog(s.db.dataDir, id, line) }

func (s *localStore) harnessLeaves(hash string) ([]leaf, error) { return s.db.harnessLeaves(hash) }
func (s *localStore) setHarnessLeaves(hash string, l []leaf) error {
	return s.db.setHarnessLeaves(hash, l)
}
func (s *localStore) benchIters() (map[string]int, error) { return s.db.benchIters() }
func (s *localStore) setBenchIters(l leaf, n int) error   { return s.db.setBenchIters(l, n) }
func (s *localStore) resetSamples(id int64) error         { return s.db.deleteSamples(id) }
func (s *localStore) addSamples(samples []sample) error   { return s.db.insertSamples(samples) }
func (s *localStore) interrupted(id int64) error {
	err := s.db.requeueJob(id)
	s.checks.syncJob(id)
	return err
}

// seedSets is how many sets of layout seeds reruns of a pair rotate
// through before they repeat.
const seedSets = 4

// seedsFor picks the layout seeds of a run: the configured ones for the
// first run of a pair, and for every rerun the next set (each seed moved
// on by a thousand), so the pooled result of a pair covers more layouts
// with every run instead of measuring the same four again.
func (s *localStore) seedsFor(j *job) []string {
	runs, err := s.db.pairRuns(j.HeadSHA, j.BaseSHA, j.Runner)
	if err != nil || runs%seedSets == 0 {
		return nil
	}
	seeds := make([]string, 0, len(s.sched.cfg.seeds))
	for _, seed := range s.sched.cfg.seeds {
		n, err := strconv.Atoi(seed)
		if err != nil {
			return nil
		}
		seeds = append(seeds, strconv.Itoa(n+1000*(runs%seedSets)))
	}
	return seeds
}

// finish stores the outcome. A finished job's results are computed from
// its samples pooled with the earlier runs of the same pair on the same
// machine; a qualification noise job then decides the worker's state.
func (s *localStore) finish(id int64, passes int, errText string, seconds float64) error {
	defer s.checks.syncJob(id)
	j, err := s.db.getJob(id)
	if err != nil || j == nil {
		return fmt.Errorf("job %d: %v", id, err)
	}
	if errText != "" {
		return s.db.finishJob(id, stateFailed, passes, errText, seconds)
	}
	samples, err := s.db.samplesFor(id)
	if err != nil {
		return err
	}
	if len(samples) == 0 {
		return s.db.finishJob(id, stateFailed, passes, "no samples", seconds)
	}
	pooled := samples
	if ids, err := s.db.pairJobIDs(j.HeadSHA, j.BaseSHA, j.Harness, j.Runner); err == nil && len(ids) > 0 {
		for _, pid := range ids {
			prev, err := s.db.samplesFor(pid)
			if err != nil {
				return err
			}
			pooled = append(pooled, prev...)
		}
		note := fmt.Sprintf("run %d of this pair: results pooled over %d runs", len(ids)+1, len(ids)+1)
		if j.Note != "" {
			note = j.Note + "; " + note
		}
		_ = s.db.setJobNote(id, note)
	}
	results := summarize(pooled)
	if err := s.db.replaceResults(id, results); err != nil {
		return err
	}
	_ = writeReport(jobReportPath(s.db.dataDir, id), j, results)
	if err := s.db.finishJob(id, stateDone, passes, "", seconds); err != nil {
		return err
	}
	if j.Kind == kindNoise && j.Runner != s.local && j.Runner != "" {
		s.qualify(j, results)
	}
	return nil
}

// qualify decides a worker's state from its noise job: its own floor, and
// how its absolute values compare with the controller machine's latest
// noise run of the same harness.
func (s *localStore) qualify(j *job, results []result) {
	r, err := s.db.getRunner(j.Runner)
	if err != nil || r == nil || r.State == runnerOK {
		return
	}
	var deltas []float64
	for _, res := range results {
		if !res.Baseline {
			deltas = append(deltas, math.Abs(res.Ns.Delta))
		}
	}
	r.NoiseMedian = percentile(deltas, 0.5)
	r.NoiseP95 = percentile(deltas, 0.95)
	r.Checks++
	reasons := []string{}
	if r.NoiseP95 > s.rule.maxP95 {
		reasons = append(reasons, fmt.Sprintf("noise p95 %.2f%% above %.2f%%", r.NoiseP95, s.rule.maxP95))
	}
	ref, err := s.db.latestNoiseJob(s.local, j.Harness)
	if err == nil && ref != nil {
		refResults, _ := s.db.resultsFor(ref.ID)
		refByLeaf := map[string]float64{}
		for _, res := range refResults {
			refByLeaf[joinKey(res.Engine, res.Object, res.Op)] = res.Ns.Head
		}
		var logs []float64
		for _, res := range results {
			if ref, ok := refByLeaf[joinKey(res.Engine, res.Object, res.Op)]; ok && ref > 0 && res.Ns.Head > 0 {
				logs = append(logs, math.Log(res.Ns.Head/ref))
			}
		}
		if len(logs) > 0 {
			r.RatioGeomean = math.Exp(mean(logs))
			sd := math.Sqrt(variance(logs, mean(logs)))
			r.RatioSpread = sd * 100
			r.ReferenceJob = ref.ID
			if math.Abs(r.RatioGeomean-1) > s.rule.maxRatio {
				reasons = append(reasons, fmt.Sprintf("speed ratio to the controller %.3f outside ±%.0f%%", r.RatioGeomean, s.rule.maxRatio*100))
			}
			if r.RatioSpread > s.rule.maxSpread {
				reasons = append(reasons, fmt.Sprintf("per-leaf ratio spread %.1f%% above %.1f%%", r.RatioSpread, s.rule.maxSpread))
			}
		}
	} else {
		reasons = append(reasons, "no controller noise run to compare with yet")
	}
	r.QualifyJob = j.ID
	if len(reasons) == 0 {
		r.State = runnerOK
		r.Note = fmt.Sprintf("qualified by job %d: noise p95 %.2f%%, ratio %.3f (spread %.1f%%)", j.ID, r.NoiseP95, r.RatioGeomean, r.RatioSpread)
		log.Printf("worker %s qualified: %s", r.Name, r.Note)
	} else {
		sort.Strings(reasons)
		r.Note = fmt.Sprintf("check %d failed (job %d): %s", r.Checks, j.ID, joinStrings(reasons, "; "))
		if r.Checks >= s.rule.maxRetries {
			r.State = runnerRejected
			r.Note += "; rejected"
		}
		log.Printf("worker %s: %s", r.Name, r.Note)
	}
	_ = s.db.putRunner(r)
}

func joinStrings(xs []string, sep string) string {
	out := ""
	for i, x := range xs {
		if i > 0 {
			out += sep
		}
		out += x
	}
	return out
}
