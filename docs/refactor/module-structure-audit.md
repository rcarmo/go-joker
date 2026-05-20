# Module structure audit

Updated: 2026-05-15

## Scope

This audit reviews the current Go module/package layout after the first refactor batches. It focuses on package boundaries, directory shape, and remaining high-value structural improvements.

## Current package shape

Module identity is now:

```text
github.com/rcarmo/go-joker
```

Current major packages:

```text
cmd/joker                         # CLI, REPL, standalone helpers
core                              # runtime kernel; still the main monolith
core/trace               # extracted trace/profile aggregation state
core/generated           # data-only generated bootstrap payload contracts/source manifest/linter registry
core/ir                  # extracted IR opcode/diagnostic/analysis helpers and neutral Program model
core/runtime             # extracted runtime feature flags and leaf helpers
core/wasm                # extracted WASM encoding/module/host metadata helpers
core/types/collections         # extracted collection storage/bitmap/trie mechanics
core/reader              # extracted reader lexical/token/scanning/form/IO mechanics
core/types/string              # extracted string mechanics
core/types/string              # extracted string cursor mechanics
std/*                             # namespace-oriented standard library packages
tests                             # integration/parity/Babashka fixture tests
benchmarks                        # benchmark/report tooling
tools/sum256dir                   # small standalone helper
```

Approximate Go file counts at this audit:

| Area | Go files | Notes |
|---|---:|---|
| `core` total | ~240 | includes tests and extracted packages |
| `core` root | 70 | still too broad; generated bootstrap, root contract tests, executor/IR/WASM families, and fast-path helper families are mechanically grouped; generated files and still-coupled runtime/object code remain here |
| `std` | ~116 | mostly healthy namespace-oriented packages, with more direct boundary tests |
| `cmd/joker` | 22 | clean CLI package after root move, now split across cohesive files plus focused tests |
| `tests` | 2 | integration harnesses and fixtures live under subdirs |
| `benchmarks` | 7 | benchmark harnesses now live under `benchmarks/core`; report tooling should stay under `tools/benchmarks` |

## Improvements already made

- Root package eliminated: no top-level `.go` files remain.
- CLI moved to `cmd/joker`.
- Internal imports/module identity moved to `github.com/rcarmo/go-joker`.
- Guardrails added:
  - `make layout-check`
  - `make import-identity-check`
  - `make generated-check`
  - `make generated-bootstrap-check`
  - `make non-goals-check`
  - `make refactor-internals-check`
  - `make core-contract-check`
- Refactor documents consolidated under `docs/refactor/`.
- Leaf/data packages extracted under `core/{generated,trace,ir,runtime,wasm,collections,reader,string,cursor}`.
- Collection and reader construction adapters plus boundary guards now prevent new direct construction drift before package moves.
- Root-independent collection storage/bitmap/trie mechanics and a broad set of reader lexical/token/scanning/form/IO mechanics have moved to real packages without importing root `core`.

## Remaining structural issues

### 1. Root `core` is still too large

`core` root still mixes object model, concrete collections, reader/parser orchestration, evaluator, runtime, IR compiler/executor, WASM lowering/runtime, generated bootstrap, and tests. Go benchmarks have moved to `benchmarks/core` and are guarded against returning to root `core`.

Current root clustering by filename indicates the next logical seams:

- `ir*.go`: compiler/executor/cache/export adapters still mostly root-coupled.
- `wasm*.go`: lowering/runtime still depends on root `IRProgram` and object handles.
- collection files: `array_*`, `vector`, `persistent_vector`, `map`, `hash_map`, `set`, `seq`, `transient`.
- reader/parser/evaluator: `read`, `reader`, `parse`, `eval`, `expr`, `tco`.
- runtime/calls: `call_fast`, `goroutine_rt`, frame and dispatch helpers.
- generated bootstrap: grouped `a_generated_bootstrap_payloads.go`, `types_*_gen.go`.

Recommendation: continue extracting pure leaf helpers and contracts before moving high-cycle packages, but use mechanical grouping where root-coupled files cannot yet move to a real package. Current root `core` file counts are 70 total Go files, 69 non-test files, 3 generated root files, and 66 hand-written non-test/non-generated root files.

### 2. Generated bootstrap remains partly root-coupled

Generated root files still live in root `core` because they instantiate core runtime values directly. The generated-file manifest guard tracks this. The target package now exists:

```text
core/generated/          # data-only generated/bootstrap contracts and source manifest
```

Current progress:

- `NamespaceSource` and `VarDoc` define the inert data-only bootstrap contract.
- `core_sources_gen.go` is emitted under `core/generated`.
- `linter_payloads_gen.go` is emitted under `core/generated` and root `ProcessLinterData` consumes it via `LinterDataByPath`.
- `make generated-bootstrap-check` compares the generated source manifest with current root `coreNamespaces`; generated package tests compare the linter payload registry with manifest linter entries.

Remaining prerequisites are in `generated-boundary.md` and `generated-bootstrap-contract.md`: broader equivalence tests for additional payload families, root runtime consumers for those payloads, and generator import path updates that avoid exporting root runtime internals.

### 3. IR package owns neutral shape, root owns execution metadata

`core/ir` now owns opcode/constants/analysis helpers and a neutral `Program` model. Root `IRProgram` remains the executable envelope because it references root-only types (`Object`, `FnExpr`, `bindingKey`, `EscapeInfo`, native helper funcs). Diagnostics, exported accessors, WASM lowering helpers, and native helper compilation now consume the neutral model where appropriate.

Next improvement:

- keep executor and escape-analysis root-bound until the runtime execution contract becomes narrow enough for real package moves;
- continue adding focused tests for call/object/frame behavior before moving executors. Constants/captures, `irMakeFn`, failure caches, native-helper/fallback state, and escape-analysis metadata already have contract coverage.

This is the highest-value next architectural move before broad collection/reader splits.

### 4. WASM lowering/runtime still follows root execution metadata

`core/wasm` now owns leaf binary/module/host metadata, and most WASM eligibility/lowering paths read neutral IR shape. Runtime instantiation, constants, object handles, and memory/import bridges remain root-coupled.

Next improvement:

- move only pure WASM bytecode helpers/constants as they are identified;
- delay full runtime/lowering move until execution metadata and object-handle contracts are explicit.

### 5. Collections need a public object/protocol contract before moving

`core-contract-check` now covers vectors, associative maps, sets, transients, seqs, persistent-vector semantics, and info/meta behavior. The collection package now has an explicit construction adapter and guard for current production call sites, but concrete implementation moves still need acyclic object/protocol implementation boundaries.

Next improvement:

- keep construction and sorted collection contracts green;
- then move concrete collection files only when object/protocol dependencies are explicit and acyclic.

### 6. Reader/parser/evaluator should move late

These layers are highly coupled to object construction, namespaces, errors, and evaluator state.

Next improvement:

- keep reader/evaluator in root until object construction, expression construction, tagged literal, and evaluator handoff contracts are explicit and acyclic;
- avoid new feature code in `parse.go`, `read.go`, or `eval.go` unless it is truly core language behavior.

### 7. Benchmark/tooling layout can be cleaner

`benchmarks/` contains both benchmark support and build-tagged report generators. This works, but the target layout could be clearer:

```text
benchmarks/                       # benchmark data/scripts/results
tools/benchmarks/                 # Go report/chart generators
tools/tracing/                    # trace/profile renderers if moved from skills/docs
```

This is lower priority than `core`, but it would reduce package noise in `go list ./...`.

## Recommended next actions

1. Keep executor/escape-analysis root-bound until the runtime execution contract becomes narrow enough for real package moves.
2. Extend generated bootstrap emission beyond the source manifest and linter registry only with broader equivalence tests.
3. Keep collection and reader construction boundary guards green before attempting package moves.
4. Keep extracting pure WASM helpers but avoid moving runtime/lowering until execution metadata and object handles are explicit.
5. Continue low-priority tooling cleanup only after core/package boundary changes have updated references and CI paths.

## Outdated content removed or superseded

- Old root-level refactor audit filenames have been removed in favor of `docs/refactor/*`.
- Early statements that tracing was only a future extraction target are superseded by extracted `core/trace`.
- Early `core/wasm or core/wasm` ambiguity is superseded by `core/wasm` for leaf helpers.
