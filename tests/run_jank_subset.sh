#!/usr/bin/env bash
set -euo pipefail
JOKER_BIN=${JOKER_BIN:-./joker}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
CP="$ROOT/tests/jank_harness:$ROOT/tests/jank_subset"
pass=0
fail=0
for f in "$ROOT"/tests/jank_subset/core_test/*.cljc; do
  name=$(basename "$f")
  if "$JOKER_BIN" --classpath "$CP" "$f" >/tmp/jank_subset.out 2>&1; then
    echo "PASS $name"
    pass=$((pass+1))
  else
    echo "FAIL $name"
    sed -n '1,8p' /tmp/jank_subset.out
    fail=$((fail+1))
  fi
done
echo "jank subset: $pass pass, $fail fail"
[ "$fail" -eq 0 ]
