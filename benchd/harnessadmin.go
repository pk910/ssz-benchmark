package main

import (
	"fmt"
	"math"
	"net"
	"net/http"
	"sort"
	"strings"
)

// A change of the harness gives its jobs a new harness version, and runs
// of different versions are not combined. When a change did not touch what
// is measured, the earlier version's runs can be taken over:
//
//	curl localhost/admin/harness?subject=dynamic-ssz              the versions, and how
//	                                                              commits measured under
//	                                                              two of them compare
//	curl -X POST localhost/admin/harness -d subject=dynamic-ssz -d from=<old> -d to=<new>
//
// The comparison is the evidence for taking over: per engine, how the
// pooled values of the same commit differ between the two versions.

// harnessVersions lists the harness versions of a library's finished jobs,
// newest first, with their job counts.
func (s *store) harnessVersions(subject string) ([]string, map[string]int, error) {
	rows, err := s.db.Query(`SELECT harness, count(*) FROM jobs WHERE state = ? AND subject = ? AND harness != '' GROUP BY harness ORDER BY max(id) DESC`, stateDone, subject)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	var order []string
	counts := map[string]int{}
	for rows.Next() {
		var h string
		var n int
		if err := rows.Scan(&h, &n); err != nil {
			return nil, nil, err
		}
		order = append(order, h)
		counts[h] = n
	}
	return order, counts, rows.Err()
}

// compareHarness compares the pooled cycles (time without counters) of
// the commits measured under both versions: per engine the median and the
// largest difference in percent over their operations.
func (s *store) compareHarness(subject, from, to string) (string, error) {
	rows, err := s.db.Query(`SELECT a.engine, a.cycles, b.cycles, a.ns, b.ns FROM commit_values a JOIN commit_values b
		ON a.subject = b.subject AND a.sha = b.sha AND a.engine = b.engine AND a.object = b.object AND a.op = b.op
		WHERE a.subject = ? AND a.harness = ? AND b.harness = ?`, subject, from, to)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	per := map[string][]float64{}
	for rows.Next() {
		var engine string
		var ca, cb, na, nb float64
		if err := rows.Scan(&engine, &ca, &cb, &na, &nb); err != nil {
			return "", err
		}
		a, b := ca, cb
		if a <= 0 || b <= 0 || strings.HasSuffix(engine, "Async") {
			a, b = na, nb
		}
		if a > 0 && b > 0 {
			per[engine] = append(per[engine], (b/a-1)*100)
		}
	}
	if len(per) == 0 {
		return "no commit is measured under both versions\n", nil
	}
	engines := make([]string, 0, len(per))
	for e := range per {
		engines = append(engines, e)
	}
	sort.Strings(engines)
	var b strings.Builder
	for _, e := range engines {
		ds := per[e]
		worst := 0.0
		for _, d := range ds {
			if math.Abs(d) > math.Abs(worst) {
				worst = d
			}
		}
		fmt.Fprintf(&b, "%-18s %3d operations  median %+.2f%%  largest %+.2f%%\n", e, len(ds), median(ds), worst)
	}
	return b.String(), nil
}

// renameHarness takes the jobs of one harness version over into another
// and recomputes the pooled values of their commits.
func (s *store) renameHarness(subject, from, to string) (int64, error) {
	rows, err := s.db.Query(`SELECT DISTINCT head_sha, base_sha, runner FROM jobs WHERE state = ? AND subject = ? AND harness IN (?, ?)`, stateDone, subject, from, to)
	if err != nil {
		return 0, err
	}
	type commit struct{ sha, runner string }
	seen := map[commit]bool{}
	for rows.Next() {
		var head, base, runner string
		if err := rows.Scan(&head, &base, &runner); err != nil {
			rows.Close()
			return 0, err
		}
		seen[commit{head, runner}] = true
		if base != "" {
			seen[commit{base, runner}] = true
		}
	}
	rows.Close()
	res, err := s.db.Exec(`UPDATE jobs SET harness = ? WHERE subject = ? AND harness = ?`, to, subject, from)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if _, err := s.db.Exec(`DELETE FROM commit_values WHERE subject = ? AND harness = ?`, subject, from); err != nil {
		return n, err
	}
	for c := range seen {
		if err := s.updateCommitValue(subject, c.sha, to, c.runner); err != nil {
			return n, err
		}
	}
	return n, nil
}

func (w *webServer) adminHarness(rw http.ResponseWriter, req *http.Request) {
	host, _, _ := net.SplitHostPort(req.RemoteAddr)
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		http.Error(rw, "local only", http.StatusForbidden)
		return
	}
	if err := req.ParseForm(); err != nil {
		http.Error(rw, err.Error(), http.StatusBadRequest)
		return
	}
	subject := req.Form.Get("subject")
	if subjectByName(subject) == nil {
		http.Error(rw, "unknown subject", http.StatusBadRequest)
		return
	}
	if req.Method == http.MethodPost {
		from, to := req.Form.Get("from"), req.Form.Get("to")
		if from == "" || to == "" || from == to {
			http.Error(rw, "from and to must name two harness versions", http.StatusBadRequest)
			return
		}
		n, err := w.db.renameHarness(subject, from, to)
		if err != nil {
			http.Error(rw, err.Error(), http.StatusInternalServerError)
			return
		}
		fmt.Fprintf(rw, "%d jobs of %s taken over from harness %s into %s\n", n, subject, from, to)
		return
	}
	order, counts, err := w.db.harnessVersions(subject)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	for i, h := range order {
		fmt.Fprintf(rw, "%s  %d finished jobs\n", h, counts[h])
		if i+1 < len(order) {
			cmp, _ := w.db.compareHarness(subject, order[i+1], h)
			fmt.Fprintf(rw, "  against %s:\n", order[i+1])
			for _, line := range strings.Split(strings.TrimRight(cmp, "\n"), "\n") {
				fmt.Fprintf(rw, "    %s\n", line)
			}
		}
	}
}
