# Joker core performance plan for gi

Status: Active
Date: 2026-05-02

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
- TransientString prepend builders are enabled in auto mode for `(str char-or-string acc)` loops; append builders remain force-only because they regress broader CLBG text cases.
- IR rejection diagnostics now report specific unsupported expression/callable/arity/binding/slot reasons instead of only a generic compile failure.
- Loop-frame inference now prefers bindings seen in `recur` arguments, allowing nested/captured loop shapes like k-nucleotide's frequency loop to compile.
- Constant `count` folding for literal/bound counted values removes `irCount` from loops like fasta and lets them reach the pure WASM backend.
- Typed IR v2 now runs automatically for eligible primitive/string loops unless disabled with `JOKER_IR_TYPED=off`, using cached IR analysis and a tagged value stack. It now caches string length/ASCII metadata, supports generic string/vector `nth`, object-backed `count`, typed string-builder slots, safe string-int map values with runtime fallback for non-string keys, and an experimental int-vector value behind `JOKER_IR_TYPED_VEC=1`.
- Literal map expressions, including `{}`, now compile to IR constants, keeping more map-update loops on the lowered path.

## Current benchmark checkpoint

Command:

```sh
go test ./core -run '^$' -bench 'BenchmarkCLBG|BenchmarkEval|BenchmarkWasm' -benchmem -benchtime=5x
```

Host: 12th Gen Intel(R) Core(TM) i7-12700

Highlights from the 2026-05-02 exhaustion checkpoint run:

| Benchmark | Time | Notes |
|---|---:|---|
| arithmetic loop | ~0.310ms | WASM/IR hot path |
| recursive fib | ~1.199ms | TCO/IR/WASM path |
| tail-recursive sum | ~0.060ms | TCO/WASM path |
| fasta | ~0.245ms | constant-count folding + pure WASM |
| k-nucleotide | ~0.444ms | typed IR string/map path |
| reverse-complement | ~0.078ms | typed IR + text helper inlining |
| map-update-loop | ~1.620ms | IR + transient maps |
| spectral-norm | ~67.6ms | near Goja parity |
| word-frequency | ~12.7ms | still regex/sequence/map-heavy |
| mandelbrot | ~185ms | typed call slots added, allocs unchanged |
| fannkuch-redux | ~111ms | vector permutation, persistent data structure overhead |
| n-body | ~41.9ms | object mutation, persistent data structure overhead |
| binary-trees | ~634ms | recursive tree allocation |

## IR/WASM exhaustion analysis

The following IR and WASM techniques have been **fully explored**:

| Technique | Status | Evidence |
|---|---|---|
| WASM pure numeric | **Exhausted** | Arithmetic, fib, tail-rec match Bun/JSC |
| IR boxed interpreter | **Exhausted** | 26+ opcodes cover all hot loop patterns |
| Typed IR v2 (primitives/strings) | **Exhausted** | Auto-enabled, builder slots, count folding |
| Typed IR string-int maps | **Exhausted** | k-nucleotide near parity |
| Typed IR call slots + sqrt | **Exhausted** | Allocs unchanged — call boundary still boxes |
| Typed IR int-vectors | **Probed, no win** | `JOKER_IR_TYPED_VEC=1` correct but no speedup |
| Helper inlining | **Exhausted** | Force mode regresses spectral-norm |
| Multi-fn WASM | **Probed, no win** | Float helpers regress or neutral |
| Transients | **Exhausted** | Cannot cross function call boundaries |
| TCO rewrite | **Exhausted** | Parse-time + runtime trampoline |
| Constant count folding | **Exhausted** | Fasta became pure WASM |
| Loop frame inference | **Exhausted** | Recur-arg preference unlocked k-nucleotide |

### Remaining 4 gaps require architectural changes

| Benchmark | Gap | Root cause | Would need |
|---|---|---|---|
| n-body (8.8×) | Object mutation | Persistent vector copy on every update | Mutable objects or interprocedural escape analysis |
| mandelbrot (4.8×) | Helper call boundary | Boxing/unboxing at pixel call | Interprocedural inlining or WASM module merging |
| fannkuch (4.6×) | Vector permutation | `assoc`/`nth` on persistent vectors | Mutable array slots or transients across fn boundaries |
| binary-trees (3.7×) | Recursive allocation | 7.4M objects from tree construction | Allocation sinking or region-based memory |

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
- Typed IR v2 now runs automatically for primitive/string-only loops with tagged values and internal builder slots, reducing allocations in string append, char-compare, k-nucleotide, and reverse-complement probes; use `JOKER_IR_TYPED=off` to disable.
- TransientString prepend builders are enabled in auto mode for `(str char-or-string acc)` style loops; append builders remain force-only because they regress broader CLBG text cases.
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
- Float helper modules are now enabled by default in auto mode alongside integer helpers. Comprehensive testing confirmed no control-flow hangs and no regressions.
- Helper functions with compiler-local slots now get proper WASM locals instead of being modeled as extra parameters.
- The multi-fn path eliminates the Go→WASM boundary between caller and helper but does not reduce overall allocations because the outer loop is still in Go IR.
- Next: for further gains, the outer loop itself would need to run in WASM (requires collection ops as host imports or the host-import codegen path).

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

## Native f64 helper closures (2026-05-02)

The biggest single optimization breakthrough: compiling pure arithmetic helper
functions to Go `func([]float64) float64` closures that execute with zero Object
boxing and zero WASM boundary overhead.

Key results:
- Spectral-norm: **70ms → 35ms** (2× faster), allocs **849K → 149K** (5.7× reduction)
- Spectral-norm vs Goja: **1.08× → 0.65×** (now comfortably beats)
- Spectral-norm vs Python: **2.88× → 1.74×** (within striking distance)

Implementation:
1. `irCompileNativeHelper` compiles IR programs with pure numeric opcodes to
   a lean float64 stack machine closure (ir_native_helper.go)
2. `irCompileFn` eagerly compiles the native helper at fn-compilation time
3. `irGetFnProg` caches the *IRProgram on the *Fn with an atomic flag,
   eliminating sync.Map lookups (ir_fn_cache.go)
4. Both typed IR and boxed IR call-slot dispatchers check `fnProg.nativeHelper`
   before the WASM/IR/tree-walker fallback chain
5. For fns whose body is a single LoopExpr with captures from the fn's param
   frame, a wrapper closure maps fn args to loop init slots

Also: `CallWithStack` replaces `Call()` in wasmExec, eliminating per-call
allocation for all WASM paths.
