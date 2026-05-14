# Reader construction contract

Updated: 2026-05-14

## Purpose

This note records the boundary required before moving `reader.go`, `read.go`, parser helpers, or tagged-literal handling out of root `core`.

The reader currently constructs concrete root objects directly and also attaches source metadata. Moving it before this contract becomes code would force broad exports from `core` or create import cycles.

## Root-coupled responsibilities today

Reader/parser code currently depends on root-owned behavior for:

- scalar constructors (`MakeString`, `MakeKeyword`, `MakeSymbol`, numeric builders)
- concrete collection builders (`ArrayVector`, `ArrayMap`, `MapSet`, lists, seqs)
- object info/source metadata (`ObjectInfo`, `Position`, `WithInfo`)
- tagged literal dispatch and unknown-tag behavior
- namespace-sensitive reader behavior and data-reader lookup
- conditional reader features
- parse/eval handoff for forms that become `Expr`
- runtime error formatting via `RT.NewError` and parse errors

## Required construction surface

Before extracting reader code, define a small construction interface. Sketch:

```go
type ReaderObjects interface {
    Nil() Object
    Bool(bool) Object
    String(string) Object
    Symbol(ns, name string) Object
    Keyword(ns, name string) Object
    Int(string) (Object, error)
    Float(string) (Object, error)
    Ratio(num, den string) (Object, error)
}

type ReaderCollections interface {
    List([]Object) Object
    Vector([]Object) Object
    Map([]Object) Object
    Set([]Object) Object
}

type ReaderMetadata interface {
    WithInfo(Object, *ObjectInfo) Object
    Derive(Object, Object) Object
}

type ReaderTags interface {
    ReadTagged(tag Symbol, form Object) (Object, error)
}
```

This is a design sketch, not an implementation API. The important rule is that extracted reader code should depend on construction/tag/metadata services, not on the entire evaluator/runtime.

## Migration sequence

1. Add focused contract tests for reader-created primitives, collections, metadata, tagged literals, and reader conditionals. **Started: `reader_construction_contract_test.go` covers primitive/collection construction, source info, metadata, namespaced maps, map/set literal errors, direct `*data-readers*` dispatch, tagged fallback behavior, and `#?`/`#?@` conditional selection/splicing.**
2. Introduce a root adapter implementing the reader construction surface. **Done: `ReaderConstructionAdapter` now wraps reader creation/read APIs and literal/surrogate/vector/map/set expression construction.**
3. Move pure lexical/token helpers first if they have no object dependency.
4. Move reader construction only after all object creation goes through the adapter. **Started: production reader/parser call sites now route through `readerConstruction`, and `construction_boundary_guard_test.go` rejects new direct construction drift.**
5. Keep parser/evaluator handoff in root until expression construction contracts are explicit.

## Non-goals

- Do not export every concrete collection type solely for reader extraction.
- Do not move parser/evaluator with the reader in the same step.
- Do not add compatibility wrappers around old root paths.
- Do not change tagged literal semantics during package movement.

## Current status

Reader extraction remains blocked on object-model/package-cycle ownership, but the construction boundary is now code rather than a sketch. Collection/object contracts are much stronger now, reader construction contract tests run from `make core-contract-check`, `ReaderConstructionAdapter` owns the narrow root construction surface, and `construction_boundary_guard_test.go` prevents new production call sites from bypassing that adapter. The remaining blocker is moving concrete reader/parser implementation without exporting the whole root object/evaluator surface.
