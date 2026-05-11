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
core/internal/trace               # extracted trace/profile aggregation state
core/internal/ir                  # extracted IR opcode/diagnostic/analysis helpers
core/internal/wasm                # extracted WASM encoding/module/host metadata helpers
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
  - `make non-goals-check`
  - `make refactor-internals-check`
  - `make core-contract-check`
- Refactor documents consolidated under `docs/refactor/`.
- Leaf packages extracted under `core/internal/{trace,ir,wasm}`.

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

### 2. Generated bootstrap remains root-coupled

Generated files still live in root `core` because they instantiate core runtime values directly. The manifest guard now tracks this, but the structural target remains:

```text
core/internal/generated/          # future generated/bootstrap package or directory
```

Prerequisites remain those in `generated-boundary.md`: explicit runtime/object initialization contracts and generator import path updates.

### 3. IR package is not yet the owner of `IRProgram`

`core/internal/ir` owns opcode/constants/analysis helpers, but `IRProgram` still lives in root `core` because it references root-only types (`Object`, `FnExpr`, `bindingKey`, `EscapeInfo`, native helper funcs).

Next improvement:

- define a minimal `ir.Program` representation that excludes root-only runtime fields, or
- split `IRProgram` into `ir.Program` plus root `core` execution metadata.

This is the highest-value next architectural move before broad collection/reader splits.

### 4. WASM lowering/runtime still follows `IRProgram`

`core/internal/wasm` now owns leaf binary/module/host metadata, but lowering/runtime files remain root-coupled.

Next improvement:

- move more pure WASM bytecode helpers/constants as they are identified;
- delay full lowering/runtime move until IR program ownership is clarified.

### 5. Collections need a public object/protocol contract before moving

The new `core-contract-check` is a good start, but the collection package still needs explicit construction, equality/hash, seq, metadata, and transient contracts.

Next improvement:

- expand contract tests for `MapSet`, seqs, transients, and sorted collections;
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

1. Continue IR split by designing `ir.Program` vs root execution metadata.
2. Add more `core-contract-check` coverage for sets, seqs, transients, and sorted collections.
3. Keep extracting pure WASM helpers but avoid moving runtime/lowering until IR ownership is clearer.
4. Update generators only after generated bootstrap contracts are explicit.
5. Consider moving build-tagged benchmark generators into `tools/benchmarks` after core refactor checkpoints.

## Outdated content removed or superseded

- Old root-level refactor audit filenames have been removed in favor of `docs/refactor/*`.
- Early statements that tracing was only a future extraction target are superseded by extracted `core/internal/trace`.
- Early `core/internal/wasm or core/wasm` ambiguity is superseded by `core/internal/wasm` for leaf helpers.
