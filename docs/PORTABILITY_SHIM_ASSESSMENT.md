# Optional portability shim assessment

Updated: 2026-05-11

## Purpose

This records the Phase 7 assessment for optional portability namespaces and helpers. The rule remains workload-driven compatibility: add a shim only when a real script or committed fixture requires it and when the behavior maps cleanly to go-joker's Go-native runtime.

## Namespaces/features reviewed

Searched current runtime fixtures, parity tests, docs, core data, and stdlib sources for demand around:

- terminal helpers (`term`)
- low-level process/OS APIs (`syscall`, `unix`)
- filesystem tree walking (`walk`)
- archive helpers (`zip`)
- diagnostics/data dumping (`dump`)
- test framework compatibility (`test` / `clojure.test`)

## Findings

- `term`: no current runtime fixture or workload requires terminal control helpers.
- `syscall` / `unix`: no portable fixture requires low-level syscall surfaces. These remain intentionally omitted unless a platform-gated, narrow use case appears.
- `walk`: go-joker already ships `joker.walk` for data-structure walking. No current fixture requires a Babashka-style filesystem walk helper.
- `zip`: no current runtime fixture requires archive creation/extraction helpers.
- `dump`: no current runtime fixture requires a diagnostic dump namespace.
- `test`: go-joker already has `joker.test`, and the imported jank subset includes a small `clojure.test` harness fixture. No broader Babashka/Clojure test framework shim is required by the current portable script suite.

## Decision

Do **not** add new optional portability shims in this batch.

Rationale:

- There is no real script demand in the current fixture/workload set.
- Broad `syscall`/`unix` compatibility would be non-portable and outside go-joker's current goals.
- Existing `joker.walk` and `joker.test` cover the repository's current needs without pretending full Babashka or JVM/Clojure catalog parity.

## Future acceptance criteria

A future optional shim may be added only if:

1. A real script or committed fixture fails without it.
2. The required API surface is narrow and documented.
3. Platform-specific behavior is guarded explicitly.
4. Tests cover success and expected unsupported behavior.
5. Docs state omissions clearly.

## Phase 7 status

- [x] Assess demand for `term`, `syscall`, `unix`.
- [x] Assess demand for `walk`, `zip`, `dump`, `test`.
- [x] Preserve omission of broad low-level surfaces without concrete use cases.
- [ ] Implement platform-gated shims only if real scripts require them.
