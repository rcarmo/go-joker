# Repository architecture refactor plan

Updated: 2026-05-11

## Goal

Reduce the current grab-bag layout by moving cohesive implementation families into packages/directories with explicit contracts. The largest target is `core`, which currently mixes reader/parser/evaluator/runtime objects, IR/JIT/WASM, concurrency, tracing, collections, tests, and generated code in one directory.

## Constraints

Go package boundaries are real API boundaries. Moving files into subdirectories changes import paths and loses access to unexported identifiers. Therefore this refactor must be staged by carving out leaf packages first, then introducing exported interfaces where needed.

## Current high-level state

- Module identity is now `github.com/rcarmo/go-joker`; remaining `candid82` references should be attribution/upstream history or third-party dependencies only.
- CLI entrypoint now lives under `cmd/joker`.
- `std/*` is already package-oriented and mostly healthy.
- `core` remains the main monolith, but leaf packages now exist under `core/internal`.
- Generated `core/a_*.go` files dominate size and are tracked by `tests/generated_files.txt`; they should eventually move behind a generated bootstrap package or clearly separated bootstrap module.
- IR/JIT/WASM compiler and executor files are still coupled to `core.Object`, `Fn`, `Expr`, `LocalEnv`, and unexported runtime helpers, but opcode/diagnostic helpers and WASM leaf helpers have been extracted.
- Tracing/profiling aggregation state is extracted into `core/internal/trace`.

## Refactor document set

This folder is the canonical refactor/audit document set:

- `README.md` — overall plan, status, and target layout.
- `code-structure.md` — broad package/module and coverage audit.
- `module-structure-audit.md` — current Go module/package layout and next structural improvements.
- `module-structure-followup.md` — second-pass package snapshot and concrete next improvements.
- `ir-boundary.md` — IR split inventory and boundary plan.
- `ir-program-split.md` — next-step design for separating neutral IR model from root-core execution metadata.
- `generated-boundary.md` — generated-code inventory and guardrails.
- `core-split.md` — collections/reader/runtime/WASM split candidates.
- `object-protocol-contracts.md` — object/protocol contracts blocking broad moves.

## Target package map

Planned package boundaries:

| Target | Current files/examples | Notes |
|---|---|---|
| `core/internal/trace` | `function_trace.go`, `symbol_trace.go`, `ir_profile.go` state machinery | Extracted leaf package. No dependency on `core`; core passes names/events/op names in. |
| `core/internal/ir` or `core/ir` | `ir*.go`, IR tests | Requires exported runtime interfaces for `Object`, `Fn`, `Expr`, call dispatch, slots, and errors. Do after trace extraction. |
| `core/internal/wasm` | `wasm*.go` leaf helpers first | Encoding, module builder, host metadata, and shared constants are extracted; full lowering/runtime still depends on IR program shape and should follow the IR split. |
| `core/runtime` | goroutine runtime, eval frames, errors, tracing hooks | Needs careful cycle avoidance with evaluator/object model. |
| `core/collections` | vectors, maps, sets, seqs, transients | Large API surface; split only after IR/generated boundaries are stable. |
| `core/reader` | `reader.go`, `read.go`, tagged literals | Candidate after object/collection API is stable. |
| `core/generated` | `a_*.go`, `types_*_gen.go` | Needs generator updates and bootstrapping contract. |
| `tools/tracing` or skill scripts | pprof/IR/function trace renderers | External tooling can move independently of Go runtime packages. |

## Execution phases

### R0 — Inventory and guardrails

- [x] Record package map and migration constraints.
- [x] Keep `make docs-check`, `make bb-compat`, full tests, and vet as required checks.
- [x] Guard module/import identity with `make import-identity-check` from `make docs-check`.
- [x] Guard explicit Babashka/non-goal boundaries with `make non-goals-check` from `make docs-check`.
- [x] Run extracted `core/internal/...` package tests from `make docs-check` via `make refactor-internals-check`.
- [x] Guard top-level layout invariants with `make layout-check` from `make docs-check`.

### R1 — Extract leaf tracing state

- [x] Move function tracing aggregation/writing into `core/internal/trace`.
- [x] Leave core-specific name derivation in `core/function_trace.go`.
- [x] Preserve JSON output shape.
- [x] Run tracing smoke test and full validation.

### R2 — Extract symbol/IR profiling state

- [x] Move symbol trace aggregation into `core/internal/trace`.
- [x] Move IR opcode profile aggregation into `core/internal/trace` while keeping opcode naming in core.
- [x] Preserve JSON output shapes.

### R3 — Define IR boundary

- [x] Audit all `ir*.go` references to unexported core symbols.
- [x] Introduce a minimal exported boundary or adapter layer for opcode names/constants in `core/internal/ir`.
- [x] Move diagnostic/export helpers first, then compiler/executor (started with opcode naming, op counting, disassembly, and shape-analysis helpers; direct tests now cover the extracted IR helper package).
- [x] Design `IRProgram` split into a small `core/internal/ir.Program` model plus root-core execution metadata.
- [x] Start `IRProgram` split with `core/internal/ir.Program` neutral model and root-core envelope population.
- [x] Migrate diagnostics/export accessors to consume `core/internal/ir.Program`.
- [ ] Migrate profile/WASM eligibility to consume `core/internal/ir.Program`.
- [x] Keep benchmark correctness tests before performance work.

### R4 — Generated code boundary

- [x] Inventory generated file families and generator source packages in `docs/refactor/generated-boundary.md`.
- [x] Add `make generated-check` guardrail and run it from `make docs-check`.
- [x] Track generated root-core file set in `tests/generated_files.txt`.
- [x] Guard architecture/refactor assessment documents from accidental removal via `make docs-check`.
- [ ] Update generators to emit under a generated package/directory or clearly separated bootstrap module.
- [ ] Move generated artifacts after runtime/object initialization boundaries are explicit.

### R5 — Collections/reader/runtime follow-up

- [x] Inventory collection/reader/runtime/evaluator/WASM split candidates in `docs/refactor/core-split.md`.
- [x] Inventory object/protocol contracts blocking broad moves in `docs/refactor/object-protocol-contracts.md`.
- [x] Add `make core-contract-check` for object/protocol contract tests that gate future splits.
- [x] Add direct `core/internal/trace` package tests.
- [x] Extend `core-contract-check` with set contracts.
- [x] Extend `core-contract-check` with transient contracts.
- [x] Extend `core-contract-check` with seq contracts.
- [x] Confirm broad R5 moves should wait until IR/generated boundaries are stable and object/protocol contracts are explicit.
- [ ] Move collections only after object/protocol contracts are explicit.
- [ ] Move reader only after object construction and tagged literal contracts are explicit.
- [ ] Move runtime/evaluator only after call/error/frame contracts are explicit.
- [ ] Prefer clean package boundaries over compatibility wrappers; breaking changes are acceptable.

## Current execution status

R3 has established the first IR boundary and extracted tested IR helper packages. A neutral `core/internal/ir.Program` model now exists and root executable `IRProgram` envelopes populate it; diagnostics/export accessors read from that model, with profile and WASM lowering still pending. R4 generated-code inventory/guardrails are in place, including a manifest of root-core generated files; moving generated artifacts waits on runtime/object initialization boundaries. R5 has extracted WASM leaf helpers and now has focused contract coverage for vectors, maps, sets, transients, seqs, metadata/info behavior, and persistent-vector semantics. The CLI entrypoint lives in `cmd/joker`.
