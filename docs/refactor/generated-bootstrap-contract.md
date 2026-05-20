# Generated bootstrap contract design note

Updated: 2026-05-20

## Purpose

This note narrows R4's next step before moving generated artifacts out of root `core`: define the minimum bootstrap contract between generated namespace data and the handwritten runtime.

Most current generated files still stay in package `core` because they freely reference namespace internals, vars, metadata, Fn/proc state, and init ordering. The former root `core/a_data.go` namespace list has moved behind the data-only generated manifest, and bootstrap output now imports moved collection/runtime owners directly for values such as `corecollections.ArrayMap`, `corert.ObjectChannel`, and `corert.Atom`/`corert.NewAtom`. Moving the remaining generated files blindly would still force broad exports or create compatibility wrappers, both of which are non-goals.

## Current generated responsibilities

Generated files currently provide three broad things:

1. **Namespace source/data payloads** from `core/data/*.joke`.
2. **Bootstrap/init glue** that installs core/runtime vars and doc metadata.
3. **Generated object helpers** such as type assertion and `WithInfo` methods.

Only the first two belong in a future generated bootstrap package. Type assertion/info generation can stay near the object model until object interfaces move.

## Proposed contract

The `core/generated` package now defines the inert payload structs for this contract. It should own generated payload data, not runtime mutation. The handwritten root runtime should own interpretation and installation.

Current/target shape:

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

type BinaryPayload struct {
    Path string
    Data []byte
}

func CoreSourceManifest() []NamespaceSource
func LinterDataPayloads() []BinaryPayload
func LinterDataByPath(path string) ([]byte, bool)
```

Root `core` would then provide the only mutation/install API:

```go
func installGeneratedSources(sources []generated.NamespaceSource) error
func installGeneratedDocs(docs []generated.VarDoc) error
```

This keeps generated output data-oriented and avoids importing root `core` from the generated package.

## Migration sequence

1. Keep existing root generated files guarded by `tests/generated_files.txt`.
2. Define the data-only payload structs under `core/generated`. **Done: `NamespaceSource` and `VarDoc` are in place with direct tests.**
3. Teach generators to emit data-only payloads under `core/generated` while still emitting the current root files. **Started: `core_sources_gen.go` now emits the core source manifest, `linter_payloads_gen.go` now emits the generated linter payload registry, and root bootstrap emission now uses moved package references for concrete collections and runtime-owned Atom/Channel wrappers.**
4. Add tests comparing data-only payloads with current root generated behavior. **Started: root-core tests compare generated source-manifest namespaces with current bootstrap behavior; internal generated tests verify manifest source paths exist and generated linter payloads match manifest linter entries; `tests/generated_source_manifest_guard.py` verifies the emitted manifest stays in sync with `CoreSourceFiles`; `tests/generated_guard.sh` now tracks the generated linter registry; `make generated-bootstrap-check` and `make docs-check` guard this equivalence.**
5. Switch root bootstrap to consume `core/generated` payloads. **Done for `*core-namespaces*`: `core/generated.CoreNamespaces()` consumes the generated source manifest, and root `setCoreNamespaces` populates `*core-namespaces*` from that helper plus the always-present `user` namespace.**
6. Remove root generated bootstrap files from `tests/generated_files.txt` only after equivalent behavior is proven. **Done for `core/a_data.go`; it is no longer emitted or tracked. The linter byte slices remain generated data files, but their registry is now emitted under `core/generated/linter_payloads_gen.go` and tracked by the generated-file guard.**
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
- Generated payload accessors should return fresh data slices where callers receive mutable structs; `core/generated` tests guard this for the source manifest. Binary payload data is currently exposed as read-only-by-contract byte slices consumed immediately by root `core`.
