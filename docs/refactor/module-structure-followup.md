# Module structure follow-up audit

Updated: 2026-05-13

## Snapshot

A second pass over the Go package layout confirms the previous findings and makes the next improvements more concrete.

Current package/file snapshot:

| Package/area | Go files | Test files | Notes |
|---|---:|---:|---|
| root | 0 | 0 | clean: no root package remains |
| `cmd/joker` | 14 | 0 | startup/orchestration is now split across cohesive helper files; keep shrinking `main.go` rather than regrowing it |
| `core` root | 201 | 64 | still the main monolith; contract tests grew intentionally before moves |
| `core/internal/generated` | 3 | 1 | data-only generated bootstrap contract and source manifest; package-level generated payloads only move here when they can be a true Go package boundary |
| `core/internal/ir` | 6 | 2 | opcodes, disassembly/counting, shape analysis, neutral Program model |
| `core/internal/trace` | 4 | 1 | aggregation state with direct JSON-shape tests |
| `core/internal/wasm` | 8 | 4 | encoding/module/host/opcode leaf helpers |
| `core/runtime` | 1 | 0 | reserved package for evaluator/runtime extraction; currently a documented boundary marker only |
| `core/collections` | 1 | 0 | reserved package for collection extraction; currently a documented boundary marker only |
| `core/reader` | 1 | 0 | reserved package for reader/parser extraction; currently a documented boundary marker only |
| `core/ir` | 1 | 0 | reserved package for future IR compiler/executor extraction; neutral leaf model remains in `core/internal/ir` |
| `core/wasm` | 1 | 0 | reserved package for future root WASM extraction; leaf helpers remain in `core/internal/wasm` |
| `core/generated` | 1 | 0 | reserved package for future generated payload families that become real package boundaries |
| `core/string` | 1 | 0 | reserved package for future extraction of root string support helpers |
| `std/*` | many small packages | mixed | namespace-oriented; now explicitly documented as `std/<namespace>.joke` + `std/<namespace>/*.go` + `std/<namespace>/<subns>/...` |
| `benchmarks` | 1 | 0 | benchmark data/fixtures remain here; report generators should not live here anymore |
| `tools/benchmarks` | 4 | 0 | build-ignore report/chart generators and Goja helper moved out of `benchmarks/` |

Root `core` clustering remains the structural hotspot:

| Family | Root files |
|---|---:|
| `ir*.go` | 46 |
| `wasm*.go` | 15 |
| generated `a_*.go` | 16 |
| `types_*_gen.go` | 2 |
| vector-related | 4 |
| map-related | 7 |
| read/reader/parse/eval | 8 |
| root tests | 49 |

## Confirmed improvements to add to the plan

### 1. Split `IRProgram` into model plus execution metadata

The highest-value next boundary remains IR ownership. `core/internal/ir` cannot own the program yet because root `IRProgram` still contains runtime-specific fields:

- `[]Object` constants and capture slots
- `bindingKey` capture keys
- `*EscapeInfo`
- `nativeF64Fn` helpers
- `*FnExpr`
- execution failure/cache flags

Status:

- `core/internal/ir.Program` now exists for bytecode, constants count, slot counts, op metadata, and analysis.
- diagnostics/export accessors, WASM lowering helpers, and native helper compilation consume the neutral model where appropriate.
- root-only execution metadata remains in `core` until call/object contracts are explicit in code.

### 2. Direct tests for `core/internal/trace`

`core/internal/trace` now has package tests for:

- function tracer event/edge aggregation
- symbol resolve/deref aggregation
- IR profile op/edge aggregation
- JSON output shape via temp files

Further trace adapter changes should keep extending these direct tests rather than relying only on root-core integration tests.

### 3. Expand object/protocol contract tests before collection moves

Current contract tests cover vectors and maps. Before moving collections, add tests for:

- `MapSet` equality/hash/seq behavior
- seq implementations (`List`, vector seqs, lazy/filter/take seqs where practical)
- transients (`TransientVector`, transient map side table behavior)
- sorted collections if they are candidates for extraction

Status:

- `make core-contract-check` now covers set, seq, and transient contracts in addition to vector/map/info/meta coverage.
- Remaining collection planning should focus on construction APIs and sorted collection contracts only if they become migration candidates.

### 4. Keep generated runtime mutation root-bound until broader bootstrap equivalence exists

Go packages are directory-scoped, so generated files that still declare `package core` cannot be moved to a subdirectory as a cosmetic cleanup without becoming a different package and breaking the build. The generated manifest guard is working and `core/internal/generated` now owns data-only bootstrap payload contracts plus the generated source manifest. Do not move root generated runtime mutation merely for tidiness until broader payload equivalence and runtime consumers exist.

Status:

- generated bootstrap contract is documented;
- source manifest emission has started;
- `make generated-bootstrap-check` guards manifest equivalence with current root `coreNamespaces`.

### 5. Move benchmark report generators later

This cleanup has now started: build-ignore report generators and the Goja helper belong under `tools/benchmarks/`, while `benchmarks/` should keep benchmark data, fixtures, and documentation.

## Desired staged tree

The intended medium-term structure is:

```text
cmd/joker/
  main.go
  args.go
  compile.go
  files.go
  lint.go
  profile.go
  repl*.go
  standalone.go

core/
  *.go                         # only code that still genuinely belongs to root core
  internal/generated/
  internal/ir/
  internal/trace/
  internal/wasm/
  runtime/
  collections/
  reader/

std/
  <namespace>.joke
  <namespace>/*.go
  <namespace>/<subns>/...

tools/
  benchmarks/
  codegen/
  scripts/
  release/
```

Where `tools/scripts/` owns auxiliary build/test/lint helper scripts that do not need to live in the repository root, `tools/release/` owns release automation that should not sit at top level, and `tools/codegen/` owns standalone code-generation helpers that do not belong in the source roots. Historical/reference documents should also move under `docs/` rather than staying in the root.

This has now started with `docs/PERFORMANCE_PLAN.md`, `docs/archive/ORIGINAL_README.md`, `docs/LIBRARIES.md`, `docs/DEVELOPER.md`, and `docs/licenses/epl-v10.html`.

Staged migration order should remain:

1. `cmd/joker` split while staying in one package.
2. Runtime/executor adapter narrowing for `core/runtime`.
3. Generated payload conversion into real generated packages when equivalence is proven.
4. Collection construction adapters before `core/collections` moves.
5. Reader construction adapters before `core/reader` moves.
6. Low-priority tooling moves such as `tools/benchmarks`, `tools/codegen`, `tools/scripts`, and `tools/release` last, once references and CI paths are updated.

## Recommended immediate next steps

1. Continue moving nested-call, fallback, and execution-state access behind `RuntimeExecutionAdapter`; keep `core/runtime` as the reserved extraction target, but do not move executor files until those seams are explicit and tested.
2. Extend generated bootstrap emission beyond the source manifest only with broader equivalence tests.
3. Define collection construction/adaptation contracts before moving collection implementations into the reserved `core/collections` target.
4. Define reader object/expression construction contracts before moving reader/parser code into the reserved `core/reader` target.
5. Keep WASM leaf extraction opportunistic, but avoid moving runtime/object-handle paths until execution metadata is explicit.
6. Keep std resource namespaces under the documented `std/<namespace>/<subns>/...` tree and reject any return to loose `lib/` placement.
7. Keep transient root build artifacts (`core.test`, `joker`, `transit.test`) out of the repository root; `layout-check` should fail when they reappear.
