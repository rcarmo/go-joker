# Core package split audit

Updated: 2026-05-11

## Purpose

This is the R5 inventory for splitting the remaining `core` monolith after the initial trace and IR helper packages. The aim is clean architectural grouping, not compatibility wrappers. Breaking internal package paths are acceptable once boundaries are explicit.

## Current problem

`core/` still contains several unrelated implementation families in one package:

- runtime object model
- persistent collections
- reader/parser
- evaluator/compiler forms
- IR/WASM/JIT execution paths
- concurrency/channel runtime
- namespace/bootstrap/generated code
- tracing adapters
- tests and benchmarks

This makes it too easy for features to reach across layers through unexported state and makes architectural intent hard to see from the repository layout.

## Proposed split order

Do not split everything at once. Move leaf or low-cycle families first, then higher-coupling layers.

### 1. Already started

- `core/internal/trace` owns tracing/profiling aggregation state.
- `core/internal/ir` owns opcode names/constants, bytecode disassembly/counting, and shape analysis.
- `cmd/joker` owns the CLI entrypoint.

### 2. Runtime/object boundary

Candidate future package:

```text
core/internal/runtime/
```

Likely contents or responsibilities:

- goroutine runtime bookkeeping
- eval frame stack helpers
- call dispatch scaffolding that is not IR-specific
- panic/error helpers only after object/error contracts are clear

Current candidate files:

- `goroutine_rt.go`
- frame/call scaffolding currently mixed through evaluator and IR files
- selected parts of `call_fast.go`, only after call contracts are explicit

Risks:

- runtime currently touches `Object`, `Fn`, `Proc`, `Var`, namespaces, and evaluator state.
- moving too early can create cycles with `core` unless object interfaces are extracted first.

### 3. Collections boundary

Candidate future package:

```text
core/internal/collections/
```

Current candidate files:

- `array_map.go`
- `array_vector.go`
- `chunked_seq.go`
- `hash_map.go`
- `list.go`
- `map.go`
- `persistent_vector.go`
- `seq.go`
- `set.go`
- `sorted_colls.go`
- `transient.go`
- `transient_string.go`
- `vector.go`
- related fast paths such as `reduce_fast.go`, `seq_ops_fast.go`, `range_fast.go` once interfaces are clear

Risks:

- collection types are part of the public runtime object model.
- reader/evaluator/std packages construct concrete collection types directly.
- numeric/hash/equality/protocol behavior crosses package boundaries.

Preferred prerequisite:

- define object/protocol contracts first, then move concrete collections behind those contracts.

### 4. Reader/parser boundary

Candidate future package:

```text
core/internal/reader/
```

Current candidate files:

- `reader.go`
- `read.go`
- `read_conditional_test.go`
- `rune_window.go`
- `line_runereader.go`
- `buffered_reader.go`
- `tagged_literals.go`
- parser-adjacent pieces in `parse.go`, `parse_slow_init.go`

Risks:

- reader currently constructs concrete `core` objects directly.
- tagged literal handling touches namespace/runtime metadata.
- parse/eval boundaries are not yet clean.

### 5. Evaluator/forms boundary

Candidate future package:

```text
core/internal/eval/
```

Current candidate files:

- `eval.go`
- `expr.go`
- `tco.go`
- `tco_rewrite.go`
- `public_forms.go`
- form/compiler pieces in `parse.go`

Risks:

- this is a high-cycle layer and should move late.
- it depends on objects, namespaces, collections, reader output, runtime frames, and IR fallback.

### 6. WASM boundary

Candidate future package:

```text
core/internal/wasm/
```

Current candidate files:

- `wasm_binary.go`
- `wasm_codegen.go`
- `wasm_codegen_host.go`
- `wasm_host.go`
- `wasm_fn.go`
- `wasm_array.go`
- `wasm_mem_nth.go`
- `wasm_multifn.go`
- `wasm_runtime.go`

Risks:

- WASM depends heavily on `IRProgram` internals and host object operations.
- move after the IR program representation is owned by `core/internal/ir`.

## R5 decision for now

R5 should remain blocked on the rest of R3/R4:

- IR compiler/executor still live in root `core`.
- generated bootstrap files still live in root `core`.
- object/runtime contracts are not explicit enough to move collections or reader cleanly.

Therefore the next implementation work should continue reducing IR/WASM/generated coupling before moving collections/reader/runtime.

## R5 checklist status

- [x] Inventory collection/reader/runtime/evaluator/WASM split candidates.
- [x] Confirm that broad R5 moves should wait until IR/generated boundaries are stable.
- [ ] Move collections only after object/protocol contracts are explicit.
- [ ] Move reader only after object construction and tagged literal contracts are explicit.
- [ ] Move runtime/evaluator only after call/error/frame contracts are explicit.
