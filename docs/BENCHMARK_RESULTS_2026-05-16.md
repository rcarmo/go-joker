# Benchmark results — 2026-05-16

Validated local rerun on `12th Gen Intel(R) Core(TM) i7-12700`. Go/Joker values are medians from 5 Go benchmark runs with `-benchtime=1s`; cross-runtime values come from `make compare-bench` and are accepted only after semantic result-payload validation.

## Direct runtime comparison

| Benchmark | Joker | Python 3.13 | Bun/JSC | Goja | let-go | Winner |
|---|---:|---:|---:|---:|---:|---|
| arithmetic-loop | 0.280ms | 6.34ms | 0.340ms | 11.0ms | 10.0ms | Joker |
| recursive-fib | 1.02ms | 12.0ms | 0.840ms | 42.4ms | 30.0ms | Bun/JSC |
| tail-recursive-sum | 0.067ms | 4.01ms | 0.250ms | 9.55ms | 5.84ms | Joker |
| map-update-loop | 0.002ms | 0.380ms | 0.110ms | 1.04ms | 2.82ms | Joker |
| word-frequency | 0.475ms | 0.630ms | 0.110ms | 0.870ms | 25.5ms | Bun/JSC |
| nbody | 0.006ms | 0.280ms | 0.200ms | 4.18ms | 1.91ms | Joker |
| spectral-norm | 0.151ms | 10.9ms | 0.610ms | 44.4ms | 34.6ms | Joker |
| binary-trees | 4.79ms | 25.7ms | 4.26ms | 134.8ms | 120.3ms | Bun/JSC |
| fannkuch | 0.174ms | 6.44ms | 0.480ms | 21.7ms | 20.4ms | Joker |
| mandelbrot | 0.095ms | 3.13ms | 0.280ms | 30.5ms | 11.7ms | Joker |
| fasta | 0.055ms | 0.160ms | 0.030ms | 0.310ms | 0.140ms | Bun/JSC |
| knucleotide | 0.010ms | 0.030ms | 0.060ms | 0.280ms | 0.460ms | Joker |
| reverse-complement | 0.000ms | 0.010ms | 0.020ms | 0.050ms | 0.120ms | Joker |
| regex-redux | 0.086ms | 0.160ms | 0.060ms | 0.070ms | 0.090ms | Bun/JSC |
| pidigits | 0.009ms | 0.050ms | 0.080ms | 0.140ms | 0.250ms | Joker |

Summary: Joker wins 10/15 workloads; Bun/JSC wins 5/15. Joker beats Python on 15/15, Goja on 14/15, and let-go on 15/15.

## let-go suite comparison

| Benchmark | let-go | go-joker | Winner |
|---|---:|---:|---|
| fib | 2467.2 | 2835.5 | let-go |
| loop-recur | 70.8 | 7.88 | go-joker |
| map-filter | 3.47 | 6.16 | let-go |
| persistent-map | 19.6 | 18.9 | go-joker |
| reduce | 89.1 | 6.27 | go-joker |
| tak | 2549.1 | 3899.8 | let-go |
| transducers | 3.43 | 7.91 | let-go |


In this rerun go-joker wins 3/7 mirrored let-go suite workloads (`loop-recur`, `persistent-map`, and `reduce`).

## Validation performed

- `benchmarks/run_benchmarks.py` parsed 15/15 selected Go benchmarks for all 5 requested runs.
- `make compare-bench` validated Python, Bun/Node, Goja, and let-go result payloads before rendering tables.
- `run_letgo_suite.go` required validated go-joker outputs for every mirrored let-go suite benchmark.
- Chart generators were rerun from `benchmarks/benchmark-history.json`.
