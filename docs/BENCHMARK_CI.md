# Benchmark CI and release baselines

Updated: 2026-07-10

The project has two deliberately separate performance checks.

## CI smoke ceiling

The regular CI job runs VM/interpreter microbenchmarks and CLBG smoke runs. Shared GitHub runner speed varies enough that historical absolute timings would create false failures, so `tests/benchmark_ci_check.sh` checks medians against broad ceilings. It remains the source of truth for smoke thresholds.

## Same-runner A/B regression comparison

`.github/workflows/benchmark-regression.yml` compares a baseline ref and a candidate ref on the same runner. Pull requests use their base and head commits; `workflow_dispatch` accepts explicit refs, which is the supported way to compare a release tag with a candidate release.

Both sides capture ten 500 ms samples of this stable representative set:

- recursive and tail-recursive execution (`Fib20`, `Tak`, `LoopRecur1M`);
- sequence reduction and closures;
- persistent map/vector operations;
- transducer pipelines.

The workflow stores the raw baseline, raw candidate, and `benchstat` report for 30 days. Those artifacts are the release benchmark baseline and comparison evidence; absolute timings are not copied between runner types.

### Local capture and comparison

```bash
make benchmark-capture BENCH_OUT=.cache/benchmarks/baseline.txt
# Check out or build the candidate, then:
make benchmark-capture BENCH_OUT=.cache/benchmarks/candidate.txt
make benchmark-compare \
  BENCH_BASELINE=.cache/benchmarks/baseline.txt \
  BENCH_OUT=.cache/benchmarks/candidate.txt
```

`benchmark-compare` runs the pinned `go tool benchstat` and then `tests/benchmark_regression_check.py`.

## Regression policy

A comparison fails when:

- either side has fewer than six samples;
- benchmark sets differ;
- median time grows by more than 15% **and** timing coefficient of variation is at most 10% on both sides;
- stable median bytes/op grows by more than 5% and at least eight bytes;
- median allocations/op increases.

Noisy timing and bytes measurements are reported but not gated. Allocation counts are always checked. This combination catches stable regressions without turning transient shared-runner noise into release blockers.

## Maintenance policy

- Treat missing benchmark names as failures.
- Change the benchmark set intentionally and on both refs before using it as a gate.
- Keep smoke ceilings broad; tight targets belong in same-runner A/B evidence.
- Do not compare absolute timings from different architectures or runner classes.
- If execution changes from interpreter/tree-walker behavior to native, typed, or WASM behavior, update this document and the relevant scripts together.
