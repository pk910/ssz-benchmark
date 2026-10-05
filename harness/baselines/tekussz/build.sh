#!/bin/bash
# Builds the Teku adapter at a commit of Consensys/teku:
#
#   build.sh <commit> <out>
#
# installs or updates the Java toolchain (toolchains/java.sh), fetches Teku
# at the commit into work/teku, generates the schema definitions for both
# forks and presets from the harness types, builds the ssz module and what
# it depends on with Teku's own Gradle wrapper (export.gradle copies the
# jars into out/lib), compiles the driver against them and leaves the
# launchers out/<fork>, which run the JVM with fixed flags: a 12 GB heap
# sized up front, the serial collector (one mutator thread, no concurrent
# collector threads next to it) and no perf data file. The JVM has no
# layout to shuffle, so the layout seeds leave no launcher of their own.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")"
COMMIT="$1"
OUT="$2"
source ../../toolchains/java.sh

# checkout <repo> <dir> <rev>: a shallow fetch of one revision.
checkout() {
  local repo="$1" dir="$2" rev="$3"
  if [ ! -d "$dir/.git" ]; then
    git init -q "$dir"
    git -C "$dir" remote add origin "$repo"
  fi
  git -C "$dir" fetch -q --depth 1 origin "$rev"
  git -C "$dir" checkout -q FETCH_HEAD
}
mkdir -p work
checkout https://github.com/Consensys/teku work/teku "$COMMIT"

PAYLOAD="${REAL_DATA:-/srv/benchd/res/real}"
python3 ../convert_foreign.py java ../../types/fulu/types.go src/bench/GenFulu.java --roots FuluBeaconState,ElectraSignedBeaconBlock \
  --preset mainnet="$PAYLOAD/fulu/spec.json" --preset minimal="$PAYLOAD/fulu/minimal/spec.json"
python3 ../convert_foreign.py java ../../types/gloas/types.go src/bench/GenGloas.java --roots GloasBeaconState,GloasSignedBeaconBlock,GloasSignedExecutionPayloadEnvelope \
  --preset mainnet="$PAYLOAD/gloas/spec.json" --preset minimal="$PAYLOAD/gloas/minimal/spec.json"

# The library: Teku's build compiles the ssz module and the modules it
# depends on and copies the jars into out/lib.
mkdir -p "$OUT"
rm -rf "$OUT/lib"
work/teku/gradlew -p work/teku --init-script "$PWD/export.gradle" -PbenchLibDir="$OUT/lib" \
  --console=plain -q :infrastructure:ssz:benchExport

# The driver, compiled against those jars (a generated file holds one
# class per preset, which the auxiliaryclass lint would refuse).
rm -rf build/classes
mkdir -p build/classes
javac --release 25 -Xlint:all,-auxiliaryclass -Werror -d build/classes -cp "$OUT/lib/*" src/bench/*.java
jar --create --file "$OUT/lib/bench.jar" -C build/classes .

for fork in fulu gloas; do
  cat > "$OUT/$fork" <<EOF
#!/bin/sh
exec "$JAVA_HOME/bin/java" -Xms12g -Xmx12g -XX:+UseSerialGC -XX:-UsePerfData -cp "$OUT/lib/*" bench.Main $fork "\$@"
EOF
  chmod +x "$OUT/$fork"
done
