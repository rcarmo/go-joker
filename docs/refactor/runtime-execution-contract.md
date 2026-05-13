# Runtime execution metadata contract

Updated: 2026-05-13

## Purpose

This note defines the contract that must exist before moving IR executors, escape analysis, evaluator/runtime frames, or collection implementations out of root `core`.

The neutral `core/internal/ir.Program` now owns bytecode/slot/analysis shape. Root `core.IRProgram` still owns executable metadata because it depends on runtime objects, environments, call dispatch, and error behavior. That split is intentional until the runtime contract below is made concrete.

## Root executable metadata that must remain in `core` for now

`core.IRProgram` still legitimately owns:

- `[]Object` constants and capture values
- `bindingKey` capture keys
- `*EscapeInfo` and safe-mutable/string-builder slot metadata
- typed/boxed execution failure caches
- native helper closures
- `*FnExpr` references for `irMakeFn`
- `traceName`
- arity/variadic executable envelopes

These fields are not pure IR shape. Moving them into `core/internal/ir` would leak root runtime internals into the IR model.

## Runtime/channel concurrency contract

The root channel runtime remains in `core` until runtime boundaries are explicit. Its current contract is:

- `Channel.Close()` is idempotent and safe under concurrent callers.
- `Channel.IsClosed()` is the only supported closed-state accessor; callers must not read internal fields directly.
- Sending after close returns false rather than panicking.
- Receiving from a closed channel returns `NIL` with `ChannelReceiveClosed`.

`channel_contract_test.go` guards the concurrent close/idempotency behavior. This matters for future runtime extraction because async helpers, `alts!`, and core send/receive procs must all share the same close-state semantics.

## Required execution boundary

Before moving executors or escape analysis, define an execution/runtime interface that covers:

```go
type RuntimeObjects interface {
    Nil() Object
    Bool(bool) Object
    MakeInt(int) Object
    MakeDouble(float64) Object
    ToBool(Object) bool
}

type RuntimeCollections interface {
    Count(Object) (int, bool)
    Nth(Object, Object) Object
    Assoc(Object, Object, Object) Object
    Conj(Object, Object) Object
    First(Object) Object
    ToTransient(Object) Object
    AssocBang(Object, Object, Object) Object
    PersistentBang(Object) Object
}

type RuntimeCalls interface {
    Call(Object, []Object) Object
    CallSelf(*IRProgram, []Object) Object
    MakeFn(*FnExpr, []Object) Object
}

type RuntimeErrors interface {
    Errorf(format string, args ...any) Object
    Panic(Object)
}
```

This is a sketch, not an implementation API. The important boundary is ownership: executors can move only when they can call object/collection/call/error operations through a narrow runtime surface rather than importing all of root `core`.

## Migration sequence

1. Keep `core/internal/ir.Program` as the neutral bytecode/shape model.
2. Keep root `core.IRProgram` as the executable envelope while executor metadata is root-coupled.
3. Add focused tests for execution metadata behavior before moving code:
   - constants/captures (**started: `ir_execution_metadata_contract_test.go` covers constants, slots, captures, escape metadata, and neutral-model analysis handoff**)
   - `irMakeFn` (**started: `ir_makefn_contract_test.go` covers FnExpr retention and current-slot capture semantics**)
   - native helper fallback/failure caches (**started: `ir_failure_cache_contract_test.go` covers compile-failure sentinels and native-helper eligibility caching**)
   - escape-analysis safe mutation slots (**started: `escape_analysis_contract_test.go` covers call-argument unsafety and string-builder slot classification**)
   - typed/boxed failure flags
   - WASM/native native-int conversion boundaries (**started: `wasm_host_contract_test.go` covers raw integer result promotion, host argument promotion, and index rejection**)
4. Introduce a small runtime execution adapter in root `core`. **Started: `RuntimeExecutionAdapter` now codifies root-owned error creation/throwing, capture-slot installation, nested-call argument/capture preparation, typed env-capture installation, `FnExpr`/slot capture construction, and typed/boxed execution failure flags; boxed and typed executors use it for `irThrow`, capture prefill, `irMakeFn`, nested-call call-slot preparation, typed capture installation, and failure gating.**
5. Move escape-analysis helpers only after they depend on neutral model plus explicit runtime facts.
6. Move boxed executor, then typed/nanbox executor, only after call/error/frame contracts are explicit.

## Non-goals

- Do not export `Object`, `FnExpr`, `bindingKey`, or frame internals solely to move files.
- Do not add compatibility wrappers around old paths.
- Do not move collections/reader/evaluator in the same step as executor migration.
- Do not collapse generated bootstrap migration into executor migration.

## Current status

- Neutral IR model: started and guarded by `core/internal/ir` tests.
- Diagnostics/export/WASM/native helper readers: migrated to the neutral model where appropriate.
- Runtime/execution-envelope tests, including WASM/native integer conversion, stable IR function-cache keys, and `RuntimeExecutionAdapter` error/function/capture/failure-flag contracts: gated by `make runtime-contract-check`, which is run by `make docs-check`.
- Executors and escape analysis: intentionally root-bound pending this runtime execution contract becoming code.
