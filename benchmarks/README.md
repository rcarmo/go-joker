# Joker Benchmarks

## CLBG Benchmarks (Computer Language Benchmarks Game)

13 CLBG benchmarks adapted from the [Benchmarks Game](https://benchmarksgame-team.pages.debian.net/benchmarksgame/), plus 2 additional runtime micro-workloads (`map_update_loop`, `word_frequency`). Scaled down for benchmark harness practicality.

### Current Results (best-Joker, 5×5x median on i7-12700)

| Benchmark | Joker | Python 3.13 | Bun/JSC | Goja | let-go | Winner |
|---|---:|---:|---:|---:|---:|---|
| arithmetic-loop | 0.308ms | 8.39ms | 0.270ms | 19.2ms | 14.7ms | Bun/JSC |
| recursive-fib | 1.22ms | 24.3ms | 1.09ms | 80.4ms | 51.4ms | Bun/JSC |
| tail-recursive-sum | 0.077ms | 8.81ms | 0.260ms | 14.7ms | 7.80ms | Joker |
| map-update-loop | 0.002ms | 0.250ms | 0.110ms | 1.66ms | 2.91ms | Joker |
| word-frequency | 0.533ms | 0.460ms | 0.210ms | 3.51ms | 25.3ms | Bun/JSC |
| nbody | 0.006ms | 0.710ms | 0.210ms | 6.27ms | 4.84ms | Joker |
| spectral-norm | 0.136ms | 20.4ms | 0.620ms | 86.0ms | 54.3ms | Joker |
| binary-trees | 4.24ms | 47.1ms | 5.78ms | 166.3ms | 120.5ms | Joker |
| fannkuch | 0.244ms | 2.09ms | 0.480ms | 16.8ms | 14.2ms | Joker |
| mandelbrot | 0.116ms | 2.74ms | 0.370ms | 33.3ms | 14.2ms | Joker |
| fasta | 0.139ms | 0.070ms | 0.030ms | 0.530ms | 0.240ms | Bun/JSC |
| knucleotide | 0.008ms | 0.030ms | 0.050ms | 0.470ms | 0.450ms | Joker |
| reverse-complement | 0.001ms | 0.010ms | 0.020ms | 0.080ms | 0.070ms | Joker |
| regex-redux | 0.068ms | 0.120ms | 0.080ms | 0.170ms | 0.100ms | Joker |
| pidigits | 0.047ms | 0.060ms | 0.020ms | 0.180ms | 0.310ms | Bun/JSC |

**Best-Joker beats Python on 13/15, Goja on 15/15, and let-go on 15/15 displayed workloads.**

### Runtime micro-workloads

`BenchmarkEvalWordFrequency` now uses native whitespace tokenization plus native `frequencies` over string keys instead of regex `re-seq` and interpreted persistent-map churn. Current focused result:

| Benchmark | Before | Current | Allocation change |
|---|---:|---:|---:|
| `BenchmarkEvalWordFrequency` | 181ms/op, 49.9MB/op, 612k allocs/op | 0.533ms/op, 0.536MB/op, 8.1k allocs/op | ~340× faster, ~93× fewer allocations |

### Benchmark intent taxonomy

The benchmark suite now separates portable/literal ports from best-Joker runtime shapes. Portable results remain useful for cross-language parity, but best-Joker variants show what users should write when targeting Joker's strongest execution paths. `core/benchmark_results_test.go` now pins representative portable, micro, and best-Joker outputs so timing-only benchmark changes cannot silently drift away from the intended computations.

| Benchmark | Intent | Notes |
|---|---|---|
| `BenchmarkEvalTailRecursiveSum` | portable/stress | Measures recursive function self-call overhead (`irCallSelf`). Useful runtime stress, but not comparable to Python's `while` loop. |
| `BenchmarkEvalTailRecursiveSumLoopRecur` | best-Joker | Equivalent sum written as `loop/recur`, matching Python's iterative shape and hitting `irRecur`. |
| `BenchmarkEvalWordFrequency` | best-Joker | Uses native tokenization + `frequencies` instead of regex plus interpreted persistent-map updates. |
| `BenchmarkEvalMapUpdateLoop` | portable/stress | Persistent-map update loop. Generic transient rewrite was rejected; `BenchmarkEvalMapUpdateLoopBestJoker` uses a native small-count helper. |
| `BenchmarkCLBGNBodyBestJoker` | best-Joker/native | Uses flat native `float64` state for the 5-body simulation. |
| `BenchmarkCLBGSpectralNormBestJoker` | best-Joker/native | Uses native `float64` slices and tight matrix-vector loops. |
| `BenchmarkCLBGMandelbrotBestJoker` | best-Joker/native | Uses native nested numeric loops for the pixel count. |
| `BenchmarkCLBGFannkuchReduxBestJoker` | best-Joker/native | Uses mutable local permutation arrays. |
| `BenchmarkCLBGBinaryTreesBestJoker` | best-Joker/native | Uses native tree nodes instead of persistent vectors. |
| `BenchmarkCLBGKnucleotideBestJoker` | best-Joker/native | Uses a native k-mer distinct-count helper instead of interpreted substring construction plus persistent-map churn. |
| `BenchmarkCLBGReverseComplementBestJoker` | best-Joker/native | Uses a byte-slice native reverse-complement helper instead of repeated string concatenation. |
| `BenchmarkCLBGRegexReduxBestJoker` | best-Joker/native | Uses count-only native regex matching instead of `re-seq` object materialization. |
| `BenchmarkCLBGBinaryTreesParallel` | concurrency smoke/best-Joker | Uses `pmap` over independent depth work; keep separate from portable single-thread CLBG. |
| `BenchmarkCLBGFasta`, `BenchmarkCLBGPidigits` | portable/stress | Already small/native enough in this suite; no separate best-Joker variant accepted. |

Focused tail-sum audit (`-benchtime=100x`):

| Benchmark | Time | Allocated | Allocs | Meaning |
|---|---:|---:|---:|---|
| `BenchmarkEvalTailRecursiveSum` | 12.37ms/op | 3.20MB/op | 299,770/op | recursive function path |
| `BenchmarkEvalTailRecursiveSumLoopRecur` | 0.080ms/op | 775B/op | 7/op | best-Joker iterative path |

Focused best-Joker CLBG/string audit (`-benchtime=50x`):

| Benchmark | Portable | Best-Joker | Speedup |
|---|---:|---:|---:|
| nbody | 2.39ms/op | 0.00445ms/op | 536× |
| spectral-norm | 51.4ms/op | 0.117ms/op | 439× |
| binary-trees | 99.3ms/op | 5.17ms/op | 19.2× |
| fannkuch | 22.6ms/op | 0.258ms/op | 87.7× |
| mandelbrot | 5.72ms/op | 0.093ms/op | 61.5× |
| k-nucleotide | 0.230ms/op | 0.0126ms/op | 18.3× |
| reverse-complement | 0.046ms/op | 0.000374ms/op | 124× |
| regex-redux | 0.351ms/op | 0.083ms/op | 4.2× |
| map-update-loop native helper | 0.598ms/op | 0.00163ms/op | 367× |
| map-update-loop transient attempt | 0.823ms/op | rejected: 184.9ms/op | generic transient dispatch too costly here |

## Parallel benchmark variants

Parallel variants are intentionally tracked separately from the single-threaded baseline benchmarks so language/runtime parity numbers remain comparable while still exposing multicore throughput.

Run these with a fixed Go scheduler width:

```bash
GOMAXPROCS=4 TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core -run '^$' -bench '^BenchmarkCLBGBinaryTrees$' -benchmem -benchtime=30x

GOMAXPROCS=4 TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
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
for 5 iterations.

For reproducible direct comparison, use the `benchmarks/compare` sub-project:

```bash
make compare-bench
# -> benchmarks/compare/out/latest/direct-comparison.md
# -> benchmarks/compare/out/latest/letgo-suite-comparison.md
```

```bash
python3 benchmarks/cross_lang_bench.py        # CPython 3.13
bun     benchmarks/cross_lang_bench.js        # Bun (or node)
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
and `.js` files — not portable Clojure source.

## Optimization Session Progress

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

## let-go Benchmark Suite Parity

Direct head-to-head against [let-go](https://github.com/nooga/let-go)'s benchmark suite.

| Benchmark | let-go | go-joker | Winner |
|---|---:|---:|---|
| fib | 1934.9ms | 1269.6ms | **go-joker** (1.5×) |
| loop-recur | 73.8ms | 8.70ms | **go-joker** (8.5×) |
| map-filter | 5.37ms | 7.48ms | let-go (1.39×) |
| persistent-map | 29.9ms | 26.2ms | **go-joker** (1.1×) |
| reduce | 109.1ms | 7.81ms | **go-joker** (14.0×) |
| tak | 2804.1ms | 2547.6ms | **go-joker** (1.1×) |
| transducers | 3.15ms | 5.40ms | let-go (1.71×) |

**go-joker wins 5/7; remaining gaps are map-filter (~1.39×) and transducers (~1.71×).**

For detailed analysis see [`docs/PARITY_STATUS.md`](../docs/PARITY_STATUS.md).

## Language Compliance Suite

Clojure language compliance is tracked separately from runtime speed using:

```bash
make parity
# -> docs/DIVERGENCE_MATRIX.md

make jank-subset
# -> runs imported jank-lang/clojure-test-suite smoke subset
```

Current result: **271/271 pass (100%)** plus **7/7 imported jank-suite smoke files**.
