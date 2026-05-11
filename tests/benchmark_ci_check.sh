#!/usr/bin/env bash
set -euo pipefail

results=${1:-bench-results.txt}

if [[ ! -f "$results" ]]; then
  echo "benchmark results file not found: $results" >&2
  exit 1
fi

# CI benchmark checks are smoke/regression guards, not performance targets.
# They intentionally use broad ceilings because GitHub runners vary widely.
# Keep these aligned with the interpreter benchmarks in core/vm_bench_test.go;
# those benchmarks currently exercise Eval over parsed forms, not native codegen.
check_bench() {
  local name="$1"
  local max_ns="$2"
  local values
  mapfile -t values < <(awk -v n="$name" '$1 ~ "^" n "-" { print $3 }' "$results" | sed 's/\..*$//' | sort -n)
  if [[ ${#values[@]} -eq 0 ]]; then
    echo "FAIL: ${name} not found in results" >&2
    return 1
  fi
  local median=${values[$((${#values[@]} / 2))]}
  echo "${name}: median ${median} ns/op (ceiling: ${max_ns} ns/op, samples: ${#values[@]})"
  if (( median > max_ns )); then
    echo "FAIL: ${name} regression detected: ${median} > ${max_ns}" >&2
    return 1
  fi
}

# Broad ceilings based on current interpreter baselines with headroom for
# slower shared runners. These catch order-of-magnitude regressions without
# failing CI for normal runner noise.
check_bench "BenchmarkFib10"        250000
check_bench "BenchmarkFib20"        15000000
check_bench "BenchmarkTak"          50000000
check_bench "BenchmarkLoopRecur1M"  10000000
