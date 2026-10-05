#!/usr/bin/env bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/scripts/project-env.sh" || exit 1
set -euo pipefail
JOKER_BIN=${JOKER_BIN:-$CLI_BIN}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
CP="$ROOT/tests/jank_harness:$ROOT/tests/jank_subset"
work=$(mktemp -d "$TMPDIR/jank-subset.XXXXXX")
trap 'rm -rf "$work"' EXIT
pass=0
fail=0
for f in "$ROOT"/tests/jank_subset/core_test/*.cljc; do
  name=$(basename "$f")
  if "$JOKER_BIN" --classpath "$CP" "$f" >"$work/jank_subset.out" 2>&1; then
    echo "PASS $name"
    pass=$((pass+1))
  else
    echo "FAIL $name"
    sed -n '1,8p' "$work/jank_subset.out"
    fail=$((fail+1))
  fi
done
echo "jank subset: $pass pass, $fail fail"
[ "$fail" -eq 0 ]
