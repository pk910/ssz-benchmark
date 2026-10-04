package main

import (
	"debug/elf"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// buildFact describes what one side of a job was built into, per harness
// package: the size of the benchmark binary and of its code, the size of
// the generated SSZ code, and how long generating and compiling took (0
// for a build taken from the cache of an earlier process).
type buildFact struct {
	JobID        int64
	Side         string
	SHA          string
	Pkg          string
	BinBytes     int64
	TextBytes    int64
	GenBytes     int64
	BuildSeconds float64
}

// buildNote is kept in a harness build directory so that later jobs using
// the cached build still know how long it took.
type buildNote struct {
	Seconds float64
}

func writeBuildNote(hdir string, seconds float64) {
	data, _ := json.Marshal(buildNote{Seconds: seconds})
	_ = os.WriteFile(filepath.Join(hdir, "build.json"), data, 0o644)
}

// buildFacts measures the build of a side from its harness directory.
func buildFacts(jobID int64, s *side) []buildFact {
	var note buildNote
	if data, err := os.ReadFile(filepath.Join(s.hdir, "build.json")); err == nil {
		_ = json.Unmarshal(data, &note)
	}
	var facts []buildFact
	for pkg, seeds := range s.bins {
		names := make([]string, 0, len(seeds))
		for seed := range seeds {
			names = append(names, seed)
		}
		if len(names) == 0 {
			continue
		}
		sort.Strings(names)
		f := buildFact{JobID: jobID, Side: s.name, SHA: s.sha, Pkg: pkg, BuildSeconds: note.Seconds}
		bin := seeds[names[0]]
		if info, err := os.Stat(bin); err == nil {
			f.BinBytes = info.Size()
		}
		if ef, err := elf.Open(bin); err == nil {
			if sec := ef.Section(".text"); sec != nil {
				f.TextBytes = int64(sec.Size)
			}
			ef.Close()
		}
		if info, err := os.Stat(filepath.Join(s.hdir, "types", pkg, "gen_ssz.go")); err == nil {
			f.GenBytes = info.Size()
		}
		facts = append(facts, f)
	}
	sort.Slice(facts, func(a, b int) bool { return facts[a].Pkg < facts[b].Pkg })
	return facts
}

func (s *store) insertBuilds(facts []buildFact) error {
	for _, f := range facts {
		if _, err := s.db.Exec(`INSERT OR REPLACE INTO builds(job_id, side, sha, pkg, bin_bytes, text_bytes, gen_bytes, build_seconds) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
			f.JobID, f.Side, f.SHA, f.Pkg, f.BinBytes, f.TextBytes, f.GenBytes, f.BuildSeconds); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) buildsFor(jobID int64) ([]buildFact, error) {
	rows, err := s.db.Query(`SELECT job_id, side, sha, pkg, bin_bytes, text_bytes, gen_bytes, build_seconds FROM builds WHERE job_id = ? ORDER BY pkg, side`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []buildFact{}
	for rows.Next() {
		var f buildFact
		if err := rows.Scan(&f.JobID, &f.Side, &f.SHA, &f.Pkg, &f.BinBytes, &f.TextBytes, &f.GenBytes, &f.BuildSeconds); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
