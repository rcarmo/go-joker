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
- CLBG-inspired benchmark suite, generated benchmark charts, and a repeat-run median benchmark harness.
- IR slot-allocation collision fixes for captured inner `let`/nested loop init expressions.
- Safe transient maps in IR loops, dropping `map-update-loop` from ~17.3ms to ~0.899ms.
- Initial IR/WASM explain diagnostics for hot-loop path inspection.
- First string/sequence IR fast path: unary `str` lowering plus string `nth` ASCII-prefix fast path inside IR.
- Cached string rune counts now accelerate repeated `count`/`subs` over stable strings while preserving Unicode semantics.
- IR equality now has primitive char/string fast paths before generic `Object.Equals`, allowing small text helper functions to stay compiled with less dispatch overhead.
- Added `irNthStringASCII`, a specialized opcode for `nth` over compile-time-known ASCII strings, reducing generic object dispatch in text loops.
- IR helper/self-call dispatch now uses stack-backed argument arrays for small arities.
- IR helper inlining now defaults to `auto`: text-oriented tiny helpers and tiny straight-line collection helpers are inlined. `JOKER_IR_INLINE=force` enables all tiny helpers for experiments; `off` disables it. This yields a large reverse-complement win without default-enabling numeric helper inlining.
- IR rejection diagnostics now report specific unsupported expression/callable/arity/binding/slot reasons instead of only a generic compile failure.
- Literal map expressions, including `{}`, now compile to IR constants, keeping more map-update loops on the lowered path.

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
- IR rejection specificity now covers unsupported expression types, unsupported callable shapes, unsupported core vars, wrong arity, binding/capture failures, slot collisions, too many captures, and dynamic map literals.
- Next: add source-location breadcrumbs for nested failures so benchmark diagnostics can point at the exact sub-form.
- Track counters for compiled/rejected/fallback cases.
- Add regression tests for nested `let`, nested `loop`, captures, closures, and helper calls.
- Keep slot allocation and capture handling safe; do not trade correctness for speed.

### B. String and sequence throughput

- Started: IR now lowers unary `str`, allowing loops that convert chars to strings to stay on the IR path; `irNth` has a Unicode-preserving ASCII-prefix fast path for strings; `irEq` has direct char/string equality; `irNthStringASCII` handles constant ASCII string indexing directly.
- Started optimizing `count`/`subs`: repeated string rune counts are cached, ASCII `subs` avoids `[]rune` conversion, and `stringSeq` implements `Count`.
- Continue optimizing `str`, `nth`, regex result handling, and sequence iteration.
- Add more ASCII/byte fast paths where semantics allow while preserving Unicode behavior.
- Reduce per-character object churn in CLBG-style string workloads.
- Consider internal builder-style optimizations for repeated concatenation patterns.

### C. Persistent data structure internals

- Continue reducing `ArrayMap`/`HashMap` update allocation.
- Improve small-map specialization and vector update/copy behavior.
- Refine escape analysis so more safe collection slots use transients.
- Preserve persistent semantics and single-owner transient safety.

### D. Function call overhead and inlining

- Started: IR helper/self-call dispatch now uses stack-backed argument arrays for small arities, reducing allocation in helper-heavy loops.
- Tiny helper inlining now has a static gate: default `auto` inlines text helpers (string/char literals or `str` usage) and tiny straight-line collection helpers (`nth`/`get`/`assoc`/`conj`/`count`/`first`), while `force` enables all tiny helpers for experiments.
- Median probes: reverse-complement remains much faster in auto mode; a synthetic collection-helper loop improves from ~1.49ms to ~0.29ms, while numeric helper loops remain protected from default inlining.
- Next: add source breadcrumbs for inlining decisions and keep broadening only when the median harness shows a win.
- Continue reducing frame/env allocation for simple calls.
- Cache compiled helper functions aggressively.
- Avoid tree-walker fallback for hot helper-call patterns.

## Medium-priority WASM workstreams

### E. Multi-function WASM modules

- Started: a one-helper WASM module prototype can emit a caller plus one captured helper function and lower `irCallSlot` to a direct WASM `call`.
- The prototype is covered by internal tests and has a conservative strategy gate: default `auto` permits integer one-helper modules, while float helper modules require `JOKER_WASM_MULTIFN=force` for probing.
- Helper functions with compiler-local slots now get proper WASM locals instead of being modeled as extra parameters.
- Next: replace the static float-off rule with a measured cost model so float helper modules are used only when they beat standalone helper WASM/IR dispatch.
- Define a broader safe capture/local ABI before enabling mandelbrot/pixel-style workloads by default.

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
- Run targeted benchmarks before/after; use `benchmarks/run_benchmarks.py` for median/stdev summaries when judging optimization decisions.
- Add regression tests for compiler/runtime correctness.
- Update `benchmarks/benchmark-history.json` and regenerate SVGs when benchmark numbers change materially.
- Keep CLBG-style results documented as pipeline stress tests, not broad real-world claims.
