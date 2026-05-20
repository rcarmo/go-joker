# Object/protocol contract audit

Updated: 2026-05-20

## Purpose

This is a prerequisite audit for moving collections, reader, runtime, and evaluator code out of the root `core` package. The goal is to identify the contracts that must become explicit before concrete types can move into `core/types/collections`, `core/reader`, or `core/runtime`.

Breaking internal package paths are acceptable. Compatibility wrappers are not a goal. The risk to avoid is import cycles and vague cross-package reach-through.

## Current object-model anchors

The canonical object/protocol surface has moved substantially out of root `core` and into `core/types`. Root `core` no longer owns or aliases `Object`; callers now use `coretypes.Object` directly. `core/types` currently owns:

- the canonical `Object` protocol, including `GetType() *Type`;
- source metadata (`Position`, `ObjectInfo`, `InfoHolder`);
- type descriptors/registry/builders and type metadata labels;
- scalar and reader values: `Int`, `Double`, `Boolean`, `Char`, `String`, `Time`, `Regex`, `Comment`;
- big numeric values and numeric mechanics: `BigInt`, `BigFloat`, `Ratio`, `Number`, `Precision`, `Ops`, promotion helpers, numeric compare/min/max/category helpers, rune/native-int bounds, and `MakeMathBigInt*` helpers;
- simple runtime values such as `RecurBindings`, `Delay`, and transient collection implementations (`TransientVector`, `TransientMap`);
- protocol contracts that are now package-independent: `Equality`, `Counted`, `Named`, `Printer`, `Pprinter`, `Formatter`, `Native`, `Pending`, `Comparable`, `Comparator`, `Sequential`, `StringReader`, `Callable`, `Conjable`, `Deref`, `CountedIndexed`, `Indexed`, `Stack`, `Gettable`, `Seq`, `Seqable`, `Associative`, `Reversible`, `Collection`, `KVReduce`, `Reduce`, `Error`, `Map`, `Set`, `Vec`, `Meta`, and `Ref`;
- shared collection helpers/contracts such as `MapIterator`, `Pair`, `EmptyMapIterator`, `SafeMerge`, `EmptyMapIteratorInstance`, and iterator error construction;
- assertion helpers for `coretypes.*` and stdlib I/O return types, using root-installed error callbacks so root `EvalError` behavior is preserved;
- small generic helpers such as `NamedSlice`, `ComparatorSlice`, `NewHash32`, `WithInfo`, and `RootObject`.

Root `core` still owns higher-level runtime/object systems that carry root-only concrete types or mutable namespace/evaluator state:

- `Nil`, `Var`, `Proc`, `Fn`, and `ExInfo`;
- sorted collections, transients, records, protocol dispatch, unchecked arithmetic procs, hierarchy/protocol/public-form installers, and collection-adjacent runtime/proc helpers that still depend on root construction, metadata propagation, proc/sorted helpers, or evaluator/runtime behavior; their recent arity/error cleanup routes through `core/types` runtime hooks where practical;
- namespace/bootstrap/proc systems and generated runtime mutation payloads;
- evaluator/parser/runtime/executor files that still require root `Fn`, `Var`, `Expr`, `LocalEnv`, namespace, and frame state.

This means future extraction work should treat `core/types` as the durable object/protocol package and `core/runtime` as the owner of runtime leaf primitives/object wrappers where cycles permit, rather than adding new root protocol aliases.

## Collections move status and remaining prerequisites

Concrete collection implementations have moved to `core/types/collections`: vectors, persistent vectors, maps, sets, lists, seqs, and chunks are no longer root `core` files. Root code constructs them through `corecollections.*`, and `tests/layout_guard.sh` rejects reintroducing the old root collection files.

The contract tests still exercise `ArrayVector`, `Vector`, `PersistentVector`, `ArrayMap`, `HashMap`, `MapSet`, list/array/vector/cons sequences, transient round-trips, and sorted collection behavior. Sorted collections and transients remain root-owned or root-adjacent because they are tied to proc registration, comparator callables, and evaluator/runtime behavior.

For any further collection-adjacent movement, preserve or document:

1. Equality/hash contract for keys and values.
2. Seq contract (`Seqable`, empty seq representation, `First`/`Rest` behavior).
3. Persistent update contract (`Assoc`, `Conj`, `Without`, structural sharing expectations).
4. Transient mutation contract and ownership/lifetime rules.
5. Metadata propagation rules.
6. Printing/`ToString` contract.
7. Construction API the reader/evaluator/std/generated packages should use. **Current state: stale `CollectionConstructionAdapter` and its guard have been removed; call sites use `corecollections.*` direct constructors.**
8. Protocol dispatch surface required by moved concrete types.
9. Runtime hook initialization for errors, arity, formatting, reduced values, nil, and type descriptors.

The collection package owns concrete data structures, not the whole object universe. Runtime/proc/env behavior should move only as coherent runtime batches.

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
├── tail_call.go
└── compiler_bridge.go
```

## Recommended next code moves

Safe moves before broad object extraction:

- Continue moving pure helpers with no root `Fn`/`Var`/`Expr`/namespace dependency into `core/ir`, `core/wasm`, and `core/types`.
- Add tests for extracted helpers before moving callers.
- Keep root-core adapter functions temporary only while their surrounding subsystem is still coupled.
- Keep reader construction behind `readerConstruction` while reader ownership is still root-coupled; collection construction adapters have been removed in favor of direct constructors until concrete types move fully into `core/types/collections`.

Do not yet move wholesale:

- sorted collections/transients whose methods still depend on root proc/comparator/evaluator coupling;
- reader/parser orchestration with namespace/tagged-literal/evaluator side effects;
- evaluator/forms/runtime frames;
- namespace/proc/bootstrap systems.

The next safe moves should continue coherent runtime/env/proc boundary work, generated/bootstrap placement cleanup, or IR/WASM executor work only when their root dependencies are explicit and acyclic.

## Checklist status

- [x] Identify object/protocol contracts that block collection moves.
- [x] Identify reader construction/tagged literal contracts.
- [x] Identify runtime/evaluator call/error/frame contracts.
- [x] Confirm broad R5 moves should continue waiting on explicit contracts.
