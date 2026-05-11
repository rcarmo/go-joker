# Babashka namespace shim assessment

Updated: 2026-05-11

## Purpose

This records the Phase 6 decision process for practical Babashka namespace shims. The rule is script-driven compatibility: add small adapters only when real fixtures or user workloads require them and the behavior maps cleanly to go-joker's Go-native runtime.

## Evidence reviewed

Repository/workload search covered:

- `babashka.fs`
- `babashka.process`
- `clojure.java.io`
- broad catalog libraries such as `babashka.curl`, `cheshire`, `clojure.data.*`, `selmer`, and `rewrite-clj`

Current findings:

- No portable runtime fixture currently requires `babashka.fs`.
- No portable runtime fixture currently requires `babashka.process`.
- `clojure.java.io` appears in linter-oriented fixtures/documentation, but not as a runtime workload requiring Java IO object compatibility.
- The current `make bb-compat` positive fixtures pass using existing go-joker facilities:
  - core `slurp`/`spit`
  - `joker.os`
  - `joker.http`
  - `joker.json` / `joker.yaml`
  - `joker.edn` / `edn`
  - pods payload support

## Decision

Do **not** add new namespace shims in this batch.

Rationale:

- Adding empty or broad alias namespaces would imply more Babashka compatibility than go-joker actually supports.
- The current fixture suite does not demonstrate a need for `babashka.fs`, `babashka.process`, or `clojure.java.io` wrappers.
- go-joker already has direct primitives for the tested portable workflows.
- Java/JVM-shaped APIs should remain explicit non-goals unless a small, Go-native convenience wrapper can be specified without pretending JVM object compatibility.

## Explicitly omitted areas

These remain omitted unless future real scripts justify a narrow subset:

- `babashka.curl`
- full `babashka.fs`
- full `babashka.process`
- full `cheshire`
- `clojure.data.*` catalog parity
- `selmer`
- `rewrite-clj`
- broad `clojure.java.io` object semantics
- Java constructors/static members/reflection/classpath behavior

## Acceptable future shim criteria

A new shim may be added only if all are true:

1. A real script or committed compatibility fixture fails without it.
2. The required API surface is small and documented.
3. The behavior maps directly to existing Go/go-joker primitives.
4. Tests cover the exact portability behavior.
5. The namespace docs state intentional omissions.

## Recommended next fixtures if demand appears

- `babashka.fs`: `exists?`, `directory?`, `regular-file?`, `create-dirs`, `delete`, `list-dir`, `path` if scripts use those names.
- `babashka.process`: `shell`/`process` wrappers only if `joker.os/exec` shape is not enough.
- `clojure.java.io`: `file`, `reader`, `writer`, `input-stream`, `output-stream` only as Go IO/file conveniences, not Java object compatibility.
