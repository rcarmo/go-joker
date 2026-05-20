# Code structure, module boundaries, and coverage audit

Generated: 2026-05-10
Updated: 2026-05-20

## Executive summary

The repository is functional and well-tested at the behavior/regression level, but it has a classic interpreter/runtime shape: a large `core` package owns most object model, evaluator, reader/parser, namespace, numeric, concurrency, IR, and WASM responsibilities. `std/*` packages are better bounded: each namespace has a small registration wrapper plus native implementation/tests.

Recent feature work improved boundaries for new code (`std/transit`, `std/system`, `std/jit` export APIs), and the refactor pass has now moved the CLI to `cmd/joker`, extracted leaf packages under `core/{trace,ir,wasm,runtime,reader,types/collections,types/string,types/numerical}`, introduced data-only generated payloads under `core/generated`, and added guards for moved collection/runtime boundaries. The older `core` package still needs gradual decomposition. The safest path is to keep defining internal contracts first and enforce them with small tests, docs, and Makefile targets.

## Current package/module shape

- `cmd/joker/` owns the CLI entrypoint, REPL, standalone compilation helpers, and platform exit handling.
- `core/trace`, `core/ir`, `core/wasm`, `core/runtime`, `core/types/collections`, `core/reader`, `core/types/string`, and `core/types/numerical` own extracted helpers and moved concrete families with direct package tests.
- `core/` is still the runtime kernel and contains:
  - remaining root runtime object systems (`object.go`, root generated helpers) plus `core/types` for the canonical object/type/protocol model
  - proc/env/evaluator glue for moved collection/runtime objects
  - reader/parser/evaluator (`read.go`, `parse.go`, `eval.go`)
  - namespace/Var/runtime environment (`ns.go`, `environment*.go`)
  - core proc implementations (`procs.go`, generated `a_code.go`)
  - concurrency/core.async/atom registration glue over runtime-owned channel/future/promise/agent/atom wrappers
  - numeric tower (`numbers.go`)
  - IR/JIT/WASM machinery (`ir_*`, `wasm_*`)
- `std/*` packages follow a clearer contract:
  - `a_<namespace>.go`: namespace registration and arity/type adapter layer
  - `*_native.go`: implementation logic
  - `*_test.go`: native behavior tests
  - `<namespace>.joke`: public docs/API source for generated docs
- `docs/` generation is runtime-driven and now fails on warning output.
- `benchmarks/core/` owns Go benchmark harnesses; root `core` tests should not define `Benchmark*` functions.
- `benchmarks/` and `tests/` are behavioral/performance guardrails rather than package-level unit tests.

## Boundaries and contracts that are clear

### `std/*` namespace contract

A standard namespace should keep these boundaries:

1. `a_<ns>.go` owns `GLOBAL_ENV.EnsureSymbolIsLib`, `InternVar`, arity checks, and public names.
2. `*_native.go` owns implementation and should not mutate namespace/global state.
3. `*_test.go` should call native helpers directly for fast unit coverage and use CLI smoke tests where namespace wiring matters.
4. `<ns>.joke` owns public documentation and generated API shape.

Recent `std/transit` and `std/system` match this pattern.

### IR/WASM/JIT contract

- `core/ir` owns opcode constants/names, bytecode counting/disassembly helpers, and IR shape analysis.
- `core/wasm` owns leaf WASM encoding/module/host metadata/value-type helpers.
- Root `core` still owns `IRProgram`, lowering, execution, WASM emission orchestration and diagnostics adapters.
- `std/jit` consumes only exported `core` bridge methods (`IrCompileFn`, `IrExec*`, `IrDisassemble`, `IrToWasmExported`, `WasmCompileBytesExported`).
- Artifact export is correctly layered in `std/jit`; it does not reach into private `IRProgram` fields.

### Documentation contract

- Public vars must have `:doc`, `:added`, `:ns`, and `:name` metadata unless explicitly private.
- `make docs-check` now treats documentation warnings as failures and runs generated-file, import-identity, explicit non-goal, and extracted-internal-package guardrails.

## Boundary concerns / maintenance risks

### 1. `core` is oversized

`core` has too many responsibilities for easy maintenance. The largest hand-maintained files should be treated as decomposition candidates:

- `core/procs.go` — many unrelated public procs in one file.
- `core/object.go` — remaining root runtime values (`Nil`, `Var`, `Proc`, `Fn`, `ExInfo`) and root-specific helpers; object/protocol/scalar/shared collection contracts have moved to `core/types`, while Atom/Channel/Future/Promise/Agent wrappers now live in `core/runtime`.
- `core/parse.go`, `core/read.go`, `core/eval.go` — acceptable for an interpreter but should remain isolated from feature-specific extensions.
- `core/types/ops_impl.go` and `core/types/numbers.go` — numeric contracts are now type-package owned and remain critical; keep focused tests around promotion, ratio, and native-int bounds.
- remaining root IR/WASM/executor files — partially extracted, but compiler/executor/runtime pieces still depend on root `Fn`/`Var`/`Expr`, namespace/frame, and call contracts.
- moved concrete collection families now live in `core/types/collections`; root `core` should not grow new collection-owned files.

Recommendation: keep collection ownership in `core/types/collections` and runtime wrapper ownership in `core/runtime`; use direct imports from root/runtime/generator code instead of root aliases. `tests/layout_guard.sh` now rejects reintroducing moved root collection files plus root channel/future/promise/agent/atom wrappers/literals. Current production collection and reader construction call sites use `corecollections.*`, and runtime-adjacent procs use `corert.*`; further root shrinking should focus on runtime/env/proc ownership, generated/bootstrap placement, and IR/WASM clusters rather than recreating shims. Continue adding feature files by responsibility (`*_ext.go`, `*_init.go`, `*_test.go`) rather than growing `procs.go`.

### 2. Runtime-installed Var metadata is implicit

The new metadata hygiene pass prevents doc warnings, but native namespace installers should still prefer explicit metadata at the install site. The hygiene pass should remain a safety net, not the primary documentation mechanism.

Recommendation: all new Go-installed public vars should use `InternVar(..., MakeMeta(...))` directly.

### 3. Generated files skew coverage and file-size metrics

`core/a_code.go` is generated and dominates line counts. Coverage should be interpreted in two tracks:

- generated/runtime aggregate coverage for CI trend visibility
- non-generated feature coverage for meaningful maintenance decisions

Current guardrail: `tests/generated_guard.sh` tracks generated root files plus generated data-package artifacts such as `core/generated/linter_payloads_gen.go`; `tests/coverage_summary.sh` filters generated files (`a_*.go`, `types_*_gen.go`, `gen_code/`) and separately reports gap-closure package coverage for `std/pods`, `std/transit`, and `std/edn`.

### 4. Some compatibility features need explicit contracts

Recent additions (`System`, `joker.transit`, `joker.edn`, `pods`/`babashka.pods`, `joker.jit/export-*`, `clojure.core.async`) should each document scope and non-goals:

- Transit is a practical Transit+JSON subset for pod/script payloads, not a full handler ecosystem yet.
- EDN reuses Joker reader/printer behavior without evaluation; options should expand only from fixture demand.
- Pods cover lifecycle/load/invoke/dynamic-vars and JSON/EDN/Transit payloads; deeper Babashka edge cases should be fixture-driven.
- System is JVM-shaped compatibility over Go/OS properties.
- core.async is goroutine-backed, not IOC-state-machine Clojure core.async.
- WASM export currently supports standalone pure numeric modules; object/collection host-import ABI is not a stable external contract yet.

## Coverage snapshot

Command used:

```sh
TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core ./std/... -coverprofile=/workspace/tmp/go-joker.cover \
  -covermode=atomic -timeout 120s -count=1
```

Current aggregate result:

- `core`: about 40% statement coverage in the Go package-level view.
- many `std/*` packages show low direct Go coverage because their public surface is exercised through generated namespace wrappers, parity scripts, docs generation, and CLI/integration tests rather than direct unit tests.
- recent features have focused tests:
  - `std/transit`: round-trip tests for maps, keywords, symbols, escaped strings.
  - `std/system`: properties, defaults, environment, time tests.
  - numeric promotion: overflow and BigFloat behavior tests.
  - benchmark result validation: semantic guardrails for benchmark workloads.
  - core.async: namespace aliases, go-loop/pipelines, map/filter/merge/split, mult/pub, reductions/callbacks.
  - HTTP persistent client: connection reuse and options.

## Coverage gaps to prioritize

1. Continue adding `std` native namespace smoke tests for packages with low direct coverage. Minimal boundary tests now exist for `crypto`, `math`, `string`, and `uuid`; remaining candidates include packages whose behavior is still mostly covered through namespace/docs/integration paths.
2. Numeric tower edge cases:
   - BigInt mixed with Ratio/Double/BigFloat
   - quotient/remainder semantics across sign combinations
   - BigFloat divide-by-zero and precision stability
3. IR/WASM export/import contract tests:
   - verify `.ir` schema stability
   - instantiate exported `.wasm` with wazero in a test and call `exec`
4. Documentation generation warning guard is now in place; add a CI artifact only if warning-free logs need archiving.
5. More direct tests for namespace registration boundaries: each new `std/*` namespace should have at least one CLI-level smoke test or package test that verifies public var resolution.

## Best-practice rules going forward

- Keep new namespaces small: adapter file, native file, tests, `.joke` docs.
- Do not add unrelated runtime features to `procs.go`; create a focused file with a clear suffix.
- Prefer exported bridge functions over cross-package access to private runtime structs.
- Treat warnings as failures; `make docs-check` now enforces this for docs.
- Every new public namespace var must have doc and added metadata at install time.
- Every new feature should include:
  - package/native unit test
  - namespace smoke test if registration matters
  - docs generation coverage
  - parity/regression test when behavior mimics Clojure/Babashka/let-go

## Immediate follow-up recommendations

- Add a generated-file-aware coverage summary target/script.
- Keep adding small direct tests for low-risk std namespaces with low Go coverage, following the recent `crypto`/`math`/`string`/`uuid` boundary-test pattern.
- Add WASM artifact execution test to close the loop on `jit/export-wasm`.
- Consider splitting `numbers.go` tests into a dedicated numeric tower test suite as semantics deepen.
