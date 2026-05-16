# Reader construction contract

Updated: 2026-05-15

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
2. Introduce a root adapter implementing the reader construction surface. **Done/ongoing: `ReaderConstructionAdapter` now wraps reader creation/read APIs, root read error/object metadata derivation (`ReadError`, `ReadObject`, `DeriveReadObject`), scalar literal object constructors (`Nil`, `Bool`, `Char`, `Double`, `String`, `Symbol`, `Keyword`, `Comment`, `Regex`), numeric token construction (`NumberFromToken`), metadata conversion/application (`MetadataFromObject`, `WithMeta`), initial collection literal constructors (`ListFrom`, `VectorFrom`), and literal/surrogate/vector/map/set expression construction.**
3. Move pure lexical/token helpers first if they have no object dependency. **Started: `core/reader` owns character classes; whitespace/comment/line scanning decisions/runs; identifier rune classification, token scanning, validation, keyword/standalone-slash/literal classification, and validation-issue enumeration; initial-token, top-level read-form, and number-token classification; delimiter/comment/top-level-trivia/terminating macro classification; dispatch classification/format-prefix selection, tagged-literal lookup-name, default-reader-name, format-prefix, and missing-reader mode decisions, and named-character classification; symbolic-value lookup; string/regex/comment/delimiter scanning and invalid-regex mode classification; unicode/octal escape scanning, fixed-width parsing, and string-escape decoding; delimited-form, conditional read-error/suppression/result decisions, and conditional/unquote/namespaced-map start/prefix/splice helpers; syntax-quote auto-gensym name helpers; pending-form popping; arg-index gap filling/ordering; rune-window history; line rune reading; and raw file/buffer/buffered/IO wrappers.**
4. Move reader construction only after all object creation goes through the adapter. **Started: production reader/parser call sites now route through `readerConstruction`, and `construction_boundary_guard_test.go` rejects new direct construction drift.**
5. Keep parser/evaluator handoff in root until expression construction contracts are explicit.

## Non-goals

- Do not export every concrete collection type solely for reader extraction.
- Do not move parser/evaluator with the reader in the same step.
- Do not add compatibility wrappers around old root paths.
- Do not change tagged literal semantics during package movement.

## Current status

Reader extraction remains blocked on object-model/package-cycle ownership for concrete reading/parsing, but the construction boundary is now code rather than a sketch. Collection/object contracts are much stronger now, reader construction contract tests run from `make core-contract-check`, `ReaderConstructionAdapter` owns the narrow root construction surface, and `construction_boundary_guard_test.go` prevents new production call sites from bypassing that adapter. The adapter now also covers root read errors and source metadata attachment/derivation, which is a prerequisite for eventually moving orchestration into `core/reader` without importing root `core`. Pure reader mechanics have continued moving to `core/reader`; the remaining blocker is moving concrete reader/parser implementation without exporting the whole root object/evaluator surface. Root `read.go` should now keep object construction, namespace/tagged-literal semantics, FORMAT/LINTER side effects, and parser/evaluator handoff, while new root-independent rune/token/form decisions should be added to `core/reader` with package tests first.
