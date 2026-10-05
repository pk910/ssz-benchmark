#!/bin/bash
# Java: one pinned Temurin JDK from Adoptium; Gradle comes with each
# project's wrapper and keeps its caches under the toolchain directory.
source "$(dirname "${BASH_SOURCE[0]}")/common.sh"
JDK_RELEASE="${JDK_RELEASE:-jdk-25.0.4.1+1}"
JDK_DIR="$BENCH_TOOLCHAINS/$(echo "$JDK_RELEASE" | tr '+' '_')"
if [ ! -f "$JDK_DIR/.complete" ]; then
  echo "toolchain: installing $JDK_RELEASE"
  fetch "https://api.adoptium.net/v3/binary/version/$(echo "$JDK_RELEASE" | sed 's/+/%2B/')/linux/x64/jdk/hotspot/normal/eclipse" "$JDK_DIR.tar.gz"
  unpack "$JDK_DIR.tar.gz" "$JDK_DIR"
fi
export JAVA_HOME="$JDK_DIR"
export PATH="$JAVA_HOME/bin:$PATH"
export GRADLE_USER_HOME="$BENCH_TOOLCHAINS/gradle"
export GRADLE_OPTS="-Dorg.gradle.daemon=false -Dorg.gradle.workers.max=$BENCH_JOBS -Dorg.gradle.parallel=false"
