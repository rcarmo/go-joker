# go-joker

![icon](docs/icon-256)

An optimized fork of [Joker](https://github.com/candid82/joker) (Clojure-like Lisp interpreter) for inclusion in [gi](https://github.com/rcarmo/gi), a self-hosted coding agent.

## What this is

This is a **performance-optimized** version of Joker with:

- **IR bytecode interpreter** (26 opcodes) for hot loops and functions
- **WASM/wazero native compilation** for pure numeric workloads
- **Generic tail-call optimization** (runtime trampoline + parse-time rewriting)
- **Transient vector support** for in-place mutation in loops
- **Evaluator fast paths** for arithmetic, comparisons, binding lookup
- **Compiled function dispatch** within the IR (nested `irExec`)

## Performance

Compared to the original Joker tree-walker:

| Benchmark | Original | Optimized (IR) | Optimized (WASM) |
|-----------|----------|----------------|------------------|
| Arithmetic loop | 189.8 ms | 28 ms (7×) | **0.36 ms (527×)** |
| Recursive fib | 546 ms | 65.6 ms (8.3×) | — |
| Word frequency | 280 ms | 7 ms (40×) | — |

Compared to other interpreters on the CLBG benchmark suite:

| vs. Engine | Joker wins | Within 4× | Over 4× |
|------------|-----------|-----------|---------|
| Goja (gi's JS engine) | 3 benchmarks | 4 benchmarks | 4 benchmarks |
| Bun/JSC (JIT) | 0 (WASM matches on arith) | — | — |
| Python 3.13 | 0 | — | — |

Joker **beats Goja** on: pidigits (1.3×), regex-redux (1.2×), recursive fib (1.2×).

The WASM backend **matches Bun/JSC** on pure integer arithmetic loops (0.36ms vs 0.38ms).

## Documentation

See [`docs/OPTIMIZATION_REPORT.md`](docs/OPTIMIZATION_REPORT.md) for the full technical report covering:

- All optimization phases and trade-offs
- Architecture diagram
- Complete file inventory
- Cross-language benchmark results
- Suggested git history for replaying the work
- Future directions

## Building

```bash
go test ./core          # run tests
go test ./core -bench . # run benchmarks
```

## Benchmarks

```bash
# Full CLBG suite
go test ./core -run '^$' -bench 'BenchmarkCLBG|BenchmarkEval|BenchmarkWasm' -benchmem -benchtime=5x

# Cross-language comparison
python3 benchmarks/cross_lang_bench.py
bun benchmarks/cross_lang_bench.js

# Regenerate chart
go run ./benchmarks/generate_svg.go ./benchmarks
```

## Upstream

Based on [candid82/joker](https://github.com/candid82/joker) v1.7.1.

The original Joker README is preserved as [`ORIGINAL_README.md`](ORIGINAL_README.md).

## License

Same as upstream Joker (EPL-1.0).
