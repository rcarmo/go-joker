# Object/protocol contract audit

Updated: 2026-05-18

## Purpose

This is a prerequisite audit for moving collections, reader, runtime, and evaluator code out of the root `core` package. The goal is to identify the contracts that must become explicit before concrete types can move into `core/collections`, `core/reader`, or `core/runtime`.

Breaking internal package paths are acceptable. Compatibility wrappers are not a goal. The risk to avoid is import cycles and vague cross-package reach-through.

## Current object-model anchors

The canonical object/protocol surface has moved substantially out of root `core` and into `core/types`. Root `core` no longer owns or aliases `Object`; callers now use `coretypes.Object` directly. `core/types` currently owns:

- the canonical `Object` protocol, including `GetType() *Type`;
- source metadata (`Position`, `ObjectInfo`, `InfoHolder`);
- type descriptors/registry/builders and type metadata labels;
- scalar and reader values: `Int`, `Double`, `Boolean`, `Char`, `String`, `Time`, `Regex`, `Comment`;
- big numeric values and numeric mechanics: `BigInt`, `BigFloat`, `Ratio`, `Number`, `Precision`, `Ops`, promotion helpers, numeric compare/min/max/category helpers, rune/native-int bounds, and `MakeMathBigInt*` helpers;
- simple runtime values such as `RecurBindings` and `Delay`;
- protocol contracts that are now package-independent: `Equality`, `Counted`, `Named`, `Printer`, `Pprinter`, `Formatter`, `Native`, `Pending`, `Comparable`, `Comparator`, `Sequential`, `StringReader`, `Callable`, `Conjable`, `Deref`, `CountedIndexed`, `Indexed`, `Stack`, `Gettable`, `Seq`, `Seqable`, `Associative`, `Reversible`, `Collection`, `KVReduce`, `Reduce`, `Error`, `Map`, `Set`, `Vec`, `Meta`, and `Ref`;
- shared collection helpers/contracts such as `MapIterator`, `Pair`, `EmptyMapIterator`, `SafeMerge`, `EmptyMapIteratorInstance`, and iterator error construction;
- assertion helpers for `coretypes.*` and stdlib I/O return types, using root-installed error callbacks so root `EvalError` behavior is preserved;
- small generic helpers such as `NamedSlice`, `ComparatorSlice`, `NewHash32`, `WithInfo`, and `RootObject`.

Root `core` still owns higher-level runtime/object systems that carry root-only concrete types or mutable namespace/evaluator state:

- `Nil`, `Var`, `Proc`, `Fn`, `ExInfo`, and `Atom`;
- concrete collection implementations (`ArrayMap`, `HashMap`, `MapSet`, vectors, seq/list/chunked/transient/sorted families) that still depend on root construction, metadata propagation, proc/sorted helpers, or concrete implementation return types;
- namespace/bootstrap/proc systems and generated runtime mutation payloads;
- evaluator/parser/runtime/executor files that still require root `Fn`, `Var`, `Expr`, `LocalEnv`, namespace, and frame state.

This means future extraction work should treat `core/types` as the durable object/protocol package and focus on the remaining root-owned concrete systems rather than adding new root protocol aliases.

## Collections move prerequisites

Recent audit work completed `PersistentVector` object semantics in root `core`: it now uses the standard counted/indexed print, equality, hash, `At`, and `Seq` contracts and preserves info/meta. `core/object_protocol_contract_test.go` now exercises these contracts across `ArrayVector`, `Vector`, and `PersistentVector`; it also covers associative map behavior across `ArrayMap` and `HashMap` for lookup, persistent `Assoc`, equality, and hash consistency. Set contracts cover membership, call behavior, persistent `Disjoin`, equality/hash, and metadata preservation across persistent `Conj`/`Disjoin`. Transient contracts cover vector `CountedIndexed` behavior, vector/map mutable updates, persistent round-trips, string-key side-table persistence, and post-`persistent!` mutation panics. Seq contracts cover list, array, vector, cons, take, and filtering sequences for first/rest/count/nth/equality/hash/cons behavior and empty/negative-index edge cases. Sorted collection contracts cover sorted-map/sorted-set ordering, sorted-map-by/sorted-set-by comparator ordering, sorted metadata, lookup, and subseq/rsubseq ordering. Numeric object contracts now include native-int range-aware integer parsing, integer ratio promotion, and guarded `BigInt.Int()` conversion. The info/meta test deliberately checks metadata copy-on-write without assuming `WithInfo` is copy-on-write, because some generated `WithInfo` methods mutate existing objects. `make core-contract-check` runs that focused contract subset from the standard docs/check path. This is a useful template for future collection/object moves, but the broader boundary still needs explicit contracts before package extraction.

Before moving concrete collection implementations, define or document:

1. Equality/hash contract for keys and values.
2. Seq contract (`Seqable`, empty seq representation, `First`/`Rest` behavior).
3. Persistent update contract (`Assoc`, `Conj`, `Without`, structural sharing expectations).
4. Transient mutation contract and ownership/lifetime rules.
5. Metadata propagation rules.
6. Printing/`ToString` contract.
7. Construction API the reader/evaluator/std packages should use. **Done for current root production callers: `CollectionConstructionAdapter` provides the construction surface, and `construction_boundary_guard_test.go` rejects new direct constructor drift outside implementation/adapter files.**
8. Protocol dispatch surface required by moved concrete types.

Candidate collection package should own concrete data structures, not the whole object universe:

```text
core/collections/
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
core/reader/
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
core/runtime/
├── calls.go
├── frames.go
├── errors.go
├── goroutines.go
└── tracing_hooks.go

core/eval/ (future target; not reserved yet)
├── eval.go
├── forms.go
├── tco.go
└── compiler_bridge.go
```

## Recommended next code moves

Safe moves before broad object extraction:

- Continue moving pure helpers with no root `Fn`/`Var`/`Expr`/namespace dependency into `core/ir`, `core/wasm`, and `core/types`.
- Add tests for extracted helpers before moving callers.
- Keep root-core adapter functions temporary only while their surrounding subsystem is still coupled.
- Use the construction boundary guard before moving collection or reader files; a failing guard means new direct root construction has drifted in and must be routed through `collectionConstruction` or `readerConstruction` first.

Do not yet move wholesale:

- concrete collections whose methods still depend on root construction helpers, sorted/proc coupling, metadata propagation, or concrete implementation details;
- reader/parser orchestration with namespace/tagged-literal/evaluator side effects;
- evaluator/forms/runtime frames;
- namespace/proc/bootstrap systems.

The next safe moves should either continue protocol/value extraction into `core/types` (for remaining root-owned values that can avoid cycles) or move concrete collection families only after their root concrete return types are replaced by package-independent contracts.

## Checklist status

- [x] Identify object/protocol contracts that block collection moves.
- [x] Identify reader construction/tagged literal contracts.
- [x] Identify runtime/evaluator call/error/frame contracts.
- [x] Confirm broad R5 moves should continue waiting on explicit contracts.
