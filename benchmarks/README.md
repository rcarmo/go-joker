# Joker Benchmarks

## CLBG Benchmarks (Computer Language Benchmarks Game)

13 benchmarks adapted from the [Benchmarks Game](https://benchmarksgame-team.pages.debian.net/benchmarksgame/). Scaled down for benchmark harness practicality.

### Current Results (5×5x min on i7-12700)

| Benchmark | Joker | Python 3.13 | Goja (Go JS) | vs Python | vs Goja |
|---|---:|---:|---:|---:|---:|
| tail-rec sum | 0.074ms | 3.60ms | 28.0ms | **0.02×** | 0.00× |
| arithmetic loop | 0.317ms | 6.65ms | 13.5ms | **0.05×** | 0.02× |
| recursive fib | 1.192ms | 21.0ms | 67.0ms | **0.06×** | 0.02× |
| pidigits | 0.020ms | 0.05ms | 0.15ms | **0.40×** | 0.13× |
| spectral-norm | 16.1ms | 24.5ms | 65.0ms | **0.66×** | 0.25× |
| fasta | 0.048ms | 0.06ms | 0.60ms | **0.80×** | 0.08× |
| regex-redux | 0.096ms | 0.09ms | 0.14ms | 1.07× | 0.69× |
| binary-trees | 98ms | 54ms | 172ms | 1.81× | 0.57× |
| mandelbrot | 14.3ms | 4.76ms | 39ms | 3.00× | 0.37× |
| k-nucleotide | 0.154ms | 0.03ms | 0.48ms | 5.13× | 0.32× |
| reverse-comp | 0.070ms | 0.01ms | 0.13ms | 7.00× | 0.54× |
| fannkuch | 110ms | 4.94ms | 24ms | 22× | 4.58× |
| n-body | 39ms | 0.66ms | 4.75ms | 59× | 8.23× |

**Beat Python: 6/13 | Beat Goja: 11/13**

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
13 algorithms in different runtimes. Each prints `name | avg ms/op | result`
for 5 iterations.

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
