#!/usr/bin/env bash
set -euo pipefail

ROOT=${1:-.}
cd "$ROOT"

cleanup=.github/workflows/actions-cleanup.yml
ci=.github/workflows/ci.yml
release=.github/workflows/build.yml
benchmark=.github/workflows/benchmark-regression.yml

[[ -f $cleanup ]] || { echo "missing consolidated Actions cleanup workflow" >&2; exit 1; }
[[ ! -e .github/workflows/prune-actions-artifacts.yml ]] || {
  echo "legacy cleanup workflow name returned; keep cleanup consolidated" >&2
  exit 1
}

for workflow in 'CI' 'Release' 'Benchmark regression' 'Race stress'; do
  grep -Fq -- "- \"$workflow\"" "$cleanup" || {
    echo "cleanup is not triggered by completed $workflow workflows" >&2
    exit 1
  }
done

for setting in \
  "KEEP_RUNS_PER_WORKFLOW: '10'" \
  "KEEP_CACHES: '100'" \
  "STANDARD_ARTIFACT_DAYS: '7'" \
  "BENCHMARK_ARTIFACT_DAYS: '30'" \
  "CACHE_RETENTION_DAYS: '7'" \
  "RUN_RETENTION_DAYS: '30'"; do
  grep -Fq "$setting" "$cleanup" || {
    echo "cleanup policy is missing $setting" >&2
    exit 1
  }
done

grep -Fq "artifact.name === 'benchmark-comparison'" "$cleanup" || {
  echo "cleanup does not preserve the benchmark evidence class" >&2
  exit 1
}
grep -Fq 'actions: write' "$cleanup" || { echo "cleanup lacks actions:write" >&2; exit 1; }
grep -Fq 'contents: read' "$cleanup" || { echo "cleanup lacks contents:read" >&2; exit 1; }

for spec in "$ci:7" "$release:7" "$benchmark:30"; do
  file=${spec%:*}
  days=${spec##*:}
  grep -Fq "retention-days: $days" "$file" || {
    echo "$file does not declare the documented $days-day artifact retention" >&2
    exit 1
  }
done

grep -Fq 'docker://rhysd/actionlint:1.7.7' "$ci" || {
  echo "CI is missing the pinned actionlint gate" >&2
  exit 1
}
grep -Fq 'docs/CI_RETENTION.md' Makefile || {
  echo "canonical documentation checks do not require the retention policy" >&2
  exit 1
}

echo "workflow retention policy guard passed"
