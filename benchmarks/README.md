# Joker Benchmarks

## CLBG Benchmarks (Computer Language Benchmarks Game)

13 CLBG benchmarks adapted from the [Benchmarks Game](https://benchmarksgame-team.pages.debian.net/benchmarksgame/), plus 2 additional runtime micro-workloads (`map_update_loop`, `word_frequency`). Scaled down for benchmark harness practicality.

### Current Results (5×5x min on i7-12700)

| Benchmark | Joker | Python 3.13 | Goja (Go JS) | vs Python | vs Goja |
|---|---:|---:|---:|---:|---:|
| tail-rec sum | 0.062ms | 5.04ms | 16.2ms | **0.01×** | 0.00× |
| arithmetic loop | 0.237ms | 6.2ms | 21.8ms | **0.04×** | 0.01× |
| recursive fib | 0.959ms | 15ms | 88.6ms | **0.06×** | 0.01× |
| pidigits | 0.016ms | 0.13ms | 0.23ms | **0.12×** | 0.07× |
| fasta | 0.066ms | 0.26ms | 0.59ms | **0.25×** | 0.11× |
| regex-redux | 0.083ms | 0.17ms | 0.2ms | **0.49×** | 0.41× |
| mandelbrot | 3.97ms | 4.97ms | 38.9ms | **0.80×** | 0.10× |
| spectral-norm | 17.4ms | 16.7ms | 74.3ms | 1.04× | 0.23× |
| nbody | 1.76ms | 0.43ms | 5.2ms | 4.09× | 0.34× |
| binary-trees | 78.3ms | 39ms | 148.1ms | 2.01× | 0.53× |
| knucleotide | 0.251ms | 0.13ms | 0.51ms | 1.93× | 0.49× |
| reverse-comp | 0.043ms | 0.03ms | 0.13ms | 1.43× | 0.33× |
| fannkuch | 33.7ms | 3.63ms | 20.3ms | 9.28× | 1.66× |

**Beat Python: 7/13 | Beat Goja: 12/13**

## Parallel benchmark variants

Parallel variants are intentionally tracked separately from the single-threaded baseline benchmarks so language/runtime parity numbers remain comparable while still exposing multicore throughput.

Run these with a fixed Go scheduler width:

```bash
GOMAXPROCS=4 TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core -run '^$' -bench '^BenchmarkCLBGBinaryTrees$' -benchmem -benchtime=10x

GOMAXPROCS=4 TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core -run '^$' -bench '^BenchmarkCLBGBinaryTreesParallel$' -benchmem -benchtime=10x
```

Current audit results, run one benchmark per isolated `tmux` session with `GOMAXPROCS=4` on i7-12700:

| Benchmark | Time | Allocated | Allocs | Notes |
|---|---:|---:|---:|---|
| `BenchmarkCLBGBinaryTrees` | 62.1ms/op | 38.8MB/op | 1,096,473/op | single-thread baseline after captured `*Fn` IR dispatch fix |
| `BenchmarkCLBGBinaryTreesParallel` | 62.1ms/op | 77.6MB/op | 2,192,176/op | `pmap` over independent tree depths; no longer faster once the baseline uses the same IR path |

Audit notes:

- `binary-trees` was the best initial concurrency candidate because each depth/iteration group is independent, but after fixing captured `*Fn` IR dispatch the single-thread baseline caught up; keep the variant as a concurrency smoke benchmark rather than a published speedup claim.
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

## Running Benchmarks

```bash
# CLBG + eval benchmarks
python3 benchmarks/run_benchmarks.py --runs 5 --bench 'BenchmarkCLBG|BenchmarkEval' --benchtime 5x

# Parser benchmarks
go test ./core -bench 'BenchmarkParse|BenchmarkDecode' -benchmem -benchtime 100x

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
| mandelbrot | 159ms | **14ms** | **11×** |
| binary-trees | 528ms | **98ms** | **5.4×** |
| spectral-norm | 70ms | **16ms** | **4.4×** |
| pidigits | 0.10ms | **0.020ms** | **5×** |
| fasta | 0.22ms | **0.048ms** | **4.6×** |

See [OPTIMIZATION_REPORT.md](../docs/OPTIMIZATION_REPORT.md) for the full architecture documentation.

## let-go Benchmark Suite Parity

Direct head-to-head against [let-go](https://github.com/nooga/let-go)'s benchmark suite.

| Benchmark | let-go | go-joker | Winner |
|---|---:|---:|---|
| fib | 3252.2ms | 480.5ms | **go-joker** (6.8×) |
| loop-recur | 95.5ms | 7.26ms | **go-joker** (13.2×) |
| map-filter | 4.34ms | 5.83ms | let-go (1.34×) |
| persistent-map | 16.5ms | 17.8ms | let-go (1.08×) |
| reduce | 99.1ms | 5.93ms | **go-joker** (16.7×) |
| tak | 3590.1ms | 514.3ms | **go-joker** (7.0×) |
| transducers | 3.90ms | 6.05ms | let-go (1.55×) |

**go-joker wins 4/7; remaining gaps are map-filter (~1.34×), transducers (~1.55×), and persistent-map (near parity).**

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
