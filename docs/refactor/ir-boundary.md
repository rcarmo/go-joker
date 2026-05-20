# IR boundary audit

Updated: 2026-05-18

## Purpose

This is the R3 inventory for moving the current `core/ir*.go` grab-bag into an architectural IR package tree. The target is clean package boundaries, not compatibility wrappers; breaking internal import paths are acceptable.

## Current IR file set

The former `core/ir*.go` grab-bag has now been split into two categories:

### Real package-owned files under `core/ir/`

- `core/ir/model.go` — neutral `Program` model
- `core/ir/opcode.go` — opcode constants/naming
- `core/ir/analysis.go` — analysis summaries
- `core/ir/disassemble.go` — disassembly helpers
- `core/ir/frame_stack.go` — reusable generic frame-stack implementation
- `core/ir/nanbox.go` — reusable NaN-box codec

### Still-root-coupled files renamed out of the misleading `ir_*` prefix

- `core/program_envelope.go` — cache and executable `IRProgram` envelope
- `core/program_envelope.go` — root IR program envelope, cache, and analysis adapter layer
- `core/fn_ir_cache.go` — function IR cache plus call dispatch bridge from `Fn`/`Proc` (the former tiny `fn_ir_dispatch.go` was folded into this file)
- `core/fn_ir_compile.go` — function compilation
- loop/expression lowering now coalesced into `core/fn_ir_cache.go`
- `core/loop_wasm_diagnostics.go` — diagnostics tied to root `Expr` / `LoopExpr`
- `core/boxed_exec.go` — boxed interpreter
- `core/typed_exec_nanbox.go` — typed/nanbox execution path
- `core/runtime_ir_exports.go` — exported runtime/JIT artifact APIs; stale debug-only IR exports were removed instead of kept as bridges
- `core/fn_ir_cache.go` — function IR cache and dispatch bridge
- `core/loop_frame_detect.go` — frame-detection helpers
- `core/inline_rewrites.go` — inline compilation/fast paths
- `core/loop_native_helpers.go` — native numeric helpers
- `core/trace_adapters.go` — symbol/IR trace adapters only; state lives in `core/trace`
- `core/typed_values.go`, `core/typed_exec.go` — typed IR metadata/executor
- `core/typed_value_accessors.go` — root-coupled typed value access helpers
- `core/typed_exec_nanbox.go` — typed/nanbox execution path plus its local root object/table bridge
- many renamed root tests that no longer use the stale `ir_*` prefix

## Coupling inventory

The IR layer currently depends on these core concepts:

### Runtime object model

- `Object`, `Seqable`, `Callable`
- concrete objects: `Int`, `Double`, `Boolean`, `String`, `Char`, `ArrayVector`, maps, transients
- singletons/constants: `NIL`, booleans
- constructors/coercions: `Make*`, `Ensure*`, arithmetic helpers

### Evaluator/compiler model

- `Expr`, `LoopExpr`, `FnExpr`, `FnArityExpr`, `CallExpr`
- `LocalEnv`, slot/binding metadata, `bindingKey`
- fallback tree evaluation through `Eval` paths
- recur/loop semantics and local slot mutation rules

### Call/runtime model

- `Fn`, `Proc`, `Var`
- `irDispatchFnCall`, `irCallSelf`, call-slot handling
- goroutine/function tracing hooks
- panic/error behavior through `RT.NewError` and exception paths

### WASM/JIT coupling

- `WasmProgram`, WASM eligibility diagnostics, bytecode export
- native f64 helper slots
- IR opcode naming shared by diagnostics, profiling, renderers, and docs

## Proposed split order

The IR split should be incremental and acyclic:

1. **Model/diagnostics leaf first**
   - Move opcode definitions, opcode naming, `Program` shape, `IRAnalysis`, frame-stack helpers, and the NaN-box codec into `core/ir`.
   - Keep only adapters in `core` for APIs that must see `LoopExpr`/`FnExpr` initially.

2. **Compiler boundary second**
   - Define a small lowering interface for expressions/locals instead of importing all of `core` internals.
   - Move lowering once expression access is explicit.

3. **Executor boundary third**
   - Define a runtime interface for object operations, calls, errors, and tracing.
   - Move boxed executor, then typed/nanbox executor.

4. **WASM last**
   - Move WASM lowering/runtime after the `IRProgram` representation is owned by the IR package.

## Draft target structure for IR

```text
core/ir/
├── model.go              # neutral Program metadata
├── opcode.go             # opcode names, widths, iteration helpers
├── analysis.go           # IR analysis summaries
├── disassemble.go        # human-readable disassembly
├── frame_stack.go        # reusable generic frame-stack helpers
├── nanbox.go             # reusable NaN-box codec
├── compile/              # later, expression lowering adapters
├── exec/                 # later, boxed interpreter
├── exec/typed/           # later, typed/nanbox executor
├── profile/              # later, adapters to core/trace IRProfile
└── wasm/                 # later, if kept under IR rather than sibling package
```

## First executable boundary candidate

`irOpcodeName` and opcode-width/iteration helpers are the safest first move because they only depend on opcode constants. However, the opcode constants and `IRProgram` are currently in `core/ir.go`, so the actual first code move should extract a `core/ir` package containing:

- opcode constants
- `OpcodeName(op byte) string`
- `OpWidth(code []byte, pc int) int` or an iterator helper

Then `core/ir_diagnostics.go`, `core/ir_profile.go`, and render/export paths can call into that package while the compiler/executor still live in `core`.

## IRProgram split note

The next concrete R3 step is documented in `ir-program-split.md`. The current shape is a package-neutral `core/ir.Program` for bytecode/slot/analysis/arity metadata, while root `core.IRProgram` still remains the executable envelope for `Object` constants, `bindingKey` captures, `FnExpr` references, native helpers, escape analysis, and execution failure caches.

## Risks

- Moving executable `IRProgram` state too early will expose many unexported core details (`bindingKey`, `Object`, `FnExpr`, `EscapeInfo`, native helper funcs).
- Moving executor before runtime interfaces exist will create import cycles or broad exports.
- Tests currently live beside `core` internals; moving tests with the package will require either exported test hooks or package-local tests in the new package.

## R3 checklist status

- [x] Audit all `ir*.go` references to unexported core symbols.
- [x] Introduce a minimal exported boundary or adapter layer for opcode names/constants and analysis helpers.
- [x] Move diagnostic/export helpers first, then compiler/executor (started with opcode naming, op counting, disassembly, and shape-analysis helpers; direct tests now cover the extracted IR helper package).
- [x] Keep benchmark correctness tests before performance work.
- [x] Document the `IRProgram` model/envelope split.
- [x] Implement the initial `IRProgram` model/envelope split: `core/ir.Program` now carries the package-neutral bytecode/slot/analysis shape while root `core.IRProgram` remains the executable envelope for core-owned runtime metadata.
