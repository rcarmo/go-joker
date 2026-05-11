# Benchmark CI guard

Updated: 2026-05-11

GitHub Actions runs two benchmark steps after the normal test job:

1. VM/interpreter microbenchmarks from `core/vm_bench_test.go`.
2. CLBG benchmark smoke runs.

The CI regression check is intentionally a **smoke ceiling**, not a performance target. Shared GitHub runner speed varies enough that tight historical thresholds create false failures. The guard therefore checks the median of repeated samples against broad ceilings in:

```text
tests/benchmark_ci_check.sh
```

That script is the source of truth for benchmark CI thresholds. Keep the workflow thin and call the script rather than duplicating threshold logic in YAML.

## Local reproduction

```bash
go test ./core \
  -bench 'BenchmarkCall|BenchmarkFib|BenchmarkTak|BenchmarkLoop|BenchmarkReduce|BenchmarkClosure|BenchmarkMap|BenchmarkVector|BenchmarkTransduce' \
  -benchmem -benchtime=1s -count=3 > bench-results.txt

tests/benchmark_ci_check.sh bench-results.txt
```

## Policy

- Treat missing benchmark names as failures.
- Use medians across samples instead of the first run.
- Keep ceilings broad enough to avoid runner-noise failures.
- Tight performance targets belong in benchmark history/reports, not CI gating.
- If a benchmark changes from interpreter/tree-walker behavior to native/typed/WASM behavior, update this document and the script comments together.
