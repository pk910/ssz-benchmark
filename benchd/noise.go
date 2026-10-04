package main

import (
	"sort"
	"strings"
)

// The noise floor is what two measurements of the same code differ by. It
// comes from self-comparisons: two runs of one commit with the same layout
// seeds in different jobs of the same harness version, machine and boot
// (the comparison a one-sided job makes against an earlier run of its
// base), and the jobs that measured one commit on both sides.

// noiseSet is one self-comparison: the results of a commit against itself.
type noiseSet struct {
	ID      int64 // the later job
	Runner  string
	results []result
}

// noiseSetLimit is how many of the newest self-comparisons count.
const noiseSetLimit = 40

// noiseJobs is how many of the newest finished jobs are searched for
// repeated runs.
const noiseJobs = 80

// noiseSets collects the newest self-comparisons.
func noiseSets(db *store) ([]noiseSet, error) {
	var sets []noiseSet
	// Jobs with the same commit on both sides.
	jobs, err := db.listJobs(noiseSetLimit, kindNoise)
	if err != nil {
		return nil, err
	}
	for _, j := range jobs {
		if j.State != stateDone {
			continue
		}
		results, err := db.resultsFor(j.ID)
		if err != nil {
			return nil, err
		}
		sets = append(sets, noiseSet{ID: j.ID, Runner: j.Runner, results: results})
	}
	// Repeated runs of a commit: per commit, harness version, machine, boot
	// and seed set, each run against the one before it.
	rows, err := db.db.Query(`SELECT id, subject, head_sha, harness, runner, boot_id FROM jobs WHERE state = ? AND kind != ? ORDER BY id DESC LIMIT ?`, stateDone, kindNoise, noiseJobs)
	if err != nil {
		return nil, err
	}
	type run struct {
		id      int64
		runner  string
		samples map[string]sample // operation|seed -> the first run of it
	}
	groups := map[string][]run{}
	type ref struct {
		id                                    int64
		subject, sha, harness, runner, bootID string
	}
	var refs []ref
	for rows.Next() {
		var r ref
		if err := rows.Scan(&r.id, &r.subject, &r.sha, &r.harness, &r.runner, &r.bootID); err != nil {
			rows.Close()
			return nil, err
		}
		refs = append(refs, r)
	}
	rows.Close()
	for _, r := range refs {
		samples, err := db.samplesFor(r.id)
		if err != nil {
			return nil, err
		}
		first := map[string]sample{}
		seeds := map[string]bool{}
		for _, sm := range samples {
			if sm.Side != "head" || sm.isDiag() || sm.fromJob() != 0 {
				continue
			}
			k := sm.Engine + "/" + sm.Object + "/" + sm.Op + "|" + sm.Seed
			if _, ok := first[k]; !ok {
				first[k] = sm
				seeds[sm.Seed] = true
			}
		}
		if len(first) == 0 {
			continue
		}
		names := make([]string, 0, len(seeds))
		for seed := range seeds {
			names = append(names, seed)
		}
		sort.Strings(names)
		key := strings.Join([]string{r.subject, r.sha, r.harness, r.runner, r.bootID, strings.Join(names, ",")}, "|")
		groups[key] = append(groups[key], run{id: r.id, runner: r.runner, samples: first})
	}
	for _, runs := range groups {
		// refs are newest first, so runs are too.
		for i := 0; i+1 < len(runs); i++ {
			later, earlier := runs[i], runs[i+1]
			var pair []sample
			for k, h := range later.samples {
				b, ok := earlier.samples[k]
				if !ok {
					continue
				}
				b.Side, h.Side = "base", "head"
				b.Pass, h.Pass = len(pair)/2, len(pair)/2
				pair = append(pair, b, h)
			}
			if len(pair) == 0 {
				continue
			}
			// Runs of one seed pair up in the statistics by their order.
			sort.SliceStable(pair, func(a, b int) bool {
				if pair[a].Seed != pair[b].Seed {
					return pair[a].Seed < pair[b].Seed
				}
				return pair[a].Side < pair[b].Side
			})
			sets = append(sets, noiseSet{ID: later.id, Runner: later.runner, results: summarize(pair)})
		}
	}
	sort.Slice(sets, func(a, b int) bool { return sets[a].ID > sets[b].ID })
	if len(sets) > noiseSetLimit {
		sets = sets[:noiseSetLimit]
	}
	return sets, nil
}
