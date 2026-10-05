#!/usr/bin/env bash
# Run each package separately: Go cannot write distinct CPU profiles for ./...
# in one invocation. Analysis is attempted after both passing and failing tests.
set -u
set -o pipefail
GO=${GO:-go}
PROFILE_ROOT=${PROFILE_ROOT:-.cache/test-profiles}
PROFILE_MEM_RATE=${PROFILE_MEM_RATE:-524288}
mkdir -p "${TMPDIR:-.cache/tmp}" "${GOTMPDIR:-.cache/gotmp}"
patterns=()
flags=()
allow_empty=0
while (($#)); do
  if [[ $1 == -- ]]; then shift; flags=("$@"); break; fi
  patterns+=("$1"); shift
done
((${#patterns[@]})) || patterns=(./...)
for flag in "${flags[@]}"; do
  case "$flag" in -bench|-bench=*|-fuzz|-fuzz=*) allow_empty=1 ;; esac
  case "$flag" in
    -cpuprofile*|-memprofile*|-memprofilerate*|-o|-o=*|-outputdir*|-coverprofile*)
      echo "Profile/binary paths are managed by test-profile.sh; use PROFILE_ROOT or COVERAGE_FILE" >&2; exit 2 ;;
  esac
done
mkdir -p "$PROFILE_ROOT" || exit 1
run=$(mktemp -d "$PROFILE_ROOT/run-$(date -u +%Y%m%dT%H%M%SZ)-XXXXXX") || exit 1
run=$(cd "$run" && pwd)
printf 'Profiles and analysis: %s\n' "$run"
printf '%s\n' "$run" > "$PROFILE_ROOT/latest-run.txt"
printf 'go=%s\n' "$GO" > "$run/invocation.txt"
"$GO" version >> "$run/invocation.txt" 2>&1
{
  printf 'cwd=%s\nheap_sampling_bytes=%s\ncpu_sampling=Go test default (100Hz)\n' "$PWD" "$PROFILE_MEM_RATE"
  printf 'revision='; git rev-parse HEAD 2>/dev/null || printf 'external consumer (see parent release invocation)\n'
  git status --short 2>/dev/null || true
  "$GO" env GOOS GOARCH GOAMD64 CGO_ENABLED GOTOOLCHAIN
  for flag in "${flags[@]}"; do
    case "$flag" in -fuzz|-fuzz=*) printf 'Fuzz workers/subprocesses are not covered by parent profiles; profile representative seeds separately.\n' ;; esac
  done
} >> "$run/invocation.txt" 2>&1
printf 'package patterns:' >> "$run/invocation.txt"; printf ' %q' "${patterns[@]}" >> "$run/invocation.txt"
printf '\nflags:' >> "$run/invocation.txt"; printf ' %q' "${flags[@]}" >> "$run/invocation.txt"; printf '\n' >> "$run/invocation.txt"
list_flags=()
for ((i=0; i<${#flags[@]}; i++)); do
  case "${flags[i]}" in
    -tags|-mod|-modfile|-overlay) list_flags+=("${flags[i]}" "${flags[i+1]}"); ((i+=1)) ;;
    -tags=*|-mod=*|-modfile=*|-overlay=*|-race) list_flags+=("${flags[i]}") ;;
  esac
done
if ! "$GO" list "${list_flags[@]}" -f '{{.ImportPath}}|{{if or .TestGoFiles .XTestGoFiles}}tests{{else}}build-only{{end}}' "${patterns[@]}" > "$run/packages.txt" 2> "$run/list.log"; then
  cat "$run/list.log" >&2; exit 1
fi
printf 'package\ttest_status\tanalysis_status\n' > "$run/status.tsv"
if [[ -n ${COVERAGE_FILE:-} ]]; then : > "$run/coverage.out"; fi
failed=0
while IFS='|' read -r package kind; do
  [[ -n $package ]] || continue
  # Full import path avoids collisions between root/provider package names.
  out="$run/$package"
  mkdir -p "$out"
  args=(test "$package" -count=1 -timeout=20m "${flags[@]}" "-cpuprofile=$out/cpu.pprof" "-memprofile=$out/heap.pprof" "-memprofilerate=$PROFILE_MEM_RATE" "-o=$out/test.bin")
  if [[ -n ${COVERAGE_FILE:-} ]]; then args+=("-coverprofile=$out/coverage.out"); fi
  printf '%q ' "$GO" "${args[@]}" > "$out/command.txt"; printf '\n' >> "$out/command.txt"
  "$GO" "${args[@]}" > "$out/test.log" 2>&1
  status=$?
  cat "$out/test.log"
  if [[ $kind == tests && $allow_empty == 0 ]] && grep -q '\[no tests to run\]' "$out/test.log"; then
    # Benchmark-only packages legitimately have no Test functions in a full run.
    # A requested focused pattern with no matches is still a verification error.
    focused=0
    for flag in "${flags[@]}"; do case "$flag" in -run|-run=*) focused=1 ;; esac; done
    if ((focused)); then
      echo 'No tests matched; refusing a false verification pass.' >&2
      status=1
    else
      echo 'Benchmark-only package: built, profiles captured, no test execution.'
    fi
  fi
  analysis=0
  if [[ $kind == tests ]]; then
    for metric in alloc_space alloc_objects; do
      if [[ -s $out/heap.pprof && -s $out/test.bin ]]; then
        "$GO" tool pprof -top "-$metric" "$out/test.bin" "$out/heap.pprof" > "$out/$metric.txt" 2>&1 || analysis=1
        "$GO" tool pprof -top -cum "-$metric" "$out/test.bin" "$out/heap.pprof" > "$out/$metric-cum.txt" 2>&1 || analysis=1
      else
        printf 'Profile unavailable (build failure, process crash or interrupted run).\n' > "$out/$metric.txt"
        analysis=1
      fi
    done
    if [[ -s $out/cpu.pprof && -s $out/test.bin ]]; then
      "$GO" tool pprof -top -cum "$out/test.bin" "$out/cpu.pprof" > "$out/cpu.txt" 2>&1 || analysis=1
    else
      printf 'CPU profile unavailable (build failure, process crash or interrupted run).\n' > "$out/cpu.txt"
      analysis=1
    fi
    {
      printf 'Package: %s\nTest exit: %s\nAnalysis exit: %s\n\n' "$package" "$status" "$analysis"
      for report in alloc_space alloc_objects cpu; do
        printf '%s\n' "=== $report ==="; head -22 "$out/$report.txt"; printf '\n'
      done
      printf 'Compare matching workload, toolchain and flags before attributing changes.\n'
      printf 'Review the leading allocation/CPU call chains; record measured reductions or why no safe change applies.\n'
    } > "$out/analysis.txt"
    cat "$out/analysis.txt"
  fi
  if [[ -n ${COVERAGE_FILE:-} && -s $out/coverage.out ]]; then
    if [[ ! -s $run/coverage.out ]]; then head -1 "$out/coverage.out" > "$run/coverage.out"; fi
    if [[ $(head -1 "$run/coverage.out") != $(head -1 "$out/coverage.out") ]]; then
      echo 'Incompatible coverage modes across packages' >&2; analysis=1
    else
      tail -n +2 "$out/coverage.out" >> "$run/coverage.out"
    fi
  fi
  printf '%s\t%s\t%s\n' "$package" "$status" "$analysis" >> "$run/status.tsv"
  ((status == 0 && analysis == 0)) || failed=1
done < "$run/packages.txt"
if [[ -n ${COVERAGE_FILE:-} ]]; then
  if [[ ! -s $run/coverage.out ]]; then printf 'mode: set\n' > "$run/coverage.out"; fi
  cp "$run/coverage.out" "$COVERAGE_FILE" || failed=1
fi
printf 'Result: %s; profiles: %s\n' "$failed" "$run"
exit "$failed"
