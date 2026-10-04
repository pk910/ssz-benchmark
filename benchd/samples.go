package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
)

// packSamples moves the single runs of a finished job from the samples
// table into one compressed blob.
func (s *store) packSamples(jobID int64) error {
	samples, err := s.sampleRows(jobID)
	if err != nil || len(samples) == 0 {
		return err
	}
	raw, err := json.Marshal(samples)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err = zw.Write(raw); err == nil {
		err = zw.Close()
	}
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT OR REPLACE INTO job_samples(job_id, data) VALUES(?, ?)`, jobID, buf.Bytes()); err == nil {
		_, err = tx.Exec(`DELETE FROM samples WHERE job_id = ?`, jobID)
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *store) sampleBlob(jobID int64) ([]sample, error) {
	var data []byte
	err := s.db.QueryRow(`SELECT data FROM job_samples WHERE job_id = ?`, jobID).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	var out []sample
	return out, json.Unmarshal(raw, &out)
}

// migrateStorage brings a database to the current layout: results without
// the per-job statistics columns, and the samples of every job that is not
// running packed into blobs. It copies the database file first.
func (s *store) migrateStorage(path string) error {
	var wide int
	if err := s.db.QueryRow(`SELECT count(*) FROM pragma_table_info('results') WHERE name = 'ns_lo'`).Scan(&wide); err != nil {
		return err
	}
	var loose int
	if err := s.db.QueryRow(`SELECT count(*) FROM (SELECT DISTINCT job_id FROM samples WHERE job_id NOT IN (SELECT id FROM jobs WHERE state IN (?, ?)))`, stateRunning, stateQueued).Scan(&loose); err != nil {
		return err
	}
	if wide == 0 && loose == 0 {
		return nil
	}
	if wide > 0 {
		backup := path + ".before-blobs"
		if _, err := os.Stat(backup); err != nil {
			if _, err := s.db.Exec(`VACUUM INTO ?`, backup); err != nil {
				return fmt.Errorf("backup: %w", err)
			}
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		for _, stmt := range []string{
			`DROP TABLE IF EXISTS results_slim`,
			resultsSchema("results_slim"),
			`INSERT INTO results_slim(` + resultColumns + `) SELECT ` + resultColumns + ` FROM results`,
			`DROP TABLE results`,
			`ALTER TABLE results_slim RENAME TO results`,
			`CREATE INDEX IF NOT EXISTS results_leaf ON results(object, op, engine, job_id)`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				tx.Rollback()
				return fmt.Errorf("slim results: %w", err)
			}
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	rows, err := s.db.Query(`SELECT DISTINCT job_id FROM samples WHERE job_id NOT IN (SELECT id FROM jobs WHERE state IN (?, ?))`, stateRunning, stateQueued)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if err := s.packSamples(id); err != nil {
			return fmt.Errorf("pack samples of job %d: %w", id, err)
		}
	}
	log.Printf("storage: results slimmed: %v, samples of %d jobs packed", wide > 0, len(ids))
	_, err = s.db.Exec(`VACUUM`)
	return err
}
