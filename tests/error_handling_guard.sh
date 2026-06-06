#!/usr/bin/env bash
set -euo pipefail

root="${1:-.}"
cd "$root"

mkdir -p .cache/tmp
ignored_errors_file="$(mktemp .cache/tmp/go-joker-ignored-errors.XXXXXX)"
raw_panic_file="$(mktemp .cache/tmp/go-joker-raw-panic-errors.XXXXXX)"
ignored_writes_file="$(mktemp .cache/tmp/go-joker-ignored-write-errors.XXXXXX)"
trap 'rm -f "$ignored_errors_file" "$raw_panic_file" "$ignored_writes_file"' EXIT

status=0

# Production code should not silently discard close/process shutdown errors.
if grep -R -E '_ = .*\.Close\(|_ = .*\.Kill\(|_, _ = .*\.Wait\(' core std \
  --exclude='*_test.go' \
  --exclude-dir='gen' >"$ignored_errors_file"; then
  cat "$ignored_errors_file" >&2
  status=1
fi

# Raw panic(err) should be wrapped in runtime errors outside standalone generators.
if grep -R 'panic(err)' core std \
  --exclude='*_test.go' \
  --exclude-dir='gen' >"$raw_panic_file"; then
  cat "$raw_panic_file" >&2
  status=1
fi

# File writes in production/runtime code should not be silently discarded.
if grep -R -E '_ = os\.WriteFile\(|if .*, err := json\.MarshalIndent\(.*; err == nil' core std \
  --exclude='*_test.go' \
  --exclude-dir='gen' \
  --exclude-dir='gen_code' >"$ignored_writes_file"; then
  cat "$ignored_writes_file" >&2
  status=1
fi

if [ "$status" -ne 0 ]; then
  echo "error handling guard: ignored close/process/write errors or raw panic(err) found" >&2
  exit "$status"
fi
