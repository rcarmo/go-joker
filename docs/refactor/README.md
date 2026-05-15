# Repository architecture refactor plan

Updated: 2026-05-15

## Goal

Reduce the current grab-bag layout by moving cohesive implementation families into packages/directories with explicit contracts. The largest target is `core`, which currently mixes reader/parser/evaluator/runtime objects, IR/JIT/WASM, concurrency, tracing, collections, tests, and generated code in one directory.

## Constraints

Go package boundaries are real API boundaries. Moving files into subdirectories changes import paths and loses access to unexported identifiers. Therefore this refactor must be staged by carving out leaf packages first, then introducing exported interfaces where needed.

## Current high-level state

- Module identity is now `github.com/rcarmo/go-joker`; remaining `candid82` references should be attribution/upstream history or third-party dependencies only.
- CLI entrypoint now lives under `cmd/joker`.
- `std/*` is already package-oriented and increasingly guarded by focused native-boundary contracts and explicit resource-layout rules.
- `core` remains the main monolith, but leaf packages now exist under `core/trace`, `core/ir`, `core/wasm`, `core/runtime`, `core/collections`, `core/reader`, `core/string`, `core/cursor`, and data-only generated payloads under `core/generated`.
- Generated `core/a_*.go` files still matter, but the root generated set has shrunk; `tests/generated_files.txt` now tracks root generated files plus data-only generated package artifacts such as `core/generated/linter_payloads_gen.go`. Remaining root generated files stay there only while they still require `package core` access. Moving them to a subdirectory must be a real package split, not a cosmetic file move.
- IR/JIT/WASM compiler and executor files are still coupled to `core.Object`, `Fn`, `Expr`, `LocalEnv`, and unexported runtime helpers, but opcode/diagnostic helpers and WASM leaf helpers have been extracted.
- Tracing/profiling aggregation state is extracted into `core/trace`.

## Refactor document set

This folder is the canonical refactor/audit document set:

- `README.md` — overall plan, status, and target layout.
- `code-structure.md` — broad package/module and coverage audit.
- `module-structure-audit.md` — current Go module/package layout and next structural improvements.
- `module-structure-followup.md` — second-pass package snapshot and concrete next improvements.
- `ir-boundary.md` — IR split inventory and boundary plan.
- `ir-program-split.md` — next-step design for separating neutral IR model from root-core execution metadata.
- `generated-boundary.md` — generated-code inventory and guardrails.
- `generated-bootstrap-contract.md` — next-step data-only contract for moving generated namespace bootstrap payloads.
- `core-split.md` — collections/reader/runtime/WASM split candidates.
- `object-protocol-contracts.md` — object/protocol contracts blocking broad moves.
- `runtime-execution-contract.md` — execution metadata contract required before moving IR executors/runtime frames.
- `reader-construction-contract.md` — object construction/tagged literal contract required before moving reader/parser code.
- `std-resource-layout.md` — repository layout rules for std namespace roots, Go packages, and pure Joker sub-namespace resources.
- `collections-extraction-audit.md` — current collection family grouping, extracted mechanics seams, and blockers for concrete collection moves.

## Target package map

Planned package boundaries:

| Target | Current files/examples | Notes |
|---|---|---|
| `core/trace` | function, symbol, and IR profile state machinery | Extracted leaf package. No dependency on `core`; root `trace_adapters.go` only passes names/events/op names in. |
| `core/ir` | `ir*.go`, IR tests | Extracted neutral IR helpers/model exist; executor/compiler movement still requires exported runtime interfaces for `Object`, `Fn`, `Expr`, call dispatch, slots, and errors. |
| `core/wasm` | `wasm*.go` leaf helpers first | Extracted encoding/module/host metadata helpers exist, but full lowering/runtime still depends on IR program shape and runtime contracts. |
| `core/runtime` | feature flags, goroutine IDs, future eval frames/errors/tracing hooks | Small runtime leaf helpers exist; production executor/runtime moves require explicit object/call/error/frame contracts first. |
| `core/collections` | vectors, maps, sets, seqs, transients | Real mechanics package exists: generic slice storage, pair-array helpers, bitmap/hash-index helpers, and opaque trie node/path helpers are extracted. Root collections delegate mechanics where safe while retaining Object/protocol behavior. Move concrete collection implementations only after object/protocol dependencies are explicit and acyclic. |
| `core/reader` | `reader.go`, `read.go`, tagged literals | Real reader mechanics package exists: line rune reading, character classification, identifier classification/validation, unicode/string escape parsing, number-token classification, and raw file/buffer/buffered/IO mechanics have moved. Root reader still owns errors, FORMAT/LINTER behavior, namespace/tagged-literal handling, concrete Object construction, and parser/evaluator handoff. |
| `core/string` | string caches and string-focused support helpers | Real helper package owns root-independent string mechanics such as char/rune caching, escaping, and nth-rune lookup; root `core` keeps Object/error wrappers. |
| `core/cursor` | string cursor mechanics | Real leaf package owns string cursor iteration mechanics; root `core.StringCursor` is only the Joker Object protocol adapter. |
| `core/generated` | source manifest, linter payload bytes/registry, future data-only payloads | Real generated package boundary exists for data-only payloads. Only move additional generated families when generator output can declare/import a real package with explicit contracts; do not place `package core` files in subdirectories. |
| `tools/tracing` or skill scripts | pprof/IR/function trace renderers | External tooling can move independently of Go runtime packages. |

## Execution phases

### R0 — Inventory and guardrails

- [x] Record package map and migration constraints.
- [x] Keep `make docs-check`, `make bb-compat`, full tests, and vet as required checks.
- [x] Guard module/import identity with `make import-identity-check` from `make docs-check`.
- [x] Guard explicit Babashka/non-goal boundaries with `make non-goals-check` from `make docs-check`.
- [x] Run extracted `core/*` helper subpackage tests from `make docs-check` via `make refactor-internals-check`.
- [x] Guard top-level and extracted internal package layout invariants with `make layout-check` from `make docs-check`.

### R1 — Extract leaf tracing state

- [x] Move function tracing aggregation/writing into `core/trace`.
- [x] Leave core-specific name derivation in `core/function_trace.go`.
- [x] Preserve JSON output shape.
- [x] Run tracing smoke test and full validation.

### R2 — Extract symbol/IR profiling state

- [x] Move symbol trace aggregation into `core/trace`.
- [x] Move IR opcode profile aggregation into `core/trace` while keeping opcode naming in core.
- [x] Preserve JSON output shapes.

### R3 — Define IR boundary

- [x] Audit all `ir*.go` references to unexported core symbols.
- [x] Introduce a minimal exported boundary or adapter layer for opcode names/constants in `core/ir`.
- [x] Move diagnostic/export helpers first, then compiler/executor (started with opcode naming, op counting, disassembly, and shape-analysis helpers; direct tests now cover the extracted IR helper package).
- [x] Design `IRProgram` split into a small `core/ir.Program` model plus root-core execution metadata.
- [x] Start `IRProgram` split with `core/ir.Program` neutral model and root-core envelope population.
- [x] Migrate diagnostics/export accessors to consume `core/ir.Program`.
- [x] Migrate WASM eligibility/basic lowering to consume `core/ir.Program`.
- [x] Migrate multi-function WASM helper eligibility/lowering to consume `core/ir.Program`.
- [x] Migrate imported WASM host-codegen eligibility/lowering to consume `core/ir.Program`.
- [x] Migrate WASM memory helper paths to consume `core/ir.Program`.
- [x] Confirm profile paths are opcode-stream based and do not own program shape.
- [x] Migrate native helper eligibility/lowering to consume `core/ir.Program`.
- [x] Document runtime/object execution metadata contract before moving executors or escape analysis.
- [x] Add initial IR execution-envelope contract tests for constants/captures/escape metadata/model handoff.
- [x] Add escape-analysis contract tests for call-argument unsafety and string-builder slots.
- [x] Add `irMakeFn` execution-envelope contract tests for FnExpr retention and slot capture semantics.
- [x] Add IR failure-cache and native-helper eligibility contract tests.
- [x] Route native-helper and fallback execution state through `RuntimeExecutionAdapter` with contract tests.
- [x] Split typed inline executor helper into a smaller cohesive root file while preserving package ownership.
- [x] Add WASM host native-int conversion contract tests.
- [x] Add initial root `RuntimeExecutionAdapter` code contract for error creation/throwing and `irMakeFn` closure construction.
- [x] Route boxed/typed `irThrow`, capture-slot prefill, and `irMakeFn` execution through `RuntimeExecutionAdapter`.
- [x] Gate runtime/execution-envelope contracts with `make runtime-contract-check` from `make docs-check`.
- [ ] Leave executor/escape-analysis root fields until runtime/object execution metadata contract becomes code.
- [x] Keep benchmark correctness tests before performance work.
- [x] Record full benchmark/profile audit in `docs/BENCHMARK_PROFILE_2026-05-12.md` to guide future runtime work.
- [x] Add stable IR function-cache key contract to prevent repeated compile/envelope allocation regressions.

### R4 — Generated code boundary

- [x] Inventory generated file families and generator source packages in `docs/refactor/generated-boundary.md`.
- [x] Add `make generated-check` guardrail and run it from `make docs-check`.
- [x] Track generated root-core file set in `tests/generated_files.txt`.
- [x] Add a collections extraction audit (`collections-extraction-audit.md`) before the first real collection mechanics move.
- [x] Start collection mechanics extraction with generic vector slice helpers in `core/collections`.
- [x] Move reader identifier rune classification into `core/reader`.
- [x] Move reader identifier validation predicates/reasons into `core/reader`.
- [x] Move reader unicode escape parsing helpers into `core/reader`.
- [x] Move reader simple string escape decoding into `core/reader`.
- [x] Move reader number token classification into `core/reader`.
- [x] Guard architecture/refactor assessment documents from accidental removal via `make docs-check`.
- [x] Design generated bootstrap contract before generator path changes.
- [x] Add `core/generated` data-only bootstrap payload contract types.
- [x] Start generator emission under `core/generated` with core source manifest.
- [x] Add equivalence test comparing generated source manifest with current root `coreNamespaces`.
- [x] Guard generated bootstrap manifest equivalence with `make generated-bootstrap-check` from `make docs-check`.
- [x] Start root runtime consumption of generated source manifest via guarded `core/generated.CoreNamespaces()` helper.
- [x] Switch `*core-namespaces*` bootstrap to generated source manifest plus always-present `user` namespace.
- [x] Stop emitting/tracking root `core/a_data.go` after generated manifest equivalence.
- [x] Guard generated source manifest paths against `core/data` files.
- [x] Guard generated source manifest sync with `CoreSourceFiles`.
- [x] Extend generated bootstrap emission beyond source manifest only after broader equivalence tests.
- [x] Generate and guard the linter payload registry under `core/generated`.
- [ ] Move remaining generated artifacts after runtime/object initialization boundaries are explicit.

### R5 — Collections/reader/runtime follow-up

- [x] Inventory collection/reader/runtime/evaluator/WASM split candidates in `docs/refactor/core-split.md`.
- [x] Inventory object/protocol contracts blocking broad moves in `docs/refactor/object-protocol-contracts.md`.
- [x] Add `make core-contract-check` for object/protocol contract tests that gate future splits.
- [x] Add direct `core/trace` package tests.
- [x] Extend `core-contract-check` with set contracts.
- [x] Extend `core-contract-check` with transient contracts.
- [x] Extend `core-contract-check` with seq contracts.
- [x] Extend `core-contract-check` with sorted collection contracts.
- [x] Extend `core-contract-check` with numeric native-int conversion/promotion contracts.
- [x] Add `make std-contract-check` for focused std native-boundary checks (`http`, `io`, `strconv`, `time`, `markdown`, `os`, `system`, `runtime`, `imaging`, `pdf`, `svg`, `random`, `bolt`, `url`, `git`, `log`, `csv`, `json`, `filepath`); see `std-native-boundary.md`.
- [x] Audit/fix native-int promotion in reader numbers, ratios, BigInt conversion, core file info, HTTP content length, IO copy counts, time durations, strconv parse-int, OS read-dir sizes/timestamps, system time values, runtime profile metrics, and WASM host conversions.
- [x] Guard closed native-int audit TODOs with `make native-int-check` from `make docs-check`.
- [x] Guard ignored close/process/write errors and raw `panic(err)` regressions with `make error-handling-check` from `make docs-check`.
- [x] Confirm broad R5 moves should wait until IR/generated boundaries are stable and object/protocol contracts are explicit.
- [x] Define and guard collection construction adapter before package moves.
- [ ] Move collections only after object/protocol implementation contracts are explicit and acyclic.
- [x] Document reader object-construction/tagged-literal contract requirements.
- [x] Add reader construction contract tests for primitives, collections, metadata, tagged readers, conditionals, namespaced maps, and literal error paths.
- [x] Run reader construction contract tests from `make core-contract-check`.
- [x] Define and guard reader/expression construction adapter before package moves.
- [ ] Move reader only after object construction, tagged literal, and evaluator handoff contracts are explicit and acyclic.
- [x] Document runtime/evaluator call/error/frame contract requirements.
- [x] Start root runtime execution adapter contract in code for error creation and function construction.
- [ ] Move runtime/evaluator only after call/error/frame contracts are explicit in code and root execution metadata has a narrow adapter.
- [x] Prefer clean package boundaries over compatibility wrappers; breaking changes are acceptable.

## Current execution status

R3 has established the first IR boundary and extracted tested IR helper packages. A neutral `core/ir.Program` model now exists and root executable `IRProgram` envelopes populate it; diagnostics/export accessors, WASM lowering helpers, and native helper compilation read from that model where appropriate. Runtime/execution-envelope contracts now run from `make runtime-contract-check` inside `make docs-check`; native-helper/fallback state now routes through `RuntimeExecutionAdapter`, and the typed inline executor helper has been split into a smaller root file. R4 generated-code inventory/guardrails are in place; `core/a_data.go` has been removed in favor of the generated source manifest, and the linter payload registry is now emitted and guarded under `core/generated` while root `core` consumes it through `LinterDataByPath`. Remaining root generated artifacts still wait on runtime/object initialization boundaries. R5 has extracted WASM leaf helpers, collection mechanics helpers, and reader lexical/token helpers, and now has focused contract coverage for vectors, maps, sets, sorted collections, transients, seqs, reader construction, native-int numeric promotion/conversion, std native-boundary returns and arity/shape checks, metadata/info behavior, and persistent-vector semantics. Collection and reader construction adapters plus boundary guards are in place before any concrete package moves. Recent audit passes hardened reader lexical edge cases, HTTP write handling, WASM memory allocation/write checks, and home-directory fallback behavior. `make native-int-check`, `make error-handling-check`, `make std-contract-check`, and the construction/generated guard tests protect recent closures. The CLI entrypoint lives in `cmd/joker` with focused command-package tests.
