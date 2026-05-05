# go-joker

![icon](docs/icon-256.png)

An optimized fork of [Joker](https://github.com/candid82/joker) (Clojure-like Lisp interpreter) for inclusion in [gi](https://github.com/rcarmo/gi), a self-hosted coding agent.

## Performance

### Cross-language benchmark matrix

![benchmark matrix](benchmarks/benchmark-cross-language.svg)

### vs. original Joker

![improvements](benchmarks/benchmark-improvements.svg)

### Highlights

| What | Result |
|------|--------|
| **N-body** | **~10.8 ms** — IR scope save/restore fix, 4.5× faster than before |
| **Mandelbrot** | **~9.7 ms** — WASM/IR typed path, 17× faster than original |
| **Binary trees** | **~85 ms** — self-recursive IR + transient vectors |
| **Cursor JSON parse** | **~99 µs** — 3.5× faster than nth-based, StringCursor IR opcodes |
| **Arithmetic loop** | **~0.24 ms** — matches Bun/JSC-class speed via WASM |
| **Joker beats Python on** | 6/13 CLBG benchmarks |
| **Joker beats Goja on** | 11/13 CLBG benchmarks |

## What's different from upstream Joker

### IR bytecode interpreter (typed + boxed paths)
Hot loops and functions compile to a flat bytecode. Eligible primitive/string loops now run on a typed IR value stack, while collection-heavy or unsupported cases fall back to the boxed IR interpreter and then to the tree-walker.

### WASM/wazero native compilation
Pure numeric loops compile further to WASM bytecode and execute via [wazero](https://github.com/tetratelabs/wazero)'s native code compiler. This achieves JIT-level performance (matching Bun/JSC) with zero CGo dependencies.

### Generic tail-call optimization
Self-recursive functions in tail position are automatically rewritten to `recur` at parse time, eliminating stack growth. A runtime trampoline handles cases the rewriter can't catch.

### Transient vectors and maps
Loops that update non-escaping vectors or maps via `assoc` automatically use in-place mutation (Clojure-style transients), eliminating persistent copy/update overhead while preserving persistent results at loop return.

### StringCursor native type
A zero-alloc O(1) string iterator with IR opcodes (`irCursorChar`, `irCursorNext`, `irCursorDone`). Cursor-based parsers run 3-3.5× faster than equivalent index-based code by eliminating per-character nth scanning and position arithmetic.

### Evaluator fast paths
Numeric operations, binding resolution, and function dispatch all have type-specialized fast paths that avoid the generic Joker evaluation machinery.

## Architecture

```
Joker Source → Reader + Parser → AST
                                  ↓
                           tco_rewrite (parse-time tail-call → recur)
                                  ↓
                              Eval() type switch
                                  ↓
                    ┌─────────────┼─────────────┐
                    ↓             ↓             ↓
              WASM/wazero    IR bytecode    Tree-walker
              (native)       (irExec)      (evalLoop)
              0.32ms ⚡       28ms           190ms
                    ↑             ↑
                    └──fallback───┘
```

- **WASM path**: pure integer/float loops → wazero JIT → native code
- **Typed IR path**: primitive/string loops → tagged values, low allocation
- **Boxed IR path**: loops with collections, fn calls, let bindings → bytecode interpreter
- **Tree-walker**: everything else (macros, special forms, I/O)
- **gi bridge**: hooks, tools, state access — callable from IR via `irCallSlot`

## Building & testing

```bash
go test ./core              # run all tests
go test ./core -bench .     # run all benchmarks
```

## Benchmarks

> **Note:** The CLBG programs were chosen as a starting point for optimizing the IR and WASM compilation pipeline, not because they represent realistic workloads. They stress specific interpreter bottlenecks (arithmetic loops, recursion, allocation, string processing) that guided the optimization work. Real-world gi scripts will have different profiles — the gains here prove the execution machinery works, not that every Joker program runs 500× faster.

```bash
# Full CLBG suite + micro benchmarks
go test ./core -run '^$' -bench 'BenchmarkCLBG|BenchmarkEval|BenchmarkWasm' -benchmem -benchtime=5x

# Cross-language comparison
python3 benchmarks/cross_lang_bench.py
bun benchmarks/cross_lang_bench.js

# Regenerate charts
go run ./benchmarks/generate_svg.go ./benchmarks
```

## Documentation

- [`docs/OPTIMIZATION_REPORT.md`](docs/OPTIMIZATION_REPORT.md) — full technical report (phases, trade-offs, outcomes, suggested git history)
- [`benchmarks/README.md`](benchmarks/README.md) — benchmark data and chart regeneration
- [`PERFORMANCE_PLAN.md`](PERFORMANCE_PLAN.md) — optimization roadmap and milestones

## Upstream

Based on [candid82/joker](https://github.com/candid82/joker) v1.7.1.  
Original README preserved as [`ORIGINAL_README.md`](ORIGINAL_README.md).

## License

Same as upstream Joker (EPL-1.0).
