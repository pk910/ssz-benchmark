#!/bin/bash
# Shared script to run all SSZ benchmarks
# Writes results to <library>_results.txt files in the root directory
# Usage: ./scripts/run-benchmarks.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"

echo "Running fastssz-v1 benchmarks..."
cd benchmarks/fastssz-v1
go mod download
go test -run=^$ -bench=. -benchmem -count=5 > "$ROOT_DIR/fastssz-v1_results.txt"
cd "$ROOT_DIR"

echo "Running fastssz-v2 benchmarks..."
cd benchmarks/fastssz-v2
go mod download
go test -run=^$ -bench=. -benchmem -count=5 > "$ROOT_DIR/fastssz-v2_results.txt"
cd "$ROOT_DIR"

echo "Running dynamicssz-codegen benchmarks..."
cd benchmarks/dynamicssz-codegen
go mod download
go test -run=^$ -bench=. -benchmem -count=5 > "$ROOT_DIR/dynamicssz-codegen_results.txt"
cd "$ROOT_DIR"

echo "Running dynamicssz-reflection benchmarks..."
cd benchmarks/dynamicssz-reflection
go mod download
go test -run=^$ -bench=. -benchmem -count=5 > "$ROOT_DIR/dynamicssz-reflection_results.txt"
cd "$ROOT_DIR"

echo "Running karalabessz benchmarks..."
cd benchmarks/karalabessz
go mod download
go test -run=^$ -bench=. -benchmem -count=5 > "$ROOT_DIR/karalabessz_results.txt"
cd "$ROOT_DIR"

echo "All benchmarks completed!"
