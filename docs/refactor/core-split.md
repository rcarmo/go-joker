# Core package split audit

Updated: 2026-05-20

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


## 2026-05-20 status update

The type/object split has advanced enough that `core/types` is now the canonical object/protocol package. Root `core` no longer defines or aliases `Object`, and the recent cleanup removed transitional root aliases for `Keyword`, `Symbol`, `Map`, `Meta`, `MetaHolder`, `MapIterator`, `Pair`, and `EmptyMapIterator`. Root callers now use explicit `coretypes.*` names for those contracts. Major moved families include scalar values (`Int`, `Double`, `Boolean`, `Char`, `String`, `Time`, `Regex`, `Comment`), big numeric values (`BigInt`, `BigFloat`, `Ratio`), numeric operation implementations, `RecurBindings`, `Delay`, symbol/name values, generic info helpers, shared collection protocols (`Map`, `Set`, `Vec`) and metadata/ref contracts (`Meta`, `MetaHolder`, `Ref`).

Root `core` file count is now 5 total Go files (1 root test file). `core/types` has 29 Go files, `core/types/collections` has 22, `core/types/string` has 21, and `core/runtime` has 41. Concrete collection implementations have moved to `core/types/collections`; root collection files are deleted and guarded against reintroduction. Runtime-owned object wrappers/state/helpers for channels, futures/promises, agents, atoms, eval errors, nil/truthiness, I/O wrappers, callstacks, interpreter state, process exit/version/error plumbing, and formatting hooks now live in `core/runtime`; root proc/env glue uses exported runtime methods and `coretypes` runtime hooks for errors/arity instead of reaching into moved state directly. Root generated bootstrap still remains `core/a_generated_bootstrap_payloads.go`; previous root `types_assert_gen.go`/`types_info_gen.go` were replaced by explicit root support co-located in `runtime_kernel.go` while moved helpers live in `core/types`.

## Proposed split order

Do not split everything at once. Move leaf or low-cycle families first, then higher-coupling layers.

### 1. Already started

- `core/trace` owns tracing/profiling aggregation state.
- `core/ir` owns opcode names/constants, bytecode disassembly/counting, shape analysis, and the neutral program model.
- `core/wasm` owns leaf WASM binary encoding/module/host helpers.
- `core/types/collections` owns concrete collection types and mechanics: vectors, persistent vectors, lists/seqs, array/hash maps, sets, chunks, formatting/indexed ops, and bitmap/hash-index helpers.
- `core/reader` owns root-independent reader mechanics such as char classes, whitespace/comment/top-level-trivia/line decisions, identifier token scanning/validation/keyword, standalone-slash, and literal classification/issue enumeration, escape/unicode parsing, top-level read-form and number-token classification, delimiter/dispatch/form helpers, rune-window history, line rune readers, and raw IO wrappers.
- `core/types/string` and `core/types/numerical` own string/cache/cursor and numeric parsing/hash/comparison mechanics; the Joker scalar values themselves live in `core/types`.
- `cmd/joker` owns the CLI entrypoint.

### 2. Runtime/object boundary

Candidate future package:

```text
core/runtime/
```

Likely contents or responsibilities:

- generic goroutine runtime bookkeeping and pending/channel primitives already in `core/runtime`
- runtime-owned wrappers for `ObjectChannel`, `ObjectFuture`, `ObjectPromise`, `Agent`, and `Atom`
- eval frame stack helpers and call dispatch scaffolding that is not IR-specific
- panic/error helpers only after object/error contracts are clear

Current candidate files:

- environment/namespace/goroutine runtime glue now coalesced into `runtime_kernel.go`; next runtime candidates are frame/call scaffolding currently mixed through evaluator and IR files
- selected parts of `call_fast.go`, only after call contracts are explicit

Risks:

- runtime currently touches `Object`, `Fn`, `Proc`, `Var`, namespaces, and evaluator state.
- moving too early can create cycles with `core` unless object interfaces are extracted first.

### 3. Collections boundary

Candidate future package:

```text
core/types/collections/
```

Moved concrete collection files:

- `array_map.go`
- `array_vector.go`
- `chunked_seq.go` collection mechanics (root chunk proc registration is coalesced into `runtime_kernel.go` until proc/env ownership moves)
- `hash_map.go`
- `list.go`
- `map.go` helper functionality
- `persistent_vector.go`
- `seq.go`
- `set.go`
- `vector.go`

Still root-owned collection-adjacent files:

- sorted/transient collection proc glue now coalesced into `runtime_kernel.go`
- `transient_string.go`
- related evaluator/runtime fast paths now coalesced into `runtime_kernel.go`

Status and risks:

- `core/types/collections` owns concrete vectors, maps, sets, lists, seqs, chunks, and their mechanics.
- collection types remain part of the public runtime object model through `coretypes` protocols and runtime hooks.
- reader/evaluator/std/generated packages now construct moved concrete collection types via `corecollections.*` direct imports.
- sorted collection proc registration, transients, evaluator fast paths, runtime hooks, and generated/bootstrap placement are the remaining collection-adjacent risks.

Preferred prerequisite:

- keep `coretypes` object/protocol contracts, generated guards, and layout guard green; do not reintroduce root collection aliases. Further movement should target runtime/env/proc ownership or generated/bootstrap placement as coherent batches.

### 4. Reader/parser boundary

Candidate future package:

```text
core/reader/
```

Current candidate files:

- parser/reader/evaluator integration now coalesced into `runtime_kernel.go`; gen-code slow bootstrap helpers are coalesced into `bootstrap_gen_code.go`
- `read_conditional_test.go`

Status and risks:

- `core/reader` now owns leaf mechanics: rune-window history, rune-stream Get/Unget/Peek position accounting, reader position stacks, line rune reader, raw file/buffer/buffered/IO wrappers, char classes, whitespace/comment/top-level-trivia/line scanning decisions/runs, identifier token scanning/checks/keyword, standalone-slash, and literal classification/validation issue enumeration, unicode/string escape parsing, top-level read-form and number-token classification, dispatch/format-prefix/delimiter/form helpers, and conditional read-error/suppression/result, conditional/unquote/namespaced-map start/prefix/splice, and syntax-quote auto-gensym name decisions.
- reader/parser still owns namespace/tagged-literal/runtime side effects; current production call sites route construction through `ReaderConstructionAdapter`, guarded by `construction_boundary_guard_test.go`. The former tiny root `reader.go` wrapper has been folded into `runtime_kernel.go`.
- tagged literal handling touches namespace/runtime metadata.
- parse/eval boundaries are not yet clean.

### 5. Evaluator/forms boundary

Candidate future package:

```text
core/eval/ (future target; not reserved yet)
```

Current candidate files:

- `runtime_kernel.go`
- expression dump/inference helpers now coalesced into `runtime_kernel.go`
- tail-call runtime trampoline and parse-time recur rewrite now coalesced into `runtime_kernel.go`
- public protocol/record/hierarchy forms now coalesced into `runtime_kernel.go`
- form/compiler pieces now coalesced into `runtime_kernel.go`

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
