# Joker Benchmarks

## CLBG Benchmarks (Computer Language Benchmarks Game)

13 CLBG benchmarks adapted from the [Benchmarks Game](https://benchmarksgame-team.pages.debian.net/benchmarksgame/), plus 2 additional runtime micro-workloads (`map_update_loop`, `word_frequency`). Scaled down for benchmark harness practicality.

### Current Results (5×5x min on i7-12700)

| Benchmark | Joker | Python 3.13 | Goja (Go JS) | vs Python | vs Goja |
|---|---:|---:|---:|---:|---:|
| tail-rec sum | 0.062ms | 5.48ms | 15.8ms | **0.01×** | 0.00× |
| arithmetic loop | 0.237ms | 8.07ms | 26.2ms | **0.03×** | 0.01× |
| recursive fib | 0.959ms | 16.6ms | 107.7ms | **0.06×** | 0.01× |
| pidigits | 0.016ms | 0.13ms | 0.20ms | **0.12×** | 0.08× |
| fasta | 0.066ms | 0.20ms | 0.65ms | **0.33×** | 0.10× |
| regex-redux | 0.083ms | 0.12ms | 0.18ms | 0.69× | 0.46× |
| spectral-norm | 17.4ms | 15.8ms | 105.2ms | 1.10× | 0.17× |
| nbody | 1.76ms | 0.57ms | 7.16ms | 3.09× | 0.25× |
| mandelbrot | 3.97ms | 5.24ms | 27.9ms | **0.76×** | 0.14× |
| binary-trees | 78.3ms | 35.8ms | 175.7ms | 2.19× | 0.45× |
| knucleotide | 0.251ms | 0.04ms | 0.57ms | 6.28× | 0.44× |
| reverse-comp | 0.043ms | 0.02ms | 0.11ms | 2.15× | 0.39× |
| fannkuch | 33.7ms | 3.59ms | 14.6ms | 9.39× | 2.31× |

**Beat Python: 6/13 | Beat Goja: 12/13**

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
| fib | 3361.6ms | 575.9ms | **go-joker** (5.8×) |
| loop-recur | 77.2ms | 6.45ms | **go-joker** (12.0×) |
| map-filter | 3.52ms | 6.54ms | let-go (1.9×) |
| persistent-map | 16.7ms | 24.1ms | let-go (1.4×) |
| reduce | 104.0ms | 214.5ms | let-go (2.1×) |
| tak | 3610.3ms | 705.8ms | **go-joker** (5.1×) |
| transducers | 3.98ms | 6.18ms | let-go (1.6×) |

**go-joker wins 3/7 (by 5–12×), let-go wins 4/7 (by 1.4–2.1×).**

For detailed analysis see [`docs/PARITY_STATUS.md`](../docs/PARITY_STATUS.md).

## Language Compliance Suite

Clojure language compliance is tracked separately from runtime speed using:

```bash
make parity
# -> docs/DIVERGENCE_MATRIX.md
```

Current result: **261/261 pass (100%)**.
