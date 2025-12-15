#!/bin/bash
# Script to update SSZ library versions to the latest dev versions from git HEAD
# Updates both go.mod and generate.go files, then runs go mod tidy & go generate
# Usage: ./scripts/update-dev-versions.sh

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(dirname "$SCRIPT_DIR")"

# Function to get the latest pseudo-version for a git repository
# Arguments: $1 = repository URL (e.g., github.com/ferranbt/fastssz)
# Returns: pseudo-version in format v0.0.0-YYYYMMDDHHMMSS-<12 char commit hash>
get_pseudo_version() {
    local repo_url="$1"
    local git_url="https://${repo_url}.git"

    # Get the latest commit hash and date from remote HEAD
    local git_info
    git_info=$(git ls-remote "$git_url" HEAD 2>/dev/null | head -1)

    if [ -z "$git_info" ]; then
        echo "Error: Could not fetch git info for $repo_url" >&2
        return 1
    fi

    local commit_hash
    commit_hash=$(echo "$git_info" | awk '{print $1}')
    local short_hash="${commit_hash:0:12}"

    # Get the commit date by cloning shallowly
    local tmp_dir
    tmp_dir=$(mktemp -d)

    git clone --depth 1 --bare "$git_url" "$tmp_dir/repo" 2>/dev/null
    local commit_date
    # Use committer date in UTC format (Go modules require UTC timestamps)
    commit_date=$(TZ=UTC git -C "$tmp_dir/repo" log -1 --format='%cd' --date=format-local:'%Y%m%d%H%M%S' 2>/dev/null)

    rm -rf "$tmp_dir"

    if [ -z "$commit_date" ]; then
        echo "Error: Could not get commit date for $repo_url" >&2
        return 1
    fi

    echo "v0.0.0-${commit_date}-${short_hash}"
}

# Function to update a go.mod file with a new version
# Arguments: $1 = go.mod path, $2 = module name, $3 = new version
update_go_mod() {
    local go_mod_path="$1"
    local module_name="$2"
    local new_version="$3"

    if [ ! -f "$go_mod_path" ]; then
        echo "Error: go.mod not found at $go_mod_path" >&2
        return 1
    fi

    # Use sed to replace the version for the specific module
    # Match the module name and replace the version (handles both tagged and pseudo-versions)
    sed -i -E "s|(${module_name}) v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1 ${new_version}|g" "$go_mod_path"

    echo "Updated $go_mod_path: $module_name -> $new_version"
}

# Function to update a generate.go file with a new version
# Arguments: $1 = generate.go path, $2 = module name (with subpath for tool), $3 = new version
update_generate_go() {
    local generate_go_path="$1"
    local module_pattern="$2"
    local new_version="$3"

    if [ ! -f "$generate_go_path" ]; then
        echo "Error: generate.go not found at $generate_go_path" >&2
        return 1
    fi

    # Use sed to replace the version in go:generate directives
    # Match patterns like @v1.0.0 or @v0.0.0-20251126100127-9cb620c1e0d0
    sed -i -E "s|(${module_pattern})@v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1@${new_version}|g" "$generate_go_path"

    echo "Updated $generate_go_path: $module_pattern -> $new_version"
}

# Function to update a shell script (like generate.sh) with a new version
# Arguments: $1 = script path, $2 = module pattern, $3 = new version
update_generate_sh() {
    local script_path="$1"
    local module_pattern="$2"
    local new_version="$3"

    if [ ! -f "$script_path" ]; then
        echo "Error: Script not found at $script_path" >&2
        return 1
    fi

    # Use sed to replace the version in go run commands
    sed -i -E "s|(${module_pattern})@v[0-9]+\.[0-9]+\.[0-9]+(-[^ ]+)?|\1@${new_version}|g" "$script_path"

    echo "Updated $script_path: $module_pattern -> $new_version"
}

# Function to run go mod tidy and go generate for a benchmark
# Arguments: $1 = benchmark directory path
run_go_commands() {
    local benchmark_dir="$1"

    echo "Running go mod tidy in $benchmark_dir..."
    (cd "$benchmark_dir" && go mod tidy)

    echo "Running rm gen_*.go in $benchmark_dir..."
    (cd "$benchmark_dir" && rm -f gen_*.go)

    echo "Running go generate in $benchmark_dir..."
    (cd "$benchmark_dir" && go generate .)
}

# Main update logic

echo "=========================================="
echo "Updating SSZ library dev versions"
echo "=========================================="

# Update fastssz-v2
echo ""
echo "--- Updating fastssz-v2 ---"
FASTSSZ_VERSION=$(get_pseudo_version "github.com/ferranbt/fastssz")
echo "Latest fastssz version: $FASTSSZ_VERSION"

update_go_mod "$ROOT_DIR/benchmarks/fastssz-v2/go.mod" "github.com/ferranbt/fastssz" "$FASTSSZ_VERSION"
update_generate_go "$ROOT_DIR/benchmarks/fastssz-v2/generate.go" "github.com/ferranbt/fastssz/sszgen" "$FASTSSZ_VERSION"
run_go_commands "$ROOT_DIR/benchmarks/fastssz-v2"

# Update karalabessz (if dev version tracking is desired)
echo ""
echo "--- Updating karalabessz ---"
KARALABESSZ_VERSION=$(get_pseudo_version "github.com/karalabe/ssz")
echo "Latest karalabe/ssz version: $KARALABESSZ_VERSION"

update_go_mod "$ROOT_DIR/benchmarks/karalabessz/go.mod" "github.com/karalabe/ssz" "$KARALABESSZ_VERSION"
update_generate_sh "$ROOT_DIR/benchmarks/karalabessz/generate.sh" "github.com/karalabe/ssz/cmd/sszgen" "$KARALABESSZ_VERSION"
run_go_commands "$ROOT_DIR/benchmarks/karalabessz"

# Update dynamicssz-codegen
echo ""
echo "--- Updating dynamicssz-codegen ---"
DYNAMICSSZ_VERSION=$(get_pseudo_version "github.com/pk910/dynamic-ssz")
echo "Latest dynamic-ssz version: $DYNAMICSSZ_VERSION"

update_go_mod "$ROOT_DIR/benchmarks/dynamicssz-codegen/go.mod" "github.com/pk910/dynamic-ssz" "$DYNAMICSSZ_VERSION"
update_generate_go "$ROOT_DIR/benchmarks/dynamicssz-codegen/generate.go" "github.com/pk910/dynamic-ssz/dynssz-gen" "$DYNAMICSSZ_VERSION"
run_go_commands "$ROOT_DIR/benchmarks/dynamicssz-codegen"

# Update dynamicssz-reflection
echo ""
echo "--- Updating dynamicssz-reflection ---"
# Reuse the same version from above
update_go_mod "$ROOT_DIR/benchmarks/dynamicssz-reflection/go.mod" "github.com/pk910/dynamic-ssz" "$DYNAMICSSZ_VERSION"
run_go_commands "$ROOT_DIR/benchmarks/dynamicssz-reflection"

echo ""
echo "=========================================="
echo "All dev versions updated successfully!"
echo "=========================================="
