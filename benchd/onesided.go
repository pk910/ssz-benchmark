package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"time"
)

// One-sided jobs. A commit job measures its head; for every run the base
// value comes from an earlier run of the base commit with the same layout
// seed, as long as the two agree within the base threshold. Where they do
// not, or where no earlier run exists, the base is measured in the job, so
// that a difference a job reports always rests on runs made minutes apart.

// baseMaxAge is how old an earlier run of the base commit may be.
const baseMaxAge = 3 * 24 * time.Hour

// layoutOff is how far (percent) the cycles of an earlier run may lie
// above the other seeds' runs of the same operation, at equal
// instructions, before it counts as a pathological layout and is not used.
const layoutOff = 20.0

// bootID identifies the current boot of the machine.
func bootID() string {
	data, _ := os.ReadFile("/proc/sys/kernel/random/boot_id")
	return strings.TrimSpace(string(data))
}

// fromJob is the job an earlier base run was taken from (0: measured in
// the sample's own job).
func (sm sample) fromJob() int64 { return int64(sm.Extra["from"]) }

// storedBaseRuns returns the earlier runs of a job's base commit that can
// stand in for measuring it: same library, harness version, Go version,
// machine and boot, not older than baseMaxAge, one per operation and seed
// (the newest), keyed "engine/object/op|seed".
func (s *store) storedBaseRuns(j *job, seeds []string) (map[string]sample, error) {
	out := map[string]sample{}
	if j.BaseSHA == "" {
		return out, nil
	}
	rows, err := s.db.Query(`SELECT id, head_sha, base_sha FROM jobs WHERE state = ? AND id != ? AND subject = ? AND harness = ? AND runner = ? AND go_version = ? AND boot_id = ? AND boot_id != '' AND finished >= ?
		AND (head_sha = ? OR base_sha = ?) ORDER BY id DESC LIMIT 20`,
		stateDone, j.ID, j.Subject, j.Harness, j.Runner, j.GoVersion, bootID(), time.Now().Add(-baseMaxAge).Unix(), j.BaseSHA, j.BaseSHA)
	if err != nil {
		return nil, err
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
			return nil, err
		}
		jobs = append(jobs, x)
	}
	rows.Close()
	// A base that is not measured more often than the head is measured by
	// the job itself, so that the base never falls behind the head.
	var baseRuns, headRuns int
	if err := s.db.QueryRow(`SELECT coalesce(sum((head_sha = ? OR (base_sha = ? AND note NOT LIKE '%one-sided:%'))), 0), coalesce(sum(head_sha = ?), 0) FROM jobs
		WHERE state = ? AND id != ? AND subject = ? AND harness = ?`, j.BaseSHA, j.BaseSHA, j.HeadSHA, stateDone, j.ID, j.Subject, j.Harness).Scan(&baseRuns, &headRuns); err != nil {
		return nil, err
	}
	if baseRuns <= headRuns {
		return out, nil
	}
	want := map[string]bool{}
	for _, seed := range seeds {
		want[seed] = true
	}
	for _, x := range jobs {
		samples, err := s.samplesFor(x.id)
		if err != nil {
			return nil, err
		}
		for _, sm := range samples {
			if sm.isDiag() || sm.fromJob() != 0 || !want[sm.Seed] || (sm.Side == "head" && x.head != j.BaseSHA) || (sm.Side == "base" && x.base != j.BaseSHA) {
				continue
			}
			key := sm.Engine + "/" + sm.Object + "/" + sm.Op + "|" + sm.Seed
			if _, ok := out[key]; !ok {
				out[key] = sm
			}
		}
	}
	// A run on a pathological layout must not become a base: its cycles
	// lie far above the other seeds' at the same instructions.
	byLeaf := map[string][]sample{}
	for key, sm := range out {
		leafKey, _, _ := strings.Cut(key, "|")
		byLeaf[leafKey] = append(byLeaf[leafKey], sm)
	}
	for key, sm := range out {
		leafKey, _, _ := strings.Cut(key, "|")
		var others []float64
		for _, o := range byLeaf[leafKey] {
			if o.Seed != sm.Seed && o.Cycles > 0 && sm.Instrs > 0 && math.Abs(o.Instrs-sm.Instrs)/sm.Instrs*100 <= instrsAgreePct {
				others = append(others, o.Cycles)
			}
		}
		if len(others) >= 2 && sm.Cycles > median(others)*(1+layoutOff/100) {
			delete(out, key)
		}
	}
	return out, nil
}

// baseFor returns the base runs of a group for one pass of a one-sided
// job, aligned with the group's leaves: the earlier run where the head's
// run agrees with it within the base threshold, a run measured now
// otherwise. When most of the group needs measuring it is measured as a
// group, like the head; single operations are measured on their own. Runs
// measured now are checked against the head as in a two-sided job.
func (r *runner) baseFor(ctx context.Context, j *job, base, head *side, seed, pkg, engine, object string, leaves []leaf, hs []sample, stored map[string]sample, iters map[string]int, pass int, seen map[string][]float64, jobDir string, logw func(string, ...any)) ([]sample, error) {
	bs := make([]sample, len(leaves))
	var need []int
	for i, l := range leaves {
		st, ok := stored[l.key()+"|"+seed]
		kh, kb := sampleKey(hs[i]), sampleKey(st)
		if !ok || kh <= 0 || kb <= 0 || (st.Cycles > 0) != (hs[i].Cycles > 0) || math.Abs(kh-kb)/kb*100 > r.cfg.baseThreshold {
			need = append(need, i)
			continue
		}
		b := st
		b.Extra = map[string]float64{"from": float64(st.JobID)}
		for k, v := range st.Extra {
			b.Extra[k] = v
		}
		b.JobID, b.Side, b.Pass = j.ID, "base", pass
		bs[i] = b
	}
	if len(need) == 0 {
		return bs, nil
	}
	if len(need)*2 > len(leaves) {
		fresh, err := r.measureGroup(ctx, j, base, seed, pkg, engine, object, leaves, iters, pass, seen, jobDir, logw)
		if err != nil {
			return nil, err
		}
		if err := r.reconcile(ctx, j, base, head, seed, leaves, iters, pass, fresh, hs, seen, jobDir, logw); err != nil {
			return nil, err
		}
		return fresh, nil
	}
	sub := make([]leaf, len(need))
	subB, subH := make([]sample, len(need)), make([]sample, len(need))
	for n, i := range need {
		sm, err := r.measureLeaf(ctx, j, base, seed, leaves[i], iters, pass, jobDir)
		if err != nil {
			return nil, err
		}
		sub[n], subB[n], subH[n] = leaves[i], sm, hs[i]
		if st, ok := stored[leaves[i].key()+"|"+seed]; ok {
			logw("%s seed %s: head is %.1f%% off the earlier base run of job %d, base measured here", leaves[i].key(), seed, (sampleKey(hs[i])/sampleKey(st)-1)*100, st.JobID)
		}
	}
	if err := r.reconcile(ctx, j, base, head, seed, sub, iters, pass, subB, subH, seen, jobDir, logw); err != nil {
		return nil, err
	}
	for n, i := range need {
		bs[i], hs[i] = subB[n], subH[n]
	}
	return bs, nil
}

// baseNote describes where the base runs of a job came from.
func baseNote(samples []sample) string {
	from := map[int64]bool{}
	borrowed, measured := 0, 0
	for _, sm := range samples {
		if sm.Side != "base" || sm.isDiag() {
			continue
		}
		if id := sm.fromJob(); id != 0 {
			borrowed++
			from[id] = true
		} else {
			measured++
		}
	}
	if borrowed == 0 {
		return ""
	}
	var ids []string
	for id := range from {
		ids = append(ids, fmt.Sprintf("#%d", id))
	}
	return fmt.Sprintf("one-sided: %d of %d base runs taken from earlier jobs (%s), %d measured here", borrowed, borrowed+measured, strings.Join(ids, ", "), measured)
}
