#!/bin/bash
# Script to run all SSZ benchmarks and update the README with results
# Usage: ./scripts/update-readme.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"

echo "Running fastssz benchmarks..."
cd benchmarks/fastssz
go mod download
go test -run=^$ -bench=. -benchmem -count=5 > ../../fastssz_results.txt
cd "$ROOT_DIR"

echo "Running dynamic-ssz benchmarks (codegen)..."
cd benchmarks/dynamicssz
go mod download
go test -run=^$ -bench=. -benchmem -count=5 > ../../dynamicssz_results.txt
cd "$ROOT_DIR"

echo "Running dynamic-ssz benchmarks (reflection)..."
cd benchmarks/dynamicssz-reflection
go mod download
go test -run=^$ -bench=. -benchmem -count=5 > ../../dynamicssz_reflection_results.txt
cd "$ROOT_DIR"

echo "Running karalabe-ssz benchmarks..."
cd benchmarks/karalabessz
go mod download
go test -run=^$ -bench=. -benchmem -count=5 > ../../karalabessz_results.txt
cd "$ROOT_DIR"

echo "Updating README..."
python3 << 'EOF'
import re
import sys
from datetime import datetime

def parse_benchmark_results(filename):
    """Parse benchmark results from Go test output."""
    results = {}
    try:
        with open(filename, 'r') as f:
            content = f.read()

        # Parse benchmark lines
        pattern = r'(Benchmark\w+)-\d+\s+(\d+)\s+([\d.]+)\s+ns/op\s+(\d+)\s+B/op\s+(\d+)\s+allocs/op'
        matches = re.findall(pattern, content)

        for match in matches:
            name, iterations, ns_op, bytes_op, allocs = match
            if name not in results:
                results[name] = {'ns_op': [], 'bytes_op': [], 'allocs': []}
            results[name]['ns_op'].append(float(ns_op))
            results[name]['bytes_op'].append(int(bytes_op))
            results[name]['allocs'].append(int(allocs))

        # Average the results
        for name in results:
            results[name] = {
                'ns_op': sum(results[name]['ns_op']) / len(results[name]['ns_op']),
                'bytes_op': sum(results[name]['bytes_op']) / len(results[name]['bytes_op']),
                'allocs': sum(results[name]['allocs']) / len(results[name]['allocs'])
            }
    except FileNotFoundError:
        print(f"Warning: {filename} not found")

    return results

def format_ns(ns):
    """Format nanoseconds to human-readable string."""
    if ns >= 1_000_000_000:
        return f"{ns/1_000_000_000:.2f}s"
    elif ns >= 1_000_000:
        return f"{ns/1_000_000:.2f}ms"
    elif ns >= 1_000:
        return f"{ns/1_000:.2f}µs"
    else:
        return f"{ns:.0f}ns"

def format_bytes(b):
    """Format bytes to human-readable string."""
    if b >= 1_000_000:
        return f"{b/1_000_000:.2f}MB"
    elif b >= 1_000:
        return f"{b/1_000:.2f}KB"
    else:
        return f"{int(b)}B"

# Parse all results
fastssz = parse_benchmark_results('fastssz_results.txt')
dynamicssz = parse_benchmark_results('dynamicssz_results.txt')
dynamicssz_refl = parse_benchmark_results('dynamicssz_reflection_results.txt')
karalabessz = parse_benchmark_results('karalabessz_results.txt')

def get_benchmark_value(results, key, field):
    if key in results:
        return results[key][field]
    return None

def make_table_row(lib_name, results, bench_name, op):
    """Generate a table row for a benchmark."""
    val = get_benchmark_value(results, bench_name, 'ns_op')
    mem = get_benchmark_value(results, bench_name, 'bytes_op')
    allocs = get_benchmark_value(results, bench_name, 'allocs')
    if val is not None:
        return f"| {lib_name} | {op} | {format_ns(val)} | {format_bytes(mem)} | {int(allocs)} |\n"
    return ""

# Build the results section
results_md = f"""## Benchmark Results

Last updated: {datetime.now().strftime('%Y-%m-%d %H:%M:%S UTC')}

### Block Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
"""

# Block Mainnet
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('fastssz', fastssz, f'BenchmarkBlockMainnet_{op}', op)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('dynamic-ssz (codegen)', dynamicssz, f'BenchmarkBlockMainnet_{op}', op)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('dynamic-ssz (reflection)', dynamicssz_refl, f'BenchmarkBlockMainnet_{op}', op)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('karalabe-ssz', karalabessz, f'BenchmarkBlockMainnet_{op}', op)

results_md += """
### State Mainnet Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
"""

# State Mainnet
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('fastssz', fastssz, f'BenchmarkStateMainnet_{op}', op)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('dynamic-ssz (codegen)', dynamicssz, f'BenchmarkStateMainnet_{op}', op)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('dynamic-ssz (reflection)', dynamicssz_refl, f'BenchmarkStateMainnet_{op}', op)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('karalabe-ssz', karalabessz, f'BenchmarkStateMainnet_{op}', op)

results_md += """
### Block Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
"""

# Block Minimal (karalabe-ssz doesn't support minimal)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('fastssz', fastssz, f'BenchmarkBlockMinimal_{op}', op)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('dynamic-ssz (codegen)', dynamicssz, f'BenchmarkBlockMinimal_{op}', op)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('dynamic-ssz (reflection)', dynamicssz_refl, f'BenchmarkBlockMinimal_{op}', op)

results_md += """
### State Minimal Benchmarks

| Library | Operation | Time | Memory | Allocations |
|---------|-----------|------|--------|-------------|
"""

# State Minimal (karalabe-ssz doesn't support minimal)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('fastssz', fastssz, f'BenchmarkStateMinimal_{op}', op)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('dynamic-ssz (codegen)', dynamicssz, f'BenchmarkStateMinimal_{op}', op)
for op in ['Unmarshal', 'Marshal', 'HashTreeRoot']:
    results_md += make_table_row('dynamic-ssz (reflection)', dynamicssz_refl, f'BenchmarkStateMinimal_{op}', op)

results_md += """
**Note:** karalabe-ssz does not support minimal preset out of the box.
"""

# Read current README
with open('README.md', 'r') as f:
    readme = f.read()

# Replace results section
start_marker = '<!-- BENCHMARK_RESULTS_START -->'
end_marker = '<!-- BENCHMARK_RESULTS_END -->'

if start_marker in readme and end_marker in readme:
    pattern = f'{start_marker}.*?{end_marker}'
    new_content = f'{start_marker}\n{results_md}\n{end_marker}'
    readme = re.sub(pattern, new_content, readme, flags=re.DOTALL)
else:
    readme += f'\n{start_marker}\n{results_md}\n{end_marker}\n'

with open('README.md', 'w') as f:
    f.write(readme)

print("README.md updated successfully")
EOF

# Clean up result files
rm -f fastssz_results.txt dynamicssz_results.txt dynamicssz_reflection_results.txt karalabessz_results.txt

echo "Done!"
