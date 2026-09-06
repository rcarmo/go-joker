# Joker Benchmarks

## CLBG Benchmarks (Computer Language Benchmarks Game)

13 CLBG benchmarks adapted from the [Benchmarks Game](https://benchmarksgame-team.pages.debian.net/benchmarksgame/), plus 2 additional runtime micro-workloads (`map_update_loop`, `word_frequency`). Scaled down for benchmark harness practicality.

### Validated comparison tables (2026-09-06)

Fresh runs on the i7-12700, after checking 97–99% CPU idle. Joker values are medians of five one-second Go samples; Python, Bun/JSC, Goja and let-go were all rerun and output-validated. All times below are milliseconds/op. Best-Joker/native helpers and portable interpreted execution are different workloads; these results do not identify the WASM tier.

| Benchmark | Joker | Python | Bun/JSC | Goja | let-go | Winner |
|---|---:|---:|---:|---:|---:|---|
| arithmetic-loop | 0.467ms | 5.370ms | 0.200ms | 10.480ms | 8.160ms | Bun/JSC |
| recursive-fib | 35.081ms | 10.000ms | 0.790ms | 40.060ms | 25.850ms | Bun/JSC |
| tail-recursive-sum | 0.083ms | 3.830ms | 0.230ms | 6.860ms | 4.980ms | Joker |
| spectral-norm | 0.096ms | 12.080ms | 0.710ms | 37.000ms | 27.460ms | Joker |
| binary-trees | 3.259ms | 20.510ms | 3.600ms | 99.670ms | 84.860ms | Joker |
| nbody | 0.004921ms | 0.460ms | 0.230ms | 2.710ms | 1.660ms | Joker |
| fannkuch | 0.140ms | 7.580ms | 0.540ms | 15.590ms | 13.380ms | Joker |
| mandelbrot | 0.077ms | 5.560ms | 0.280ms | 17.280ms | 8.190ms | Joker |
| fasta | 0.259ms | 0.090ms | 0.020ms | 0.230ms | 0.150ms | Bun/JSC |
| pidigits | 4.063ms | 0.070ms | 0.110ms | 0.190ms | 0.140ms | Python |
| knucleotide | 0.007836ms | 0.050ms | 0.070ms | 0.240ms | 0.220ms | Joker |
| reverse-complement | 0.000311ms | 0.010ms | 0.040ms | 0.050ms | 0.110ms | Joker |
| regex-redux | 0.058ms | 0.100ms | 0.040ms | 0.110ms | 0.070ms | Bun/JSC |
| map-update-loop | 0.001535ms | 0.270ms | 0.150ms | 0.990ms | 1.790ms | Joker |
| word-frequency | 0.356ms | 0.470ms | 0.190ms | 1.070ms | 19.100ms | Bun/JSC |

Joker wins **9/15** displayed workloads, Bun/JSC **5/15**, and Python **1/15**. The separate mirrored let-go suite favours go-joker on **2/7** workloads (loop-recur and reduce).

The full 26-benchmark Joker samples, environment details and cross-runtime outputs are in [results/2026-09-06](results/2026-09-06/). The previous snapshot and charts are preserved in [history/2026-05-22](history/2026-05-22/).

The corrected pidigits fixture uses arbitrary-precision integer quotient and checksum 129. Its historical timings are not comparable and are excluded from baseline-speedup charts. Portable Binary Trees measured 76.326 ms/op; its best-Joker helper measured 3.259 ms/op. Portable Mandelbrot measured 0.140 ms/op; the best-Joker helper measured 0.077 ms/op. See the [complete Joker summary](results/2026-09-06/joker-summary.md) for allocations, variation and all portable/helper pairs.

Older optimisation notes below are historical measurements, not this refresh.

## Parallel benchmark variants

Parallel variants are intentionally tracked separately from the single-threaded baseline benchmarks so language/runtime parity numbers remain comparable while still exposing multicore throughput.

Run these with a fixed Go scheduler width:

```bash
mkdir -p .cache/tmp .cache/gotmp
GOMAXPROCS=4 TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/gotmp \
  go test ./core -run '^$' -bench '^BenchmarkCLBGBinaryTrees$' -benchmem -benchtime=30x

GOMAXPROCS=4 TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/gotmp \
  go test ./core -run '^$' -bench '^BenchmarkCLBGBinaryTreesParallel$' -benchmem -benchtime=30x
```

Current audit results, run one benchmark per isolated `tmux` session with `GOMAXPROCS=4` on i7-12700:

| Benchmark | Time | Allocated | Allocs | Notes |
|---|---:|---:|---:|---|
| `BenchmarkCLBGBinaryTrees` | 101.0ms/op | 38.8MB/op | 1,096,463/op | single-thread baseline |
| `BenchmarkCLBGBinaryTreesParallel` | 51.1ms/op | 77.6MB/op | 2,192,161/op | `pmap` over independent tree depths; **1.98× faster** |

Audit notes:

- `binary-trees` remains the accepted concurrency variant: each depth/iteration group is independent and coarse enough to amortize `pmap` overhead when benchmarked in isolated sessions.
- `mandelbrot` row-level parallelism and `spectral-norm` row-level parallelism were tested but rejected for now because their scaled-down benchmark sizes are too small; goroutine/list materialization overhead outweighed the work.
- A `pcalls` variant of recursive `fib` exposed a missing IR dispatch path for captured/self-recursive `*Fn` calls from the tree walker. That correctness/performance bug is fixed, but the parallel benchmark variant is still rejected because `pcalls` overhead dominates at this scaled-down size.

## Parser Benchmarks (Pure Implementations)

Same recursive-descent algorithm in each language. No native/C library calls.

| Parser | Joker | Python 3.13 | Bun/JSC | vs Python | vs Bun |
|---|---:|---:|---:|---:|---:|
| JSON small (78 chars) | 337µs | 17.9µs | 2.1µs | 19× | 161× |
| JSON medium (340 chars) | 1421µs | 52.5µs | 11.6µs | 27× | 123× |
| XML small (80 chars) | 523µs | 11.6µs | 2.3µs | 45× | 227× |
| XML medium (330 chars) | 2041µs | 46.6µs | 9.7µs | 44× | 210× |
| YAML small (45 chars) | 153µs | 2.3µs | 1.8µs | 67× | 85× |
| YAML medium (180 chars) | 478µs | 7.2µs | 5.2µs | 66× | 92× |
| HTML small (50 chars) | 85µs | 4.8µs | 1.1µs | 18× | 77× |
| HTML medium (200 chars) | 599µs | 23.3µs | 5.5µs | 26× | 109× |

### Native Go-backed Parsing (std/ namespaces)

| Parser | Joker Native | Python (pure) | vs Python |
|---|---:|---:|---:|
| JSON small | 3.4µs | 17.9µs | **5× faster** |
| JSON medium | 17.3µs | 52.5µs | **3× faster** |
| HTML small | 0.22µs | 4.8µs | **22× faster** |
| HTML medium | 1.6µs | 23.3µs | **15× faster** |

## Latest profile audit

A full benchmark/profile pass was recorded in [`docs/BENCHMARK_PROFILE_2026-05-12.md`](../docs/BENCHMARK_PROFILE_2026-05-12.md). The main finding is that portable/interpreted CLBG shapes are dominated by allocation churn and Go GC scanning; best-Joker/native-shaped variants remain low-allocation and fast. Near-term optimization should prioritize repeated IR compile/envelope allocation and IR frame-stack allocation over opcode-level micro-optimizations.

## Running Benchmarks

```bash
# CLBG + eval benchmarks
python3 benchmarks/run_benchmarks.py --runs 5 --bench 'BenchmarkCLBG|BenchmarkEval' --benchtime 5x

# Parser benchmarks
go test ./benchmarks/core -bench 'BenchmarkParse|BenchmarkDecode' -benchmem -benchtime 100x

# Native parser benchmarks
go test ./std/json ./std/yaml ./std/html -bench 'BenchmarkNative' -benchmem

# JIT namespace benchmarks
go test ./std/jit -bench 'BenchmarkJIT' -benchmem
```

### Cross-language reimplementations

The `cross_lang_bench.{py,js,clj}` files are self-contained ports of the same
15 workloads in different runtimes. Each prints `name | avg ms/op | result`
for 5 iterations. The output payload is part of the contract: `make compare-bench`
validates every reported `result` before using timings so table/chart updates cannot
silently compare different computations.

For reproducible direct comparison, use the `benchmarks/compare` sub-project:

```bash
make compare-bench
# validates python/bun-or-node/goja/let-go result payloads when those runtimes run
# cleans stale comparison outputs before collecting new data
# -> benchmarks/compare/out/latest/direct-comparison.md
# -> benchmarks/compare/out/latest/letgo-suite-comparison.md
```

```bash
python3 benchmarks/cross_lang_bench.py        # CPython 3.13
bun     benchmarks/cross_lang_bench.js        # Bun; Node fallback is supported by collect.sh
lg      benchmarks/cross_lang_bench.clj       # let-go
```

[let-go](https://github.com/nooga/let-go) is a small Clojure-like language
with a bytecode stack VM written in Go. Install with Homebrew:

```bash
brew tap nooga/let-go https://github.com/nooga/let-go
brew install let-go
```

(Or build from source — `go install github.com/nooga/let-go@latest`.) The
`.clj` file is a let-go port of the same algorithms, analogous to the `.py`
and `.js` files — not portable Clojure source. Optional runtimes may produce a
whole-file `# SKIPPED` marker, but malformed benchmark-looking lines, duplicate
results, missing result payloads, and mixed skip/output files are rejected.

Benchmark chart generators read from `benchmarks/benchmark-history.json` and fail
fast on missing or non-positive required values. `make docs-check` also validates
the README benchmark table and the Python summary parser, including decimal
`ns/op` values from fast Go benchmarks.

## Historical Optimization Session Progress

Session start → final (best-of-5 min values):

| Benchmark | Start | Final | Speedup |
|---|---:|---:|---:|
| nbody | 34.2ms | **0.006ms** | **5700×** |
| mandelbrot | 159ms | **0.116ms** | **1370×** |
| spectral-norm | 70ms | **0.136ms** | **515×** |
| binary-trees | 528ms | **4.24ms** | **125×** |
| fannkuch | 94.1ms | **0.244ms** | **386×** |
| word-frequency | 279.9ms | **0.533ms** | **525×** |
| pidigits | 0.10ms | **0.047ms** | **2.1×** |
| fasta | 0.22ms | **0.139ms** | **1.6×** |

See [OPTIMIZATION_REPORT.md](../docs/OPTIMIZATION_REPORT.md) for the full architecture documentation.

## let-go Benchmark Suite Parity (2026-09-06)

Three warmups and ten timed runs; current results in ms/op.

| Benchmark | let-go | go-joker | Winner |
|---|---:|---:|---|
| fib | 1733.2 | 2134.1 | let-go |
| loop-recur | 48.6 | 8.84 | go-joker |
| map-filter | 3.17 | 6.02 | let-go |
| persistent-map | 15.0 | 16.9 | let-go |
| reduce | 67.2 | 6.87 | go-joker |
| tak | 1887.8 | 2637.2 | let-go |
| transducers | 2.84 | 6.05 | let-go |

Go-joker wins 2/7 workloads: loop-recur and reduce. See [raw results](results/2026-09-06/letgo-suite-comparison.md).

## Language Compliance Suite

Clojure language compliance is tracked separately from runtime speed using:

```bash
make parity
# -> docs/DIVERGENCE_MATRIX.md

make jank-subset
# -> runs imported jank-lang/clojure-test-suite smoke subset
```

Current result: **271/271 pass (100%)** plus **7/7 imported jank-suite smoke files**.
