#!/usr/bin/env bash
set -euo pipefail

baseline=${1:?usage: benchmark_compare.sh BASELINE CANDIDATE [REPORT]}
candidate=${2:?usage: benchmark_compare.sh BASELINE CANDIDATE [REPORT]}
report=${3:-}

for file in "$baseline" "$candidate"; do
  if [[ ! -s "$file" ]]; then
    echo "benchmark input is missing or empty: $file" >&2
    exit 1
  fi
done

if [[ -n "$report" ]]; then
  mkdir -p "$(dirname "$report")"
  go tool benchstat "$baseline" "$candidate" | tee "$report"
else
  go tool benchstat "$baseline" "$candidate"
fi
PYTHONDONTWRITEBYTECODE=1 python3 "$(dirname "$0")/benchmark_regression_check.py" "$baseline" "$candidate"
