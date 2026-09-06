## Benchmark refresh: 2026-09-06

The refresh ran after the execution-tier audit, at source revision `d5121c1a`, on the Intel Core i7-12700 with six visible CPUs and Go 1.26.5. Initial load averages were 0.14/0.14/0.25; sampled CPU idle was 97–99%. Runs were sequential rather than competing with each other.

Joker measurements cover 26 benchmarks, five one-second Go samples each. Parsing is outside the timed section. Python 3.14.6, Bun 1.4.0/JSC, Goja and let-go were all rerun with the repository's five-iteration comparison scripts; output validation passed. The mirrored let-go suite used three warmups and ten timed samples.

## Results

The current best-Joker table wins 9/15 displayed workloads; Bun/JSC wins five and Python one. This table mixes explicitly selected native helpers with portable workloads and must not be described as a WASM benchmark.

| Workload | Portable ms/op | Best-Joker helper ms/op |
|---|---:|---:|
| Binary Trees | 76.326 | 3.259 |
| Mandelbrot | 0.140 | 0.077 |
| N-body | 294.214 | 0.004921 |
| Fannkuch | 250.070 | 0.140 |

The separate let-go suite favours go-joker on 2/7 workloads: loop-recur (8.84 versus 48.6 ms/op) and reduce (6.87 versus 67.2 ms/op). These are separate scripts and must not be substituted for the cross-runtime table.

The corrected pidigits fixture uses arbitrary-precision integer quotient, returns checksum 129 and measures about 4.063 ms/op. Its previous floating-point fixture was incorrect. Historical pidigits timings are excluded from baseline-speedup charts rather than presented as a regression or speedup.

## Reproduction and retained data

Current data: [raw results and environment](../benchmarks/results/2026-09-06/), [complete Joker timing/allocation summary](../benchmarks/results/2026-09-06/joker-summary.md), and [let-go suite](../benchmarks/results/2026-09-06/letgo-suite-comparison.md).

The [previous snapshot](../benchmarks/history/2026-05-22/) retains the old JSON and SVG files. Other dated benchmark reports remain historical records. Current charts are generated from `benchmarks/benchmark-history.json`; baseline charts exclude incompatible workloads. The historical baseline was not rerun, so those charts are historical comparisons, not controlled before/after tests of one patch.

```sh
python3 benchmarks/run_benchmarks.py --pkg ./benchmarks/core --runs 5 \
  --bench 'BenchmarkCLBG|BenchmarkEval' --benchtime 1s
make compare-bench
make benchmark-docs-check
```

Comparison scripts round runtime timings to hundredths of a millisecond, so small differences in the cross-runtime table should not be treated as statistical significance. Raw Go samples include variation and allocation counts; no broad application-performance claim follows from these microbenchmarks.
