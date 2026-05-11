#!/usr/bin/env bash
set -euo pipefail

root=${1:-.}
cd "$root"

check() {
  local pattern=$1
  local file=$2
  if ! grep -qiE "$pattern" "$file"; then
    echo "non-goals guard: missing pattern '$pattern' in $file" >&2
    return 1
  fi
}

status=0
check 'Java|JVM|reflection|classpath' docs/BABASHKA_FIT_GAP.md || status=1
check 'bb\.edn|tasks|deps|Maven|Clojars' docs/BABASHKA_FIT_GAP.md || status=1
check 'SCI' docs/BABASHKA_FIT_GAP.md || status=1
check 'library catalog|babashka\.curl|cheshire|selmer|rewrite-clj' docs/BABASHKA_SHIM_ASSESSMENT.md || status=1
check 'CLI.*identity|bb.*CLI' docs/BABASHKA_FIT_GAP.md || status=1
check 'syscall|unix|low-level' docs/PORTABILITY_SHIM_ASSESSMENT.md || status=1

exit "$status"
