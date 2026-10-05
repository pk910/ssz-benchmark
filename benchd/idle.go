package main

import (
	"database/sql"
	"sort"
	"time"
)

// Idle time refines what was measured before. Every commit worth another
// run is a candidate with a weight; the scheduler takes the candidate
// with the fewest runs in the refinement window per unit of weight, among
// equals the one whose library ran longest ago, and never the commit that
// ran just before. A commit with few runs gets its turns first, one that
// was run a lot waits for the others. The runs are planned and queued for
// some hours ahead (idlePlan).
//
// Weights: the head of an open pull request 8, and half for each of its
// earlier heads below it (4, 2, 1, then none); a library's latest release
// 6 and the head of its main branch 4, with the commits below that head
// halving in the same way; each multiplied by the library's own weight.
const (
	weightPRHead  = 8.0
	weightRelease = 6.0
	weightMaster  = 4.0

	// refineWindow is the period whose runs count: with the runs of all
	// time, a commit measured for weeks would never get a turn again next
	// to a new one.
	refineWindow = 48 * time.Hour
)

// candidate is a commit that idle time may run again.
type candidate struct {
	sub    *subject
	weight float64
	job    *job // the job to queue for it
}

// openPRs lists the open pull requests of the mirrored library.
func (c *prCache) openPRs() []pullRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]pullRequest, 0, len(c.byHead))
	for _, pr := range c.byHead {
		out = append(out, pr)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Number < out[b].Number })
	return out
}

// headRuns counts the finished jobs of a library that measured a commit
// as their head since the given time, and tells whether one finished with
// the given harness version at all (a commit without one has its first
// job still to come, or does not build).
func (s *store) headRuns(subject, sha, harness string, since time.Time) (recent int, measured bool, last time.Time, err error) {
	var done int
	var at int64
	err = s.db.QueryRow(`SELECT coalesce(sum(finished >= ?), 0), coalesce(sum(harness = ?), 0), coalesce(max(finished), 0) FROM jobs WHERE state = ? AND subject = ? AND head_sha = ?`,
		since.Unix(), harness, stateDone, subject, sha).Scan(&recent, &done, &at)
	return recent, done > 0, time.Unix(at, 0), err
}

// lastRun is when a job of the library finished last.
func (s *store) lastRun(subject string) (time.Time, error) {
	var at int64
	err := s.db.QueryRow(`SELECT coalesce(max(finished), 0) FROM jobs WHERE subject = ? AND state = ?`, subject, stateDone).Scan(&at)
	return time.Unix(at, 0), err
}

// prHeads returns the measured heads of a pull request, the current one
// first: per head its newest finished job with a base.
func (s *store) prHeads(pr int) ([]*job, error) {
	// A head's place is that of its first job: a rerun of an earlier head
	// does not make it the current one.
	return s.scanJobs(s.db.Query(`SELECT `+jobColumns+` FROM jobs JOIN (
		SELECT max(id) AS last, min(id) AS first FROM jobs WHERE state = ? AND kind = ? AND pr = ? AND base_sha != '' GROUP BY head_sha) g ON id = g.last ORDER BY g.first DESC`, stateDone, kindCommit, pr))
}

// candidates lists every commit idle time may run again, with its weight.
func (s *scheduler) candidates() ([]candidate, error) {
	var out []candidate
	own := subjectByName(subjectDynSSZ)
	// Open pull requests: the head, and the heads it replaced.
	for _, pr := range s.prs.openPRs() {
		heads, err := s.db.prHeads(pr.Number)
		if err != nil {
			return nil, err
		}
		weight := weightPRHead
		for _, prev := range heads {
			if weight < 1 {
				break
			}
			if prev.Subject == own.Name {
				out = append(out, candidate{sub: own, weight: weight * own.weight(), job: &job{Kind: kindCommit, Subject: own.Name, Branch: prev.Branch, HeadSHA: prev.HeadSHA, HeadDesc: prev.HeadDesc,
					BaseSHA: prev.BaseSHA, BaseRef: prev.BaseRef, BaseDesc: prev.BaseDesc, PR: prev.PR}})
			}
			weight /= 2
		}
	}
	// The targets of every library; a commit that is several targets
	// counts with the highest weight.
	targets, err := s.db.targets()
	if err != nil {
		return nil, err
	}
	type key struct{ subject, sha string }
	byCommit := map[key]*candidate{}
	var order []key
	add := func(sub *subject, sha string, weight float64, names, labels []string) {
		k := key{sub.Name, sha}
		c := byCommit[k]
		if c == nil {
			c = &candidate{sub: sub}
			byCommit[k] = c
			order = append(order, k)
		}
		if weight > c.weight {
			c.weight = weight
		}
		var allNames, allLabels []string
		if c.job != nil {
			allNames, allLabels = splitList(c.job.Targets, "+"), splitList(c.job.Branch, ", ")
		}
		c.job = targetJob(sub, sha, append(allNames, names...), append(allLabels, labels...), s.describe(sub, sha), "")
		if len(names) == 0 && c.job.Targets == "" {
			// A commit below the head of the main branch.
			c.job.Kind = kindCommit
		}
	}
	for _, t := range targets {
		sub := subjectByName(t.Subject)
		if sub == nil {
			continue
		}
		weight := weightRelease
		if t.Name == targetMaster {
			weight = weightMaster
		}
		add(sub, t.SHA, weight*sub.weight(), []string{t.Name}, []string{t.Label})
		if t.Name == targetMaster && sub.mirrored() {
			// The commits below the head of the main branch, halving.
			sha := t.SHA
			for w := weight / 2; w >= 1; w /= 2 {
				parent, err := s.git.revParse(sha + "^")
				if err != nil {
					break
				}
				sha = parent
				add(sub, sha, w*sub.weight(), nil, []string{t.Label})
			}
		}
	}
	for _, k := range order {
		out = append(out, *byCommit[k])
	}
	return out, nil
}

func splitList(s, sep string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, part := range splitTrim(s, sep) {
		out = append(out, part)
	}
	return out
}

// idleHorizon is how far ahead idle time is planned: the refinement runs
// of that period are queued together, so that what the machine will do is
// visible and no commit comes twice in a row.
const idleHorizon = 6 * time.Hour

// idleGroup is what two consecutive refinement runs should not share: the
// pull request, or else the library.
func idleGroup(j *job) string {
	if j.PR != 0 {
		return "pr/" + strconvI(int64(j.PR))
	}
	return j.Subject
}

// lastJob returns the job that ran last (or runs now).
func (s *store) lastJob() (*job, error) {
	j, err := scanJob(s.db.QueryRow(`SELECT `+jobColumns+` FROM jobs WHERE state IN (?, ?) ORDER BY coalesce(started, 0) DESC, id DESC LIMIT 1`, stateDone, stateRunning))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return j, err
}

// runSeconds is how long the last finished job of a commit took.
func (s *store) runSeconds(subject, sha string) float64 {
	var secs float64
	_ = s.db.QueryRow(`SELECT coalesce((SELECT seconds FROM jobs WHERE subject = ? AND head_sha = ? AND state = ? ORDER BY id DESC LIMIT 1), 0)`, subject, sha, stateDone).Scan(&secs)
	return secs
}

// idleJob picks the next refinement run.
func (s *scheduler) idleJob() (*job, error) {
	plan, err := s.idlePlan(0)
	if err != nil || len(plan) == 0 {
		return nil, err
	}
	return plan[0], nil
}

// idlePlan plans the refinement runs of the coming period (at least one
// run). Each is the candidate with the fewest runs per unit of weight,
// counting the runs in the window and those planned before it; among
// equals the one whose library ran longest ago, and of that library the
// commit that ran longest ago. No run follows one of the same commit, and
// none one of the same pull request or library while another candidate
// is there: a commit that is far behind catches up in turns with the
// others, not in a row.
func (s *scheduler) idlePlan(horizon time.Duration) ([]*job, error) {
	cands, err := s.candidates()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	since := now.Add(-refineWindow)
	type scored struct {
		candidate
		runs       float64
		last, self time.Time // last run of the library, and of the commit
		took       time.Duration
	}
	var pool []*scored
	lastOf := map[string]time.Time{}
	for _, c := range cands {
		hash, err := subjectHash(s.cfg.harnessDir, c.sub)
		if err != nil {
			return nil, err
		}
		recent, measured, self, err := s.db.headRuns(c.sub.Name, c.job.HeadSHA, hash, since)
		if err != nil {
			return nil, err
		}
		if !measured || c.weight <= 0 {
			// Its first job is still to come, or it does not build.
			continue
		}
		if _, ok := lastOf[c.sub.Name]; !ok {
			last, err := s.db.lastRun(c.sub.Name)
			if err != nil {
				return nil, err
			}
			lastOf[c.sub.Name] = last
		}
		took := time.Duration(s.db.runSeconds(c.sub.Name, c.job.HeadSHA) * float64(time.Second))
		if took <= 0 {
			took = 20 * time.Minute
		}
		pool = append(pool, &scored{candidate: c, runs: float64(recent), self: self, took: took})
	}
	if len(pool) == 0 {
		return nil, nil
	}
	prevSHA, prevGroup := "", ""
	if last, err := s.db.lastJob(); err == nil && last != nil {
		prevSHA, prevGroup = last.HeadSHA, idleGroup(last)
	}
	less := func(a, b *scored) bool {
		if sa, sb := a.runs/a.weight, b.runs/b.weight; sa != sb {
			return sa < sb
		}
		if la, lb := lastOf[a.sub.Name], lastOf[b.sub.Name]; !la.Equal(lb) {
			return la.Before(lb)
		}
		return a.self.Before(b.self)
	}
	var plan []*job
	var planned time.Duration
	for len(plan) == 0 || (planned < horizon && len(plan) < 100) {
		// The best of another pull request or library; else the best of
		// another commit; the commit that ran last only when the machine
		// would stand still otherwise.
		var best *scored
		for _, allow := range []func(*scored) bool{
			func(c *scored) bool { return idleGroup(c.job) != prevGroup },
			func(c *scored) bool { return c.job.HeadSHA != prevSHA },
			func(c *scored) bool { return len(plan) == 0 },
		} {
			for _, c := range pool {
				if allow(c) && (best == nil || less(c, best)) {
					best = c
				}
			}
			if best != nil {
				break
			}
		}
		if best == nil {
			break
		}
		j := *best.job
		j.Note = "refinement run"
		j.Refinement = true
		plan = append(plan, &j)
		planned += best.took
		at := now.Add(planned)
		best.runs++
		best.self, lastOf[best.sub.Name] = at, at
		prevSHA, prevGroup = j.HeadSHA, idleGroup(&j)
	}
	return plan, nil
}

// stillCandidate reports whether a planned refinement run is still worth
// running: its commit may have left the candidates since (a merged pull
// request, a head that moved on).
func (s *scheduler) stillCandidate(j *job) bool {
	if s == nil {
		return true
	}
	cands, err := s.candidates()
	if err != nil {
		return true
	}
	for _, c := range cands {
		if c.sub.Name == j.Subject && c.job.HeadSHA == j.HeadSHA {
			return true
		}
	}
	return false
}
