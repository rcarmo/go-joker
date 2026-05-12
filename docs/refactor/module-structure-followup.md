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
| `core/internal/generated` | 3 | 1 | data-only generated bootstrap contract and source manifest |
| `core/internal/ir` | 5 | 2 | opcodes, disassembly/counting, shape analysis, neutral Program model |
| `core/internal/trace` | 4 | 1 | aggregation state with direct JSON-shape tests |
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

The generated manifest guard is working and `core/internal/generated` now owns data-only bootstrap payload contracts plus the generated source manifest. Do not move root generated runtime mutation merely for tidiness until broader payload equivalence and runtime consumers exist.

Status:

- generated bootstrap contract is documented;
- source manifest emission has started;
- `make generated-bootstrap-check` guards manifest equivalence with current root `coreNamespaces`.

### 5. Move benchmark report generators later

`benchmarks/` is lower priority but still noisy in `go list ./...`. Build-tagged report generators can eventually move to:

```text
tools/benchmarks/
```

Improvement to plan:

- defer until core package boundaries settle.

## Recommended immediate next steps

1. Keep executor/escape-analysis root-bound until the runtime execution contract becomes code.
2. Extend generated bootstrap emission beyond the source manifest only with broader equivalence tests.
3. Add construction/sorted collection contracts only if those types become migration candidates.
4. Keep WASM leaf extraction opportunistic, but avoid moving runtime/object-handle paths until execution metadata is explicit.
