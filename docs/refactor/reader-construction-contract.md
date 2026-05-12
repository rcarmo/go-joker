# Reader construction contract

Updated: 2026-05-12

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

1. Add focused contract tests for reader-created primitives, collections, metadata, and tagged literals. **Started: `reader_construction_contract_test.go` covers primitive/collection construction, source info, metadata, direct `*data-readers*` dispatch, and tagged fallback behavior.**
2. Introduce a root adapter implementing the reader construction surface.
3. Move pure lexical/token helpers first if they have no object dependency.
4. Move reader construction only after all object creation goes through the adapter.
5. Keep parser/evaluator handoff in root until expression construction contracts are explicit.

## Non-goals

- Do not export every concrete collection type solely for reader extraction.
- Do not move parser/evaluator with the reader in the same step.
- Do not add compatibility wrappers around old root paths.
- Do not change tagged literal semantics during package movement.

## Current status

Reader extraction remains blocked. Collection/object contracts are much stronger now, and initial reader construction contract tests cover source metadata plus direct/fallback tagged literal behavior, but an explicit construction adapter plus expression construction boundaries are still needed before package movement.
