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
- `core` is the main monolith.
- Generated `core/a_*.go` files dominate size and should eventually move behind a generated bootstrap package or at least a generated-code directory with explicit init hooks.
- IR/JIT/WASM files are heavily coupled to `core.Object`, `Fn`, `Expr`, `LocalEnv`, and unexported runtime helpers.
- Tracing/profiling code is comparatively separable and is the first extraction target.

## Target package map

Planned package boundaries:

| Target | Current files/examples | Notes |
|---|---|---|
| `core/internal/trace` | `function_trace.go`, later `symbol_trace.go`, `ir_profile.go` state machinery | Leaf package. No dependency on `core`; core passes names/events/op names in. First extraction target. |
| `core/internal/ir` or `core/ir` | `ir*.go`, IR tests | Requires exported runtime interfaces for `Object`, `Fn`, `Expr`, call dispatch, slots, and errors. Do after trace extraction. |
| `core/internal/wasm` or `core/wasm` | `wasm*.go` | Depends on IR program shape; should follow IR split. |
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
- [x] Keep benchmark correctness tests before performance work.

### R4 — Generated code boundary

- [x] Inventory generated file families and generator source packages in `docs/GENERATED_BOUNDARY_AUDIT.md`.
- [x] Add `make generated-check` guardrail and run it from `make docs-check`.
- [x] Guard architecture/refactor assessment documents from accidental removal via `make docs-check`.
- [ ] Update generators to emit under a generated package/directory or clearly separated bootstrap module.
- [ ] Move generated artifacts after runtime/object initialization boundaries are explicit.

### R5 — Collections/reader/runtime follow-up

- [x] Inventory collection/reader/runtime/evaluator/WASM split candidates in `docs/CORE_SPLIT_AUDIT.md`.
- [x] Confirm broad R5 moves should wait until IR/generated boundaries are stable.
- [ ] Move collections only after object/protocol contracts are explicit.
- [ ] Move reader only after object construction and tagged literal contracts are explicit.
- [ ] Move runtime/evaluator only after call/error/frame contracts are explicit.
- [ ] Prefer clean package boundaries over compatibility wrappers; breaking changes are acceptable.

## Current execution status

R3 has established the first IR boundary. R4 generated-code inventory/guardrails are now in place; moving generated artifacts waits on runtime/object initialization boundaries. The CLI entrypoint has moved to `cmd/joker` as the first top-level repository layout cleanup.
