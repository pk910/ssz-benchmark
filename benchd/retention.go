package main

import (
	"compress/gzip"
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// compressJobLogs gzips every plain file of a job's log directory and
// removes the original. A file whose compressed twin exists already (a
// line logged after the job was compressed) is appended to it as another
// gzip member.
func compressJobLogs(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.Type().IsRegular() || strings.HasSuffix(e.Name(), ".gz") {
			continue
		}
		if err := compressFile(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

func compressFile(path string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(path+".gz", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(out)
	if _, err = io.Copy(zw, in); err == nil {
		err = zw.Close()
	}
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// compressAllJobLogs compresses the log directories of all jobs.
func compressAllJobLogs(dataDir string) {
	dirs, _ := os.ReadDir(filepath.Join(dataDir, "jobs"))
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		if err := compressJobLogs(filepath.Join(dataDir, "jobs", d.Name())); err != nil {
			log.Printf("compress logs of job %s: %v", d.Name(), err)
		}
	}
}

// jobFiles lists the log files of a job by their plain names.
func jobFiles(dir string) []string {
	entries, _ := os.ReadDir(dir)
	files := []string{}
	seen := map[string]bool{}
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".gz")
		if !seen[name] {
			seen[name] = true
			files = append(files, name)
		}
	}
	return files
}

// serveJobFile serves a log file of a job, whether it is stored plain,
// compressed, or both (the compressed part is the older one).
func serveJobFile(rw http.ResponseWriter, req *http.Request, path string) {
	zf, zerr := os.Open(path + ".gz")
	if zerr != nil {
		http.ServeFile(rw, req, path)
		return
	}
	defer zf.Close()
	zr, err := gzip.NewReader(zf)
	if err != nil {
		http.Error(rw, err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = io.Copy(rw, zr)
	if f, err := os.Open(path); err == nil {
		_, _ = io.Copy(rw, f)
		f.Close()
	}
}

// pruneSamples deletes the single-run samples of jobs that finished before
// the given time. The jobs and their results stay.
func (s *store) pruneSamples(before time.Time) (int64, error) {
	res, err := s.db.Exec(`DELETE FROM samples WHERE job_id IN (SELECT id FROM jobs WHERE finished IS NOT NULL AND finished < ? AND state IN (?, ?))`,
		before.Unix(), stateDone, stateFailed)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// retentionLoop prunes the samples older than the retention once a day.
func retentionLoop(ctx context.Context, db *store, retention time.Duration) {
	if retention <= 0 {
		return
	}
	for {
		if n, err := db.pruneSamples(time.Now().Add(-retention)); err != nil {
			log.Printf("prune samples: %v", err)
		} else if n > 0 {
			log.Printf("pruned %d samples of jobs finished more than %s ago", n, retention)
		}
		sleepCtx(ctx, 24*time.Hour)
		if ctx.Err() != nil {
			return
		}
	}
}
