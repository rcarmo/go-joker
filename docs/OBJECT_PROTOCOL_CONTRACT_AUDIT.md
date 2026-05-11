# Object/protocol contract audit

Updated: 2026-05-11

## Purpose

This is a prerequisite audit for moving collections, reader, runtime, and evaluator code out of the root `core` package. The goal is to identify the contracts that must become explicit before concrete types can move into `core/internal/collections`, `core/internal/reader`, or `core/internal/runtime`.

Breaking internal package paths are acceptable. Compatibility wrappers are not a goal. The risk to avoid is import cycles and vague cross-package reach-through.

## Current object-model anchors

The root `core` package currently owns the central runtime object model:

- `Object`
- concrete scalar types (`Int`, `Double`, `Boolean`, `String`, `Char`, `Symbol`, `Keyword`, etc.)
- aggregate interfaces (`Seqable`, `Map`, `Set`, `Associative`, `Vec`, `Callable`, `CountedIndexed`, etc.)
- protocol and metadata behavior
- equality, hashing, comparison, printing, and object info/source metadata
- runtime singletons (`NIL`, booleans, EOF-like sentinels)
- constructors such as `MakeString`, `MakeKeyword`, collection builders, numeric builders

Because these are used by almost every subsystem, extracting collections or reader before this contract is explicit would create broad exports or cycles.

## Collections move prerequisites

Before moving concrete collection implementations, define or document:

1. Equality/hash contract for keys and values.
2. Seq contract (`Seqable`, empty seq representation, `First`/`Rest` behavior).
3. Persistent update contract (`Assoc`, `Conj`, `Without`, structural sharing expectations).
4. Transient mutation contract and ownership/lifetime rules.
5. Metadata propagation rules.
6. Printing/`ToString` contract.
7. Construction API the reader/evaluator/std packages should use.
8. Protocol dispatch surface required by moved concrete types.

Candidate collection package should own concrete data structures, not the whole object universe:

```text
core/internal/collections/
├── vector.go
├── map.go
├── set.go
├── seq.go
├── transient.go
└── hash.go
```

## Reader move prerequisites

Before moving reader/parser code, define or document:

1. Object construction interface for scalars and collections.
2. Tagged literal registration and lookup contract.
3. Source metadata/object info attachment contract.
4. Namespace/data-reader interaction boundaries.
5. Error reporting shape and line/column handling.
6. Conditional read feature flags.

Candidate reader package should not know evaluator internals:

```text
core/internal/reader/
├── lexer.go
├── reader.go
├── tagged.go
├── source_info.go
└── conditionals.go
```

## Runtime/evaluator move prerequisites

Before moving runtime/evaluator pieces, define or document:

1. Call dispatch contract for `Fn`, `Proc`, `Callable`, and IR-compiled functions.
2. Error/panic/ex-info contract.
3. Eval frame/source stack contract.
4. Dynamic var/thread binding contract.
5. Namespace/var deref and symbol resolution hooks.
6. Tracing/profiling hook points.
7. Concurrency/goroutine runtime ownership.

Candidate packages should avoid mutual imports with collections/reader:

```text
core/internal/runtime/
├── calls.go
├── frames.go
├── errors.go
├── goroutines.go
└── tracing_hooks.go

core/internal/eval/
├── eval.go
├── forms.go
├── tco.go
└── compiler_bridge.go
```

## Recommended next code moves

Safe moves before broad object extraction:

- Continue moving pure helpers with no `core.Object` dependency into `core/internal/ir` and `core/internal/wasm`.
- Add tests for extracted helpers before moving callers.
- Keep root-core adapter functions temporary only while their surrounding subsystem is still coupled.

Do not yet move:

- concrete collections
- reader/parser
- evaluator/forms
- runtime errors/frames

until the above contracts are made concrete in code or a narrower design doc.

## Checklist status

- [x] Identify object/protocol contracts that block collection moves.
- [x] Identify reader construction/tagged literal contracts.
- [x] Identify runtime/evaluator call/error/frame contracts.
- [x] Confirm broad R5 moves should continue waiting on explicit contracts.
