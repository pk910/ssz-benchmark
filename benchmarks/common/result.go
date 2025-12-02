// Package common provides shared benchmark infrastructure
package common

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// BenchmarkResult represents the result of a single benchmark
type BenchmarkResult struct {
	Name         string  `json:"name"`
	Iterations   int     `json:"iterations"`
	NsPerOp      float64 `json:"ns_per_op"`
	BytesPerOp   int64   `json:"bytes_per_op"`
	AllocsPerOp  int64   `json:"allocs_per_op"`
}

// BenchmarkSuite represents results for a complete benchmark suite
type BenchmarkSuite struct {
	Library   string             `json:"library"`
	Timestamp time.Time          `json:"timestamp"`
	Results   []*BenchmarkResult `json:"results"`
}

// WriteResults writes benchmark results to a JSON file
func WriteResults(suite *BenchmarkSuite, filename string) error {
	data, err := json.MarshalIndent(suite, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal results: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write results: %w", err)
	}

	return nil
}

// LoadResults loads benchmark results from a JSON file
func LoadResults(filename string) (*BenchmarkSuite, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read results: %w", err)
	}

	var suite BenchmarkSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("failed to unmarshal results: %w", err)
	}

	return &suite, nil
}
