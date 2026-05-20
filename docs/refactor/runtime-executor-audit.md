# Runtime/executor extraction audit

Updated: 2026-05-20

## Purpose

This audit records the remaining root-owned executor/runtime families before any real move into `core/runtime`. The goal is to keep package moves honest: executor code may move only after dependencies are explicit, acyclic, and routed through `RuntimeExecutionAdapter` or similarly narrow contracts.

## Root executor families

### Boxed executor

Files:

- boxed executor now coalesced into `runtime_kernel.go`
- IR cache/compiler/executor cluster now coalesced into `runtime_kernel.go` (also owns the former tiny `fn_ir_dispatch.go` dispatch bridge)
- `fn_ir_compile.go`

Current state:

- Uses `runtimeExec` for executable envelope access, constants, bytecode, nested calls, Fn construction, call dispatch, collection/string/cursor helpers, native helper dispatch, and failure gating.
- Still root-bound through `Object`, `Callable`, `Fn`, `FnExpr`, `LocalEnv`, `Expr`, `Seq`, root errors, opcode-local value semantics, and tree-walker fallback.

Safe next steps:

- Keep replacing direct root-object reach-through with adapter methods when the method has a stable semantic boundary.
- Do not move boxed executor loops until frame/result/error construction no longer requires root internals.

### Typed executor and nanbox executor

Files:

- `typed_exec.go`
- `typed_exec_inline.go`
- `typed_exec_nanbox.go`
- `typed_values.go`
- `typed_value_accessors.go`

Current state:

- Uses adapter seams for many Fn/program/call/failure/collection operations.
- Still root-bound through `irValue`, typed value representation, object boxing/unboxing, root numeric/string semantics, fast-path object predicates, and failure fallback behavior.

Safe next steps:

- Keep boxed executor boundary stable first.
- Add contract tests before moving typed value representation; it is runtime representation, not neutral IR.

### Escape analysis and inline rewrites

Files:

- `escape_analysis.go`
- `inline_rewrites.go`
- `loop_frame_detect.go`
- `loop_native_helpers.go`

Current state:

- Escape facts are tied to root `Expr`, `LocalEnv`, binding keys, native helper eligibility, safe mutable slots, and string-builder slots.
- Some facts are already covered by contract tests, but movement would still require an explicit expression/runtime-facts interface.

Safe next steps:

- Keep escape analysis in root until it consumes neutral IR/expression facts rather than root AST/runtime structures.
- Move only leaf helper predicates if they do not mention root `Expr`, `LocalEnv`, `FnExpr`, `Var`, namespace/frame state, or binding keys.

### WASM/native helper runtime

Files:

- WASM compile/lowering glue now coalesced into `runtime_kernel.go`
- `wasm_compile_host.go`
- `wasm_exec_runtime.go`
- `wasm_host_funcs.go`
- `wasm_helper_backend.go`
- `wasm_mem_nth_backend.go`
- native recursive specialization now coalesced into `runtime_kernel.go`
- `loop_wasm_diagnostics.go`

Current state:

- Neutral eligibility/lowering reads increasingly consume `core/ir.Program` and `core/wasm` helpers.
- Runtime execution remains root-bound through object promotion, host argument/result conversion, Fn/native-helper state, WASM failure fallback, mem-nth retry gating, and std/root object semantics.

Safe next steps:

- Keep `core/wasm` for neutral helpers only.
- Do not move host/runtime execution until typed/object conversion is behind a stable runtime adapter.

### Channel/pending concurrency mechanics

Files:

- `core/runtime/channel.go`
- `core/runtime/pending.go`
- `core/runtime/agent.go`
- root `runtime_kernel.go` for concurrency/core.async registration glue

Current state:

- Generic close-state, send, and receive mechanics now live in `core/runtime.Channel[T]` with package-local tests.
- Generic pending-value mechanics now live in `core/runtime.Future[T,E]` and `core/runtime.Promise[T]` with package-local tests.
- `core/runtime.ObjectChannel` wraps `runtime.Channel[FutureResult]`, preserving `Object`, `Error`, type/hash, proc registration, and `alts!` reflection integration while removing the old root `core.Channel` wrapper file.
- `core/runtime.ObjectFuture` and `core/runtime.ObjectPromise` wrap runtime pending primitives, preserving Object/Error/proc semantics while moving blocking/realization/deliver-once mechanics out of root; package-local tests cover deref, realized state, error propagation, and deliver-once behavior. `Delay` now owns a local promise primitive in `core/types`, avoiding a `core/types` → `core/runtime` import cycle.
- `core/runtime.Agent` owns the agent object wrapper, queue, worker loop, and error state; root proc registration uses exported `Send`, `Await`, and `Error` methods plus goroutine registration hooks, and package-local tests cover send/await/error behavior.
- `core/runtime.Atom` owns atom value/meta/object/ref behavior and exposes `Swap`, `Reset`, and `CompareAndSet` methods; root atom proc/validator/watch glue no longer reaches into atom mutex/value fields directly.
- Core send/receive/go/future/promise/delay/agent/atom procs now call runtime-owned wrappers instead of manipulating raw done channels, queues, or value slots directly.

Safe next steps:

- Keep `alts!` root-bound until Object/vector/result construction seams are explicit; it still performs reflection-select setup and vector result construction in root proc glue.
- Avoid exposing raw runtime channels except for reflection-select integration that cannot currently be moved without root object/vector/result construction seams.
- Keep concurrency/core.async proc/env registration glue in root `runtime_kernel.go` until `Proc`, `Fn`, `GLOBAL_ENV`, `NIL`, and call helpers have an acyclic runtime boundary.

### Environment, frames, and parse/eval handoff

Files:

- environment/namespace runtime glue now coalesced into `runtime_kernel.go`
- fast-startup `ReferCoreToUser` hook now coalesced into `a_generated_bootstrap_payloads.go`
- gen-code environment construction now coalesced into `bootstrap_gen_code.go`
- parser/eval-facing parts now coalesced into `runtime_kernel.go`

Current state:

- These own root evaluation frames, locals, dynamic vars, closures, binding keys, and namespace/eval semantics.
- They are not leaf runtime helpers and should remain root until a much narrower frame contract exists.

## Current blockers for moving executor loops

- `Object` and concrete object conversions are root-owned.
- `Fn`, `FnExpr`, `Expr`, `LocalEnv`, `bindingKey`, `Var`, and namespace/eval state are root-owned.
- `irValue` and typed/nanbox representation are still coupled to root object conversion and fallback semantics.
- Error creation/throwing, tree-walker fallback, and failure caches are runtime behavior, not neutral IR.
- Collection/string/cursor operations still depend on root protocols and object types despite adapter seams.

## Next safe runtime work

1. Continue adding narrow `RuntimeExecutionAdapter` methods only when an executor still reaches into root internals directly.
2. Add/extend contract tests for any newly adapterized behavior before moving files.
3. Move only root-independent runtime leaf helpers first; do not move executor loops or escape analysis by force.
4. Keep `docs/refactor/runtime-execution-contract.md` synchronized with adapter coverage and blockers.


## 2026-05-18 core/types cleanup note

The root object/protocol split progressed: shared contracts such as `Map`, `Meta`, `Set`, `Vec`, `Ref`, assertion helpers for moved types/std I/O, and generic `WithInfo`/`RootObject` helpers now live in `core/types`, and root compatibility aliases were removed. This reduces protocol-level blockers but does not by itself move concrete reader/evaluator/runtime/collection implementations; those packages should continue to rely on explicit adapters and avoid importing root-only concrete state.
