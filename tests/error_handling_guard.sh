#!/usr/bin/env bash
set -euo pipefail

root="${1:-.}"
cd "$root"

status=0

# Production code should not silently discard close/process shutdown errors.
if grep -R -E '_ = .*\.Close\(|_ = .*\.Kill\(|_, _ = .*\.Wait\(' core std \
  --exclude='*_test.go' \
  --exclude-dir='gen' >/tmp/go-joker-ignored-errors.txt; then
  cat /tmp/go-joker-ignored-errors.txt >&2
  status=1
fi

# Raw panic(err) should be wrapped in runtime errors outside standalone generators.
if grep -R 'panic(err)' core std \
  --exclude='*_test.go' \
  --exclude-dir='gen' >/tmp/go-joker-raw-panic-errors.txt; then
  cat /tmp/go-joker-raw-panic-errors.txt >&2
  status=1
fi

# File writes in production/runtime code should not be silently discarded.
if grep -R -E '_ = os\.WriteFile\(|if .*, err := json\.MarshalIndent\(.*; err == nil' core std \
  --exclude='*_test.go' \
  --exclude-dir='gen' \
  --exclude-dir='gen_code' >/tmp/go-joker-ignored-write-errors.txt; then
  cat /tmp/go-joker-ignored-write-errors.txt >&2
  status=1
fi

if [ "$status" -ne 0 ]; then
  echo "error handling guard: ignored close/process/write errors or raw panic(err) found" >&2
  exit "$status"
fi
