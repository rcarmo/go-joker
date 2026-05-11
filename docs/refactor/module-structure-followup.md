# Module structure follow-up audit

Updated: 2026-05-11

## Snapshot

A second pass over the Go package layout confirms the previous findings and makes the next improvements more concrete.

Current package/file snapshot:

| Package/area | Go files | Test files | Notes |
|---|---:|---:|---|
| root | 0 | 0 | clean: no root package remains |
| `cmd/joker` | 7 | 0 | CLI/REPL/standalone helpers are grouped correctly |
| `core` root | 185 | 49 | still the main monolith |
| `core/internal/ir` | 4 | 1 | opcodes, disassembly/counting, shape analysis |
| `core/internal/trace` | 3 | 0 | aggregation state only; no direct tests yet |
| `core/internal/wasm` | 8 | 4 | encoding/module/host/opcode leaf helpers |
| `std/*` | many small packages | mixed | mostly namespace-oriented and healthy |
| `benchmarks` | 5 | 0 | still mixes package stub and build-tagged report tools |

Root `core` clustering remains the structural hotspot:

| Family | Root files |
|---|---:|
| `ir*.go` | 42 |
| `wasm*.go` | 14 |
| generated `a_*.go` | 17 |
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

Improvement to plan:

- introduce a small `core/internal/ir.Program` model for bytecode, constants count, slot counts, op metadata, and analysis;
- keep root-only execution metadata in `core` until call/object contracts are explicit.

### 2. Add direct tests for `core/internal/trace`

`core/internal/trace` is extracted but currently has no package tests. It should get direct tests for:

- function tracer event/edge aggregation
- symbol resolve/deref aggregation
- IR profile op/edge aggregation
- JSON output shape via temp files

Improvement to plan:

- add `core/internal/trace` unit tests before further trace adapter changes.

### 3. Expand object/protocol contract tests before collection moves

Current contract tests cover vectors and maps. Before moving collections, add tests for:

- `MapSet` equality/hash/seq behavior
- seq implementations (`List`, vector seqs, lazy/filter/take seqs where practical)
- transients (`TransientVector`, transient map side table behavior)
- sorted collections if they are candidates for extraction

Improvement to plan:

- extend `make core-contract-check` to cover set/seq/transient contracts.

### 4. Keep generated files root-bound until bootstrap contract exists

The generated manifest guard is working. Do not move generated files merely for tidiness until there is a bootstrap API that does not reach into root `core` internals.

Improvement to plan:

- design generated bootstrap contract before generator path changes.

### 5. Move benchmark report generators later

`benchmarks/` is lower priority but still noisy in `go list ./...`. Build-tagged report generators can eventually move to:

```text
tools/benchmarks/
```

Improvement to plan:

- defer until core package boundaries settle.

## Recommended immediate next steps

1. Add `core/internal/trace` package tests.
2. Start an `ir.Program` model design note or minimal type extraction.
3. Expand `core-contract-check` with set/seq/transient contracts.
4. Keep WASM leaf extraction opportunistic, but avoid moving lowering/runtime until IR ownership is clearer.
