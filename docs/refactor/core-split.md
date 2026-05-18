# Core package split audit

Updated: 2026-05-18

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
- tests (Go benchmarks now live under `benchmarks/core`)

This makes it too easy for features to reach across layers through unexported state and makes architectural intent hard to see from the repository layout.


## 2026-05-18 status update

The type/object split has advanced enough that `core/types` is now the canonical object/protocol package. Root `core` no longer defines or aliases `Object`, and the recent cleanup removed transitional root aliases for `Keyword`, `Symbol`, `Map`, `Meta`, `MetaHolder`, `MapIterator`, `Pair`, and `EmptyMapIterator`. Root callers now use explicit `coretypes.*` names for those contracts. Major moved families include scalar values (`Int`, `Double`, `Boolean`, `Char`, `String`, `Time`, `Regex`, `Comment`), big numeric values (`BigInt`, `BigFloat`, `Ratio`), numeric operation implementations, `RecurBindings`, `Delay`, symbol/name values, generic info helpers, shared collection protocols (`Map`, `Set`, `Vec`) and metadata/ref contracts (`Meta`, `MetaHolder`, `Ref`).

Root `core` file count is now 68 total Go files (1 root test file). `core/types` has 23 Go files. Concrete collection implementations remain root-owned, but their public/shared protocols now live in `core/types`; the next collection-move blockers are concrete implementation dependencies, metadata propagation, sorted collection/proc coupling, and construction cycles rather than root protocol aliases. Root generated files remain `core/a_generated_bootstrap_payloads.go`, `core/types_assert_gen.go`, and `core/types_info_gen.go`; `types_assert_gen.go` now contains only root-owned helper assertions (`Namespace`, `Var`, `Fn`, `Atom`, `File`, `Channel`) because `coretypes.*` and stdlib I/O assertions live in `core/types`.

## Proposed split order

Do not split everything at once. Move leaf or low-cycle families first, then higher-coupling layers.

### 1. Already started

- `core/trace` owns tracing/profiling aggregation state.
- `core/ir` owns opcode names/constants, bytecode disassembly/counting, shape analysis, and the neutral program model.
- `core/wasm` owns leaf WASM binary encoding/module/host helpers.
- `core/collections` owns root-independent collection mechanics such as generic slice storage, persistent list-node storage, map equality traversal, indexed operations, pair arrays, bitmap/hash-index helpers, and opaque trie nodes.
- `core/reader` owns root-independent reader mechanics such as char classes, whitespace/comment/top-level-trivia/line decisions, identifier token scanning/validation/keyword, standalone-slash, and literal classification/issue enumeration, escape/unicode parsing, top-level read-form and number-token classification, delimiter/dispatch/form helpers, rune-window history, line rune readers, and raw IO wrappers.
- `core/string` and `core/cursor` own root-independent string/cache/cursor mechanics; the Joker `String` value itself now lives in `core/types`.
- `cmd/joker` owns the CLI entrypoint.

### 2. Runtime/object boundary

Candidate future package:

```text
core/runtime/
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
core/collections/
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

Status and risks:

- `core/collections` now owns pure mechanics helpers used by `ArrayVector`, legacy `Vector`, `PersistentVector`, and `HashMap` where safe.
- collection types remain part of the public runtime object model.
- reader/evaluator/std packages used to construct concrete collection types directly; current production call sites now route through `CollectionConstructionAdapter`, guarded by `construction_boundary_guard_test.go`.
- numeric/hash/equality/protocol behavior crosses package boundaries.

Preferred prerequisite:

- keep `coretypes` object/protocol contracts and the construction adapter guard green, then move concrete collections only when implementation dependencies are explicit and acyclic. Most protocols have moved; remaining blockers are concrete implementation return types, metadata propagation, sorted collection/proc coupling, and initialization cycles.

### 4. Reader/parser boundary

Candidate future package:

```text
core/reader/
```

Current candidate files:

- `read.go`
- `read_conditional_test.go`
- tagged literal handling inside `read.go`
- parser-adjacent pieces in `parse.go`, `parse_slow_init.go`

Status and risks:

- `core/reader` now owns leaf mechanics: rune-window history, rune-stream Get/Unget/Peek position accounting, reader position stacks, line rune reader, raw file/buffer/buffered/IO wrappers, char classes, whitespace/comment/top-level-trivia/line scanning decisions/runs, identifier token scanning/checks/keyword, standalone-slash, and literal classification/validation issue enumeration, unicode/string escape parsing, top-level read-form and number-token classification, dispatch/format-prefix/delimiter/form helpers, and conditional read-error/suppression/result, conditional/unquote/namespaced-map start/prefix/splice, and syntax-quote auto-gensym name decisions.
- reader/parser still owns namespace/tagged-literal/runtime side effects; current production call sites route construction through `ReaderConstructionAdapter`, guarded by `construction_boundary_guard_test.go`. The former tiny root `reader.go` wrapper has been folded into `read.go`.
- tagged literal handling touches namespace/runtime metadata.
- parse/eval boundaries are not yet clean.

### 5. Evaluator/forms boundary

Candidate future package:

```text
core/eval/ (future target; not reserved yet)
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
core/wasm/
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

Current extraction:

- `core/wasm/encoding.go` owns ULEB/SLEB/f64 byte encoding helpers and direct tests.
- `core/wasm/module.go` owns generic WASM module section construction and direct tests.
- `core/wasm/host.go` owns host import names/module metadata and direct tests.
- `core/wasm/opcodes.go` owns shared WASM value-type/opcode constants and direct tests.
- `core/wasm_binary.go` remains as a compatibility adapter for the rest of the root-core WASM code while IR/WASM assembly flow is still coupled to `IRProgram` internals.

Risks:

- WASM depends heavily on `IRProgram` internals and host object operations.
- move the rest after the IR program representation is owned by `core/ir`.

## R5 decision for now

R5 should remain blocked on the rest of R3/R4:

- IR compiler/executor still live in root `core`, although a neutral `core/ir.Program` model and initial `RuntimeExecutionAdapter` contract now reduce the boundary.
- most generated bootstrap files still live in root `core`; the source manifest and linter payload registry have moved to `core/generated` as data-only package boundaries.
- object/runtime contracts are much narrower: `core/types` owns the canonical object/protocol surface, but root metadata, namespace/proc/bootstrap, concrete collections, and evaluator/runtime state still block broad package moves.

Therefore the next implementation work should continue reducing IR/WASM/generated coupling and codifying object/reader/runtime adapters before moving collections/reader/runtime implementations.

## R5 checklist status

- [x] Inventory collection/reader/runtime/evaluator/WASM split candidates.
- [x] Confirm that broad R5 moves should wait until IR/generated boundaries are stable.
- [x] Add object/protocol, reader construction, std native-boundary, and runtime execution contract guardrails that make future split prerequisites explicit.
- [x] Add narrow collection construction adapter and guard current production call sites.
- [x] Add narrow reader/expression construction adapter and guard current production call sites.
- [ ] Move collections only after object/protocol implementation contracts are explicit and acyclic.
- [ ] Move reader only after object construction, expression/tagged-literal, and evaluator handoff boundaries are explicit and acyclic.
- [ ] Move runtime/evaluator only after call/error/frame contracts are explicit in code and root execution metadata has a narrow adapter.
