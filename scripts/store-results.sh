#!/bin/bash
# Script to run all SSZ benchmarks and store results in JSON format
# Usage: ./scripts/store-results.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

cd "$ROOT_DIR"

# Create results directory if it doesn't exist
mkdir -p results

# Try to fetch existing results from benchmark-results branch
echo "Fetching existing results from benchmark-results branch..."
if git ls-remote --heads origin benchmark-results 2>/dev/null | grep -q benchmark-results; then
    git fetch origin benchmark-results
    # Extract existing JSON files from the branch
    git archive origin/benchmark-results -- results/ 2>/dev/null | tar -x 2>/dev/null || echo "No existing results found"
else
    echo "No benchmark-results branch found, starting fresh"
fi

# Run all benchmarks using shared script
"$SCRIPT_DIR/run-benchmarks.sh"

echo "Processing results and updating JSON files..."
python3 << 'EOF'
import re
import json
import os
import time

MAX_RESULTS = 1000

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

def extract_version(go_mod_path, package_pattern):
    """Extract version of a package from go.mod file."""
    try:
        with open(go_mod_path, 'r') as f:
            content = f.read()
        match = re.search(rf'{package_pattern}\s+(v[\d.]+(?:-[\w.]+)?)', content)
        if match:
            return match.group(1)
    except FileNotFoundError:
        pass
    return "unknown"

def convert_benchmark_name(bench_name):
    """Convert benchmark name to result key format."""
    # BenchmarkBlockMainnet_Unmarshal -> UnmarshalMainnetBlock
    # BenchmarkStateMainnet_Marshal -> MarshalMainnetState
    match = re.match(r'Benchmark(Block|State)(Mainnet|Minimal)_(\w+)', bench_name)
    if match:
        data_type = match.group(1)
        preset = match.group(2)
        operation = match.group(3)
        return f"{operation}{preset}{data_type}"
    return bench_name

def load_existing_json(filepath):
    """Load existing JSON file or return empty structure."""
    if os.path.exists(filepath):
        try:
            with open(filepath, 'r') as f:
                data = json.load(f)
                print(f"  Loaded {len(data.get('benchmarks', []))} existing results from {filepath}")
                return data
        except (json.JSONDecodeError, IOError) as e:
            print(f"  Warning: Could not load {filepath}: {e}")
    else:
        print(f"  No existing file at {filepath}, starting fresh")
    return {"benchmarks": []}

def save_json(filepath, data, pretty=False):
    """Save data to JSON file."""
    with open(filepath, 'w') as f:
        if pretty:
            json.dump(data, f, indent=2)
        else:
            json.dump(data, f, separators=(',', ':'))

def load_existing_aggregation(filepath):
    """Load existing aggregation file or return empty structure."""
    if os.path.exists(filepath):
        try:
            with open(filepath, 'r') as f:
                data = json.load(f)
                print(f"  Loaded existing aggregation with {len(data.get('aggregations', []))} versions from {filepath}")
                return data
        except (json.JSONDecodeError, IOError) as e:
            print(f"  Warning: Could not load aggregation {filepath}: {e}")
    else:
        print(f"  No existing aggregation at {filepath}, starting fresh")
    return {"aggregations": []}

def update_aggregation(existing_aggregation, version, new_results):
    """Update aggregation data incrementally for a specific version."""
    # Find existing entry for this version
    version_entry = None
    for entry in existing_aggregation.get("aggregations", []):
        if entry.get("version") == version:
            version_entry = entry
            break

    # If no existing entry, create new one
    if version_entry is None:
        version_entry = {
            "version": version,
            "results": {}
        }
        existing_aggregation["aggregations"].append(version_entry)

    # Update each benchmark result incrementally
    for key, values in new_results.items():
        ns_val, bytes_val, alloc_val = values[0], values[1], values[2]

        if key in version_entry["results"]:
            # Update existing aggregation incrementally
            existing = version_entry["results"][key]
            old_samples = existing["samples"]
            new_samples = old_samples + 1

            # Update average: new_avg = (old_avg * old_samples + new_value) / new_samples
            existing["ns_op"][0] = (existing["ns_op"][0] * old_samples + ns_val) / new_samples
            existing["ns_op"][1] = min(existing["ns_op"][1], ns_val)
            existing["ns_op"][2] = max(existing["ns_op"][2], ns_val)

            existing["bytes"][0] = (existing["bytes"][0] * old_samples + bytes_val) / new_samples
            existing["bytes"][1] = min(existing["bytes"][1], bytes_val)
            existing["bytes"][2] = max(existing["bytes"][2], bytes_val)

            existing["alloc"][0] = (existing["alloc"][0] * old_samples + alloc_val) / new_samples
            existing["alloc"][1] = min(existing["alloc"][1], alloc_val)
            existing["alloc"][2] = max(existing["alloc"][2], alloc_val)

            existing["samples"] = new_samples
        else:
            # Create new entry for this benchmark
            version_entry["results"][key] = {
                "samples": 1,
                "ns_op": [ns_val, ns_val, ns_val],
                "bytes": [bytes_val, bytes_val, bytes_val],
                "alloc": [alloc_val, alloc_val, alloc_val]
            }

    # Sort by version (newest first)
    existing_aggregation["aggregations"].sort(key=lambda x: x["version"], reverse=True)

    return existing_aggregation

def process_benchmark(name, results_file, go_mod_path, package_pattern, json_file):
    """Process a benchmark and update its JSON file."""
    print(f"Processing {name}...")

    results = parse_benchmark_results(results_file)
    if not results:
        print(f"  No results found for {name}")
        return

    version = extract_version(go_mod_path, package_pattern)
    print(f"  Version: {version}")

    # Convert results to the desired format
    formatted_results = {}
    for bench_name, data in results.items():
        key = convert_benchmark_name(bench_name)
        formatted_results[key] = [
            data['ns_op'],
            data['bytes_op'],
            data['allocs']
        ]

    # Create new benchmark entry
    new_entry = {
        "time": int(time.time()),
        "version": version,
        "results": formatted_results
    }

    # Load existing data and append
    data = load_existing_json(json_file)
    data["benchmarks"].append(new_entry)

    # Apply retention - keep only the last MAX_RESULTS
    if len(data["benchmarks"]) > MAX_RESULTS:
        removed = len(data["benchmarks"]) - MAX_RESULTS
        data["benchmarks"] = data["benchmarks"][-MAX_RESULTS:]
        print(f"  Applied retention: removed {removed} old results, keeping {MAX_RESULTS}")

    # Save updated data
    save_json(json_file, data)
    print(f"  Saved {len(data['benchmarks'])} results to {json_file}")

    # Load existing aggregation and update incrementally
    aggregation_file = json_file.replace('.json', '-aggregation.json')
    aggregation_data = load_existing_aggregation(aggregation_file)
    aggregation_data = update_aggregation(aggregation_data, version, formatted_results)
    save_json(aggregation_file, aggregation_data, pretty=True)
    print(f"  Updated aggregation ({len(aggregation_data['aggregations'])} versions) in {aggregation_file}")

# Define benchmarks to process
benchmarks = [
    {
        "name": "fastssz-v1",
        "results_file": "fastssz-v1_results.txt",
        "go_mod_path": "benchmarks/fastssz-v1/go.mod",
        "package_pattern": r"github\.com/ferranbt/fastssz",
        "json_file": "results/fastssz-v1.json"
    },
    {
        "name": "fastssz-v2",
        "results_file": "fastssz-v2_results.txt",
        "go_mod_path": "benchmarks/fastssz-v2/go.mod",
        "package_pattern": r"github\.com/ferranbt/fastssz",
        "json_file": "results/fastssz-v2.json"
    },
    {
        "name": "dynamicssz-codegen",
        "results_file": "dynamicssz-codegen_results.txt",
        "go_mod_path": "benchmarks/dynamicssz-codegen/go.mod",
        "package_pattern": r"github\.com/pk910/dynamic-ssz",
        "json_file": "results/dynamicssz-codegen.json"
    },
    {
        "name": "dynamicssz-reflection",
        "results_file": "dynamicssz-reflection_results.txt",
        "go_mod_path": "benchmarks/dynamicssz-reflection/go.mod",
        "package_pattern": r"github\.com/pk910/dynamic-ssz",
        "json_file": "results/dynamicssz-reflection.json"
    },
    {
        "name": "karalabessz",
        "results_file": "karalabessz_results.txt",
        "go_mod_path": "benchmarks/karalabessz/go.mod",
        "package_pattern": r"github\.com/karalabe/ssz",
        "json_file": "results/karalabessz.json"
    }
]

for benchmark in benchmarks:
    process_benchmark(
        benchmark["name"],
        benchmark["results_file"],
        benchmark["go_mod_path"],
        benchmark["package_pattern"],
        benchmark["json_file"]
    )

print("\nAll results processed successfully!")
EOF

# Clean up temporary result files
rm -f fastssz-v1_results.txt fastssz-v2_results.txt dynamicssz-codegen_results.txt dynamicssz-reflection_results.txt karalabessz_results.txt

echo "Done!"
