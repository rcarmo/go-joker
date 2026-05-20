# Repository architecture refactor plan

Updated: 2026-05-20

## Goal

Reduce the current grab-bag layout by moving cohesive implementation families into packages/directories with explicit contracts. The largest target is `core`, which currently mixes reader/parser/evaluator/runtime objects, IR/JIT/WASM, concurrency, tracing, collections, tests, and generated code in one directory.

## Constraints

Go package boundaries are real API boundaries. Moving files into subdirectories changes import paths and loses access to unexported identifiers. Therefore this refactor must be staged by carving out leaf packages first, then introducing exported interfaces where needed.

## Current high-level state

- Module identity is now `github.com/rcarmo/go-joker`; remaining `candid82` references should be attribution/upstream history or third-party dependencies only.
- CLI entrypoint now lives under `cmd/joker`.
- `std/*` is already package-oriented and increasingly guarded by focused native-boundary contracts and explicit resource-layout rules.
- `core` remains the main monolith, but the object/type split is now real: `core/types` owns the canonical `Object` protocol, type descriptors/registry, scalar values, big numeric values/ops, simple runtime values (`Delay`, `RecurBindings`), most root-independent protocols, and shared collection/runtime contracts (`Map`, `MapIterator`, `Pair`, `Meta`, `MetaHolder`, `Set`, `Vec`, `Ref`). Concrete collections now live in `core/types/collections`, and runtime-owned wrappers for channels, futures/promises, agents, and atoms now live in `core/runtime`. Leaf packages also exist under `core/trace`, `core/ir`, `core/wasm`, `core/reader`, `core/types/string`, and `core/types/numerical`, with data-only generated payloads under `core/generated`.
- Generated `core/a_*.go` files still matter, but the root generated set has shrunk; `tests/generated_files.txt` now tracks root generated files plus data-only generated package artifacts such as `core/generated/linter_payloads_gen.go`. Remaining root generated files stay there only while they still require `package core` access. Moving them to a subdirectory must be a real package split, not a cosmetic file move.
- IR/JIT/WASM compiler and executor files are still coupled to root `Fn`, `Expr`, `LocalEnv`, namespace/frame state, and unexported runtime helpers, but opcode/diagnostic helpers, WASM leaf helpers, and the canonical `coretypes.Object` surface have been extracted. `RuntimeExecutionAdapter` now covers equality, mutable-slot candidate detection, object assoc/nth fallbacks, Fn/runtime call dispatch, program model/constant access, and many executable-envelope seams, but executor loops remain root-owned until frame/call contracts are narrower.
- Tracing/profiling aggregation state is extracted into `core/trace`.


Current focused cleanup metrics (2026-05-20): root `core/*.go` is 29 files with one consolidated root test file; `core/types` is 19 files and owns the canonical object/type/protocol/value contracts plus shared protocols (`Map`, `Meta`, `Set`, `Vec`, `Ref`), transient implementations, tagged-literal parsing helpers, runtime hooks, and generated/assertion replacements for `coretypes.*` and stdlib I/O return types. `core/types/collections` owns concrete vectors/maps/sets/lists/seqs/chunks, and `core/runtime` owns channel/future/promise/agent/atom wrappers plus runtime primitives.

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
- `runtime-executor-audit.md` — current executor/runtime file-family ownership, blockers, and safe next seams.
- `reader-construction-contract.md` — object construction/tagged literal contract required before moving reader/parser code.
- `reader-parser-audit.md` — current remaining reader/parser root ownership, blockers, and next safe extraction seams.
- `std-resource-layout.md` — repository layout rules for std namespace roots, Go packages, and pure Joker sub-namespace resources.
- `collections-extraction-audit.md` — current collection family grouping, extracted mechanics seams, and blockers for concrete collection moves.

## Target package map

Planned package boundaries:

| Target | Current files/examples | Notes |
|---|---|---|
| `core/trace` | function, symbol, and IR profile state machinery | Extracted leaf package. No dependency on `core`; root `trace_adapters.go` only passes names/events/op names in. |
| `core/ir` | `ir*.go`, IR tests | Extracted neutral IR helpers/model exist; executor/compiler movement still requires exported runtime interfaces for `Object`, `Fn`, `Expr`, call dispatch, slots, and errors. |
| `core/wasm` | `wasm*.go` leaf helpers first | Extracted encoding/module/host metadata helpers exist, but full lowering/runtime still depends on IR program shape and runtime contracts. |
| `core/runtime` | feature flags, goroutine IDs, pending/channel primitives, runtime wrappers, future eval frames/errors/tracing hooks | Runtime leaf helpers and wrappers for `ObjectChannel`, `ObjectFuture`, `ObjectPromise`, `Agent`, and `Atom` exist; production executor/runtime/env moves still require explicit object/call/error/frame/namespace contracts first. |
| `core/types/collections` | vectors, maps, sets, seqs, chunks | Concrete collection package exists: vectors, persistent vectors, lists/seqs, array/hash maps, sets, chunks, formatting/indexed ops, bitmap/hash-index helpers, and trie/path helpers are extracted. Root code uses `corecollections.*` direct constructors. Remaining collection-adjacent work is sorted/proc coupling, transients, evaluator fast paths, and generated/bootstrap placement. |
| `core/reader` | `read.go`, tagged literals | Real reader mechanics package exists: rune-window history, line rune reading, rune-stream Get/Unget/Peek position mechanics, reader position-stack snapshots, character classification, whitespace/comment/line scanning, identifier token scanning/validation configuration/issue enumeration, unicode/string escape parsing, number-token classification, dispatch/form/prefix helpers, and raw file/buffer/buffered/IO mechanics have moved. Root reader still owns filename interning, errors, FORMAT/LINTER behavior, namespace/tagged-literal handling, concrete Object construction, and parser/evaluator handoff; the tiny root `reader.go` wrapper has been folded into `read.go`. `ReaderConstructionAdapter` now covers read errors, source metadata, scalar/comment/regex/numeric literals, metadata, list/vector/map/set literal construction, conditional vectors, and expression constructors. |
| `core/types/string` | string caches, cursor mechanics, and string-focused support helpers | Real helper package owns root-independent string mechanics such as char/rune caching, escaping, joining, nth-rune lookup, and cursor iteration; root keeps object/error adapters where needed. |
| `core/generated` | source manifest, linter payload bytes/registry, future data-only payloads | Real generated package boundary exists for data-only payloads. Only move additional generated families when generator output can declare/import a real package with explicit contracts; do not place `package core` files in subdirectories. Root `types_assert_gen.go`/`types_info_gen.go` have been replaced by explicit root support co-located in `object.go`. |
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
- [x] Leave core-specific name derivation co-located with core call mechanics in `core/object.go`.
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
- [x] Harden benchmark comparison correctness: cross-runtime result payload validation, stale-output cleanup, strict chart/table input validation, decimal Go benchmark parsing, import-safe Python runner, Node-compatible JS timing, Goja fail-fast behavior, and required validated go-joker let-go suite results.

### R4 — Generated code boundary

- [x] Inventory generated file families and generator source packages in `docs/refactor/generated-boundary.md`.
- [x] Classify remaining root generated artifacts as runtime-mutating bootstrap or object-model helper code before any next generated move.
- [x] Add `make generated-check` guardrail and run it from `make docs-check`.
- [x] Track generated root-core file set in `tests/generated_files.txt`.
- [x] Add a collections extraction audit (`collections-extraction-audit.md`) before the first real collection mechanics move.
- [x] Start collection mechanics extraction with generic vector slice helpers in `core/types/collections`.
- [x] Move reader identifier rune classification into `core/reader`.
- [x] Move reader identifier validation predicates/reasons/configuration/issue enumeration/explanations into `core/reader` and remove stale root wrappers.
- [x] Move reader unicode escape parsing helpers into `core/reader`.
- [x] Move reader fixed-width unicode/octal parsing helper into `core/reader`.
- [x] Move reader simple string escape decoding into `core/reader`.
- [x] Move reader number token classification into `core/reader`.
- [x] Move reader identifier token validation into `core/reader`.
- [x] Move reader identifier token scanning into `core/reader`.
- [x] Move reader keyword/auto-resolved-keyword and standalone slash identifier classification into `core/reader`.
- [x] Move reader identifier literal classification into `core/reader`.
- [x] Move reader initial token and top-level read-form classification into `core/reader`.
- [x] Move reader comment and line-skip scanning into `core/reader`.
- [x] Move reader string unicode escape scanning into `core/reader`.
- [x] Move reader regex literal body scanning and invalid-regex mode classification into `core/reader`.
- [x] Move reader delimiter token scanning into `core/reader`.
- [x] Reuse reader delimiter token scanning for character unicode literals.
- [x] Move reader expected-token and delimiter peek mechanics into `core/reader`.
- [x] Move reader whitespace-skip decisions/runs into `core/reader`.
- [x] Move reader string escape classification into `core/reader`.
- [x] Move reader string literal body scanning into `core/reader`.
- [x] Move reader terminating macro classification into `core/reader` and remove stale root wrappers.
- [x] Move reader comment-start, top-level trivia, and closing-delimiter classification into `core/reader`.
- [x] Move reader named character classification into `core/reader`.
- [x] Move reader character unicode/octal classification into `core/reader`.
- [x] Move reader symbolic-value lookup into `core/reader`.
- [x] Move reader source filename fallback into `core/reader`.
- [x] Move reader dispatch macro classification, format-prefix selection, and tagged-literal lookup-name/default-reader-name/format-prefix/missing-reader mode decisions into `core/reader`.
- [x] Move reader map form-count helpers into `core/reader`.
- [x] Move reader bare arg-literal classification into `core/reader`.
- [x] Move reader conditional/unquote/namespaced-map/simple-macro/delimited-form loop, read-error mode, suppression, result, start, prefix, and splice helpers into `core/reader`.
- [x] Move reader arg-index gap filling/ordering into `core/reader`.
- [x] Move reader pending-form pop and top-level splice-surrogate helpers into `core/reader`.
- [x] Move reader syntax-quote auto-gensym name classification/prefix helpers into `core/reader`.
- [x] Move reader position-stack mechanics into `core/reader` while root keeps ObjectInfo construction.
- [x] Move reader rune-stream Get/Unget/Peek and position accounting mechanics into `core/reader` while root keeps filename interning and error conversion.
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
- [ ] Move remaining generated artifacts after runtime/object initialization boundaries are explicit. (Remaining root generated artifacts are classified in `generated-boundary.md` as runtime-mutating bootstrap or object-model helper code.)

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
- [x] Add `make std-contract-check` for focused std native-boundary checks (`http`, `io`, `strconv`, `time`, `markdown`, `os`, `system`, `runtime`, `imaging`, `pdf`, `svg`, `random`, `bolt`, `url`, `git`, `log`, `csv`, `json`, `filepath`, `crypto`, `math`, `string`, `uuid`); see `std-native-boundary.md`.
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

R3 has established the first IR boundary and extracted tested IR helper packages. A neutral `core/ir.Program` model now exists and root executable `IRProgram` envelopes populate it; diagnostics/export accessors, WASM lowering helpers, and native helper compilation read from that model where appropriate. Runtime/execution-envelope contracts now run from `make runtime-contract-check` inside `make docs-check`; native-helper/fallback state now routes through `RuntimeExecutionAdapter`, and the typed inline executor helper has been split into a smaller root file. R4 generated-code inventory/guardrails are in place; `core/a_data.go` has been removed in favor of the generated source manifest, and the linter payload registry is now emitted and guarded under `core/generated` while root `core` consumes it through `LinterDataByPath`. Remaining root generated artifacts still wait on runtime/object initialization boundaries. R5 has extracted WASM leaf helpers, collection mechanics helpers, and reader lexical/token helpers, and now has focused contract coverage for vectors, maps, sets, sorted collections, transients, seqs, reader construction, native-int numeric promotion/conversion, std native-boundary returns and arity/shape checks, metadata/info behavior, and persistent-vector semantics. Collection and reader construction adapters plus boundary guards are in place before any concrete package moves. Recent audit passes hardened reader lexical edge cases, HTTP write/status/client-option/address handling, SVG/PDF/imaging native dimension/color/finite-float bounds, CSV delimiter options, OS process output/handle lifecycle, pod/timeout millisecond duration bounds, WASM memory allocation/write checks, home-directory fallback behavior, string bounds/version parsing, sorted-map duplicate semantics, transient/MapSet zero values, Transit cmap parsing, external URL parsing, standalone compile short writes, protocol extension pair arity, and `alts!` option arity. `make native-int-check`, `make error-handling-check`, `make std-contract-check`, staticcheck, govulncheck, and the construction/generated guard tests protect recent closures. The CLI entrypoint lives in `cmd/joker` with focused command-package tests.
