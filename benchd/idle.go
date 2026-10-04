package main

import (
	"sort"
	"time"
)

// Idle time refines what was measured before. Every commit worth another
// run is a candidate with a weight; the scheduler takes the candidate
// with the fewest runs in the refinement window per unit of weight, among
// equals the one whose library ran longest ago. A new commit has no runs
// and comes first by itself; one that was run a lot waits for the others.
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

// idleJob picks the refinement run for an empty queue: the candidate with
// the fewest runs in the window per unit of weight; among equals the one
// whose library ran longest ago, and of that library the commit that ran
// longest ago.
func (s *scheduler) idleJob() (*job, error) {
	cands, err := s.candidates()
	if err != nil {
		return nil, err
	}
	since := time.Now().Add(-refineWindow)
	type scored struct {
		candidate
		score      float64
		last, self time.Time // last run of the library, and of the commit
	}
	var pool []scored
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
		last, ok := lastOf[c.sub.Name]
		if !ok {
			if last, err = s.db.lastRun(c.sub.Name); err != nil {
				return nil, err
			}
			lastOf[c.sub.Name] = last
		}
		pool = append(pool, scored{c, float64(recent) / c.weight, last, self})
	}
	if len(pool) == 0 {
		return nil, nil
	}
	sort.SliceStable(pool, func(a, b int) bool {
		if pool[a].score != pool[b].score {
			return pool[a].score < pool[b].score
		}
		if !pool[a].last.Equal(pool[b].last) {
			return pool[a].last.Before(pool[b].last)
		}
		return pool[a].self.Before(pool[b].self)
	})
	j := pool[0].job
	j.Note = "refinement run"
	j.Refinement = true
	return j, nil
}
