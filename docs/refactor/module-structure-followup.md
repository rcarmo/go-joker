# Module structure follow-up audit

Updated: 2026-05-15

## Snapshot

A second pass over the Go package layout confirms the previous findings and makes the next improvements more concrete.

Current package/file snapshot:

| Package/area | Go files | Test files | Notes |
|---|---:|---:|---|
| root | 0 | 0 | clean: no root package remains |
| `cmd/joker` | 19 | 3 | startup/orchestration is now split across cohesive helper files with focused arg/lint/compile tests; keep shrinking `main.go` rather than regrowing it |
| `core` root | 136 | 29 | still the main monolith; many tiny tests were consolidated and benchmarks moved to `benchmarks/core`; generated files and still-coupled runtime/object code remain in root |
| `core/generated` | 11 | 2 | data-only generated bootstrap contract, source manifest, linter byte payloads, and generated linter payload registry |
| `core/ir` | 13 | 5 | opcodes, disassembly/counting, shape analysis, neutral Program model, and IR leaf helpers |
| `core/trace` | 6 | 2 | aggregation state with direct JSON-shape tests |
| `core/wasm` | 13 | 5 | encoding/module/host/opcode/value leaf helpers |
| `core/runtime` | 11 | 5 | feature flags and runtime leaf helpers live here; executor/object-bound runtime code remains in root `core` |
| `core/collections` | 9 | 4 | real mechanics package for generic slice storage, pair arrays, bitmap/hash-index helpers, and opaque trie nodes; root keeps Object/protocol adapters |
| `core/reader` | 39 | 19 | reader leaf helpers now include chars, whitespace/comment/line decisions, identifier classification/token scanning/validation issue scans, unicode/string escapes, number-token classification, dispatch/character/form helpers, line rune reader, and raw IO mechanics; parser/object implementation remains root-bound |
| `core/string` | 39 | 19 | real helper package for root-independent string mechanics; root keeps Object/error wrappers |
| `core/cursor` | 3 | 1 | real leaf package for string cursor mechanics; root keeps Object protocol adapter |
| `std/*` | many small packages | mixed | namespace-oriented; now explicitly documented as `std/<namespace>.joke` + `std/<namespace>/*.go` + `std/<namespace>/<subns>/...` |
| `benchmarks` | 1 | 0 | benchmark data/fixtures remain here; report generators should not live here anymore |
| `tools/benchmarks` | 4 | 0 | build-ignore report/chart generators and Goja helper moved out of `benchmarks/` |

Root `core` clustering remains the structural hotspot:

| Family | Root files |
|---|---:|
| `ir*.go` | about 46 |
| `wasm*.go` | about 15 |
| generated `a_*.go` | 10 |
| `types_*_gen.go` | 2 |
| vector-related | 4 |
| map-related | 7 |
| read/reader/parse/eval | 8 |
| root tests | 29 |

## Confirmed improvements to add to the plan

### 1. Split `IRProgram` into model plus execution metadata

The highest-value next boundary remains IR ownership. `core/ir` cannot own the program yet because root `IRProgram` still contains runtime-specific fields:

- `[]Object` constants and capture slots
- `bindingKey` capture keys
- `*EscapeInfo`
- `nativeF64Fn` helpers
- `*FnExpr`
- execution failure/cache flags

Status:

- `core/ir.Program` now exists for bytecode, constants count, slot counts, op metadata, and analysis.
- diagnostics/export accessors, WASM lowering helpers, and native helper compilation consume the neutral model where appropriate.
- root-only execution metadata remains in `core` until call/object contracts are explicit in code.

### 2. Direct tests for `core/trace`

`core/trace` now has package tests for:

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

Go packages are directory-scoped, so generated files that still declare `package core` cannot be moved to a subdirectory as a cosmetic cleanup without becoming a different package and breaking the build. The generated manifest guard is working and `core/generated` now owns data-only bootstrap payload contracts plus the generated source manifest. Do not move root generated runtime mutation merely for tidiness until broader payload equivalence and runtime consumers exist.

Status:

- generated bootstrap contract is documented;
- source manifest emission and linter payload registry emission have started;
- root `ProcessLinterData` consumes linter payloads through `core/generated.LinterDataByPath`;
- `make generated-bootstrap-check` guards manifest equivalence with current root `coreNamespaces`, and generated package tests guard linter registry/manifest equivalence.

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
  generated/
  ir/
  trace/
  wasm/
  runtime/
  collections/
  reader/
  string/
  cursor/

std/
  <namespace>.joke
  <namespace>/*.go
  <namespace>/<subns>/...

benchmarks/
  core/                         # Go benchmark harnesses kept out of root core

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
3. Generated payload conversion into real generated packages when equivalence is proven. **In progress: source manifest plus linter registry now have real `core/generated` package boundaries.**
4. Collection construction adapters before `core/collections` moves. **Done for current production callers: `CollectionConstructionAdapter` plus a boundary guard are in place. Pure vector/map storage mechanics have started moving; concrete Object/protocol implementations remain root-bound.**
5. Reader construction adapters before `core/reader` moves. **Done for current production callers and unpacked expression construction: `ReaderConstructionAdapter` plus a boundary guard are in place. Pure lexical/token helpers have started moving; tagged literals/object/parser orchestration remain root-bound.**
6. Low-priority tooling moves such as `tools/benchmarks`, `tools/codegen`, `tools/scripts`, and `tools/release` last, once references and CI paths are updated.

## Recommended immediate next steps

1. Continue narrowing runtime call/object/frame access behind `RuntimeExecutionAdapter`; keep `core/runtime` as the reserved extraction target, but do not move executor files until those seams are explicit and tested.
2. Continue collection extraction by moving only root-independent mechanics into `core/collections`; do not force concrete vector/map/set/seq implementations across the package boundary while they depend on root Object/protocol/equality/hash behavior.
3. Continue reader extraction by moving lexical/token helpers into `core/reader`; keep concrete Object construction, FORMAT/LINTER behavior, namespace resolution, tagged literals, and parser/evaluator handoff in root until adapter contracts are explicit.
4. Extend generated bootstrap emission beyond the source manifest and linter registry only with broader equivalence tests.
5. Keep audit hardening in the validation cadence (`staticcheck-sa`, `vuln`, vet, race tests for leaf packages) because recent passes found real HTTP/WASM/osutil edge cases.
6. Keep WASM leaf extraction opportunistic, but avoid moving runtime/object-handle paths until execution metadata is explicit.
7. Keep std resource namespaces under the documented `std/<namespace>/<subns>/...` tree and reject any return to loose `lib/` placement.
8. Keep transient root build artifacts (`core.test`, `joker`, `transit.test`) out of the repository root; `layout-check` should fail when they reappear.
