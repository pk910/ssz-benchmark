package main

import (
	"math"
)

// commitValue is the pooled value of one operation of a commit: the
// medians over every run that measured the commit, on whichever side of a
// job, with one harness version.
type commitValue struct {
	Subject, SHA, Harness             string
	Engine, Object, Op                string
	JobID                             int64 // newest job that measured it
	N                                 int   // runs
	Ns, Cycles, Instrs, Bytes, Allocs float64
	CVNs, CVCycles                    float64 // spread of the runs, percent
}

// valueJobs is how many of the newest jobs of a commit are pooled.
const valueJobs = 60

// updateCommitValues recomputes the pooled values of the commits a job
// measured.
func (s *store) updateCommitValues(j *job) error {
	shas := []string{j.HeadSHA}
	if j.BaseSHA != "" && j.BaseSHA != j.HeadSHA {
		shas = append(shas, j.BaseSHA)
	}
	for _, sha := range shas {
		if err := s.updateCommitValue(j.Subject, sha, j.Harness, j.Runner); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) updateCommitValue(subject, sha, harness, runner string) error {
	rows, err := s.db.Query(`SELECT id, head_sha, base_sha FROM jobs WHERE state = ? AND subject = ? AND harness = ? AND runner = ? AND (head_sha = ? OR base_sha = ?) ORDER BY id DESC LIMIT ?`,
		stateDone, subject, harness, runner, sha, sha, valueJobs)
	if err != nil {
		return err
	}
	type src struct {
		id         int64
		head, base string
	}
	var jobs []src
	for rows.Next() {
		var x src
		if err := rows.Scan(&x.id, &x.head, &x.base); err != nil {
			rows.Close()
			return err
		}
		jobs = append(jobs, x)
	}
	rows.Close()
	type acc struct {
		ns, cycles, instrs, bytes, allocs []float64
	}
	leaves := map[leaf]*acc{}
	var newest int64
	for _, x := range jobs {
		samples, err := s.samplesFor(x.id)
		if err != nil {
			return err
		}
		newest = max(newest, x.id)
		for _, sm := range samples {
			if sm.isDiag() || sm.fromJob() != 0 || (sm.Side == "head" && x.head != sha) || (sm.Side == "base" && x.base != sha) {
				continue
			}
			k := leaf{Engine: sm.Engine, Object: sm.Object, Op: sm.Op}
			a := leaves[k]
			if a == nil {
				a = &acc{}
				leaves[k] = a
			}
			a.ns = append(a.ns, sm.Ns)
			a.bytes = append(a.bytes, sm.Bytes)
			a.allocs = append(a.allocs, sm.Allocs)
			if sm.Cycles > 0 {
				a.cycles = append(a.cycles, sm.Cycles)
				a.instrs = append(a.instrs, sm.Instrs)
			}
		}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM commit_values WHERE subject = ? AND sha = ? AND harness = ?`, subject, sha, harness); err != nil {
		tx.Rollback()
		return err
	}
	spread := func(xs []float64) float64 {
		if len(xs) < 2 {
			return 0
		}
		m := mean(xs)
		if m == 0 {
			return 0
		}
		return math.Sqrt(variance(xs, m)) / m * 100
	}
	med := func(xs []float64) float64 {
		if len(xs) == 0 {
			return 0
		}
		return median(xs)
	}
	for k, a := range leaves {
		if _, err := tx.Exec(`INSERT INTO commit_values(subject, sha, harness, engine, object, op, job_id, n, ns, cycles, instrs, bytes, allocs, cv_ns, cv_cycles)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			subject, sha, harness, k.Engine, k.Object, k.Op, newest, len(a.ns), med(a.ns), med(a.cycles), med(a.instrs), med(a.bytes), med(a.allocs), spread(a.ns), spread(a.cycles)); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// commitValues returns the pooled values of a commit with the harness
// version it was measured with last.
func (s *store) commitValues(subject, sha string) ([]commitValue, error) {
	rows, err := s.db.Query(`SELECT subject, sha, harness, engine, object, op, job_id, n, ns, cycles, instrs, bytes, allocs, cv_ns, cv_cycles FROM commit_values
		WHERE subject = ? AND sha = ? AND harness = (SELECT harness FROM commit_values WHERE subject = ? AND sha = ? ORDER BY job_id DESC LIMIT 1)`, subject, sha, subject, sha)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []commitValue
	for rows.Next() {
		var v commitValue
		if err := rows.Scan(&v.Subject, &v.SHA, &v.Harness, &v.Engine, &v.Object, &v.Op, &v.JobID, &v.N, &v.Ns, &v.Cycles, &v.Instrs, &v.Bytes, &v.Allocs, &v.CVNs, &v.CVCycles); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// backfillCommitValues computes the pooled values of every measured commit
// once.
func (s *store) backfillCommitValues() error {
	const key = "commit_values:pooled"
	if v, _ := s.getKV(key); v != "" {
		return nil
	}
	jobs, err := s.listJobs(1<<30, "")
	if err != nil {
		return err
	}
	done := map[string]bool{}
	for _, j := range jobs {
		if j.State != stateDone || j.Kind == kindBaseline {
			continue
		}
		for _, sha := range []string{j.HeadSHA, j.BaseSHA} {
			k := j.Subject + "|" + sha + "|" + j.Harness + "|" + j.Runner
			if sha == "" || done[k] {
				continue
			}
			done[k] = true
			if err := s.updateCommitValue(j.Subject, sha, j.Harness, j.Runner); err != nil {
				return err
			}
		}
	}
	return s.setKV(key, "done")
}
