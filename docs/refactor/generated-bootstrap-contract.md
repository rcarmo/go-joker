# Generated bootstrap contract design note

Updated: 2026-05-11

## Purpose

This note narrows R4's next step before moving generated artifacts out of root `core`: define the minimum bootstrap contract between generated namespace data and the handwritten runtime.

The current generated files stay in package `core` because they freely reference runtime object constructors, namespace internals, vars, metadata, and init ordering. Moving them blindly would either force broad exports or create compatibility wrappers, both of which are non-goals.

## Current generated responsibilities

Generated files currently provide three broad things:

1. **Namespace source/data payloads** from `core/data/*.joke`.
2. **Bootstrap/init glue** that installs core/runtime vars and doc metadata.
3. **Generated object helpers** such as type assertion and `WithInfo` methods.

Only the first two belong in a future generated bootstrap package. Type assertion/info generation can stay near the object model until object interfaces move.

## Proposed contract

The `core/internal/generated` package now defines the inert payload structs for this contract. It should own generated payload data, not runtime mutation. The handwritten root runtime should own interpretation and installation.

Candidate shape:

```go
package generated

type NamespaceSource struct {
    Name string
    Path string
    Source string
}

type VarDoc struct {
    Namespace string
    Name string
    Doc string
    Arglists []string
    Private bool
}

func CoreSources() []NamespaceSource
func CoreDocs() []VarDoc
```

Root `core` would then provide the only mutation/install API:

```go
func installGeneratedSources(sources []generated.NamespaceSource) error
func installGeneratedDocs(docs []generated.VarDoc) error
```

This keeps generated output data-oriented and avoids importing root `core` from the generated package.

## Migration sequence

1. Keep existing root generated files guarded by `tests/generated_files.txt`.
2. Define the data-only payload structs under `core/internal/generated`. **Done: `NamespaceSource` and `VarDoc` are in place with direct tests.**
3. Teach generators to emit data-only payloads under `core/internal/generated` while still emitting the current root files. **Started: `core_sources_gen.go` now emits the core source manifest.**
4. Add tests comparing data-only payloads with current root generated behavior.
5. Switch root bootstrap to consume `core/internal/generated` payloads.
6. Remove root generated bootstrap files from `tests/generated_files.txt` only after equivalent behavior is proven.
7. Leave type assertion/info generation near the object model until object boundaries are explicit.

## Non-goals

- Do not export root `core` namespace internals solely for generated code.
- Do not move generated files by path alone without changing ownership semantics.
- Do not add long-term compatibility wrappers.
- Do not combine generated bootstrap migration with object/protocol extraction.

## Guardrails

- `make generated-check` remains mandatory from `make docs-check`.
- Regenerated output must be reproducible.
- Namespace/docs behavior must remain covered by docs generation and parity checks.
