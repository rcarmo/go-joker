# Joker core performance plan for gi

Status: Active
Date: 2026-04-30

This plan tracks the work to make core Joker faster for gi scripting. Additional namespaces and public convenience wrappers stay on the roadmap, but the current priority is the core evaluator/IR/runtime.

## Goals

1. Keep Joker language semantics compatible with upstream.
2. Make the hot subset used by gi scripts fast: loops, recursion, numeric ops, text/sequence processing, map/vector updates, and helper calls.
3. Prefer all-Go, embeddable optimizations: evaluator fast paths, IR, transients, and wazero WASM where it is safe.
4. Keep every optimization measurable, testable, and bisectable.

## Completed major work

- Evaluator fast paths for hot numeric procs and binding lookup.
- Stack-backed argument arrays for common call/recur arities.
- Reduced loop/frame allocation and removed hot-path `defer` use.
- Direct `ArraySeq` indexing for `SeqNth`/`SeqTryNth`.
- Lowered IR stack machine for hot loops and helper functions.
- Tail-call optimization and parse-time tail-call-to-`recur` rewriting.
- WASM/wazero backend for pure numeric loops and self-recursive functions.
- WASM f64 support and compiled helper dispatch for spectral-norm style code.
- Escape analysis for safe in-place collection mutation.
- Internal and explicit transient vector/map support.
- WASM linear-memory f64/i64 arrays as an experimental foundation.
- CLBG-inspired benchmark suite and generated benchmark charts.
- IR slot-allocation collision fixes for captured inner `let`/nested loop init expressions.
- Safe transient maps in IR loops, dropping `map-update-loop` from ~17.3ms to ~0.899ms.
- Initial IR/WASM explain diagnostics for hot-loop path inspection.

## Current benchmark checkpoint

Command:

```sh
go test ./core -run '^$' -bench 'BenchmarkCLBG|BenchmarkEval|BenchmarkWasm' -benchmem -benchtime=5x
```

Host: 12th Gen Intel(R) Core(TM) i7-12700

Highlights from the 2026-04-30 run:

| Benchmark | Time | Notes |
|---|---:|---|
| arithmetic loop | ~0.264ms | WASM/IR hot path |
| recursive fib | ~0.957ms | TCO/IR/WASM path |
| tail-recursive sum | ~0.060ms | TCO/WASM path |
| map-update-loop | ~0.899ms | IR + transient maps |
| word-frequency | ~7.71ms | IR + maps, still sequence/text-heavy |
| k-nucleotide | ~0.927ms | improved, still string-heavy |
| fannkuch-redux | ~82.2ms | collection-heavy |
| mandelbrot | ~128ms | helper-call/WASM multi-function gap |

## High-priority workstreams

### A. IR coverage and diagnostics

- Broaden IR coverage before adding new public APIs.
- Initial `IR explain`/`WASM explain` helpers now identify compiled loops, slot/capture/op counts, pure-WASM eligibility, host-import requirements, string-op rejection, helper-call/multi-function gaps, and no-loop cases.
- Next: improve IR rejection specificity for unsupported AST forms instead of returning only a generic compile rejection.
- Track counters for compiled/rejected/fallback cases.
- Add regression tests for nested `let`, nested `loop`, captures, closures, and helper calls.
- Keep slot allocation and capture handling safe; do not trade correctness for speed.

### B. String and sequence throughput

- Optimize `str`, `nth`, `subs`, `count`, regex result handling, and sequence iteration.
- Add ASCII/byte fast paths where semantics allow while preserving Unicode behavior.
- Reduce per-character object churn in CLBG-style string workloads.
- Consider internal builder-style optimizations for repeated concatenation patterns.

### C. Persistent data structure internals

- Continue reducing `ArrayMap`/`HashMap` update allocation.
- Improve small-map specialization and vector update/copy behavior.
- Refine escape analysis so more safe collection slots use transients.
- Preserve persistent semantics and single-owner transient safety.

### D. Function call overhead and inlining

- Revisit tiny local function inlining now that slot-collision regressions are covered.
- Reduce frame/env allocation for simple calls.
- Cache compiled helper functions aggressively.
- Avoid tree-walker fallback for hot helper-call patterns.

## Medium-priority WASM workstreams

### E. Multi-function WASM modules

- Emit multiple functions in one WASM module.
- Support direct WASM-to-WASM calls for eligible helper functions.
- Define a safe capture/local ABI.
- Target mandelbrot/pixel-style helper-heavy numeric workloads.

### F. WASM host imports for collections

- Keep disabled until the handle ABI and structured control-flow lowering are fully validated.
- Avoid recursive imported-WASM collection functions until multi-function support exists.
- Add comparison tests against IR/tree-walker before enabling by default.

### G. WASM linear memory auto-use

- Existing f64/i64 arrays are explicit and experimental.
- Add IR opcodes for typed load/store before automatic use.
- Use WASM memory operations directly, not host-side `Memory.Read`/`Write`, for real speedups.

## Roadmap only

- Public `core.joke` wrappers for transients.
- Additional namespaces and libraries.
- Broader bridge/API exposure beyond the core runtime.

## Validation rules

For every optimization:

- Run `go test ./core`.
- Run targeted benchmarks before/after.
- Add regression tests for compiler/runtime correctness.
- Update `benchmarks/benchmark-history.json` and regenerate SVGs when benchmark numbers change materially.
- Keep CLBG-style results documented as pipeline stress tests, not broad real-world claims.
