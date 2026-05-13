# Module structure audit

Updated: 2026-05-11

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
core/generated           # data-only generated bootstrap payload contracts/source manifest
core/internal/ir                  # extracted IR opcode/diagnostic/analysis helpers and neutral Program model
core/wasm                # extracted WASM encoding/module/host metadata helpers
std/*                             # namespace-oriented standard library packages
tests                             # integration/parity/Babashka fixture tests
benchmarks                        # benchmark/report tooling
tools/sum256dir                   # small standalone helper
```

Approximate Go file counts at this audit:

| Area | Go files | Notes |
|---|---:|---|
| `core` total | ~205 | includes tests and extracted internals |
| `core` root | ~185 | still too broad; largest remaining target |
| `std` | ~116 | mostly healthy namespace-oriented packages |
| `cmd/joker` | 7 | clean CLI package after root move |
| `tests` | 2 | integration harnesses and fixtures live under subdirs |
| `benchmarks` | 7 | mixed benchmark helpers, most build-tagged ignore |

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
- Leaf packages extracted under `core/internal/{generated,trace,ir,wasm}`.

## Remaining structural issues

### 1. Root `core` is still too large

`core` root still mixes object model, collections, reader/parser, evaluator, runtime, IR compiler/executor, WASM lowering/runtime, generated bootstrap, and benchmarks/tests.

Current root clustering by filename indicates the next logical seams:

- `ir*.go`: compiler/executor/cache/export adapters still mostly root-coupled.
- `wasm*.go`: lowering/runtime still depends on root `IRProgram` and object handles.
- collection files: `array_*`, `vector`, `persistent_vector`, `map`, `hash_map`, `set`, `seq`, `transient`.
- reader/parser/evaluator: `read`, `reader`, `parse`, `eval`, `expr`, `tco`.
- runtime/calls: `call_fast`, `goroutine_rt`, frame and dispatch helpers.
- generated bootstrap: `a_*.go`, `types_*_gen.go`.

Recommendation: continue extracting pure leaf helpers and contracts before moving high-cycle packages.

### 2. Generated bootstrap remains partly root-coupled

Generated root files still live in root `core` because they instantiate core runtime values directly. The generated-file manifest guard tracks this. The target package now exists:

```text
core/generated/          # data-only generated/bootstrap contracts and source manifest
```

Current progress:

- `NamespaceSource` and `VarDoc` define the inert data-only bootstrap contract.
- `core_sources_gen.go` is emitted under `core/generated`.
- `make generated-bootstrap-check` compares the generated source manifest with current root `coreNamespaces`.

Remaining prerequisites are in `generated-boundary.md` and `generated-bootstrap-contract.md`: broader equivalence tests, root runtime consumers for generated payloads, and generator import path updates that avoid exporting root runtime internals.

### 3. IR package owns neutral shape, root owns execution metadata

`core/internal/ir` now owns opcode/constants/analysis helpers and a neutral `Program` model. Root `IRProgram` remains the executable envelope because it references root-only types (`Object`, `FnExpr`, `bindingKey`, `EscapeInfo`, native helper funcs). Diagnostics, exported accessors, WASM lowering helpers, and native helper compilation now consume the neutral model where appropriate.

Next improvement:

- keep executor and escape-analysis root-bound until the runtime execution contract becomes code;
- add focused tests for constants/captures, `irMakeFn`, failure caches, and escape-analysis metadata before moving executors.

This is the highest-value next architectural move before broad collection/reader splits.

### 4. WASM lowering/runtime still follows root execution metadata

`core/wasm` now owns leaf binary/module/host metadata, and most WASM eligibility/lowering paths read neutral IR shape. Runtime instantiation, constants, object handles, and memory/import bridges remain root-coupled.

Next improvement:

- move only pure WASM bytecode helpers/constants as they are identified;
- delay full runtime/lowering move until execution metadata and object-handle contracts are explicit.

### 5. Collections need a public object/protocol contract before moving

`core-contract-check` now covers vectors, associative maps, sets, transients, seqs, persistent-vector semantics, and info/meta behavior. The collection package still needs explicit construction APIs and any sorted-collection contracts before concrete files move.

Next improvement:

- add sorted collection/construction contracts if those types are migration candidates;
- then move concrete collection files behind those tests.

### 6. Reader/parser/evaluator should move late

These layers are highly coupled to object construction, namespaces, errors, and evaluator state.

Next improvement:

- keep reader/evaluator in root until object construction and tagged literal contracts are explicit;
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

1. Keep executor/escape-analysis root-bound until the runtime execution contract becomes code.
2. Extend generated bootstrap emission beyond the source manifest only with broader equivalence tests.
3. Add sorted collection/construction contracts if those types become migration candidates.
4. Keep extracting pure WASM helpers but avoid moving runtime/lowering until execution metadata and object handles are explicit.
5. Consider moving build-tagged benchmark generators into `tools/benchmarks` after core refactor checkpoints.

## Outdated content removed or superseded

- Old root-level refactor audit filenames have been removed in favor of `docs/refactor/*`.
- Early statements that tracing was only a future extraction target are superseded by extracted `core/trace`.
- Early `core/wasm or core/wasm` ambiguity is superseded by `core/wasm` for leaf helpers.
