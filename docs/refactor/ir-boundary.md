# IR boundary audit

Updated: 2026-05-11

## Purpose

This is the R3 inventory for moving the current `core/ir*.go` grab-bag into an architectural IR package tree. The target is clean package boundaries, not compatibility wrappers; breaking internal import paths are acceptable.

## Current IR file set

Current files matching `core/ir*.go` include:

- `core/ir.go` — cache and executable `IRProgram` envelope; opcode model has started moving to `core/internal/ir`
- `core/ir_analysis.go` — analysis summaries
- `core/ir_call_dispatch.go` — call dispatch bridge from `Fn`/`Proc`
- `core/ir_compile_fn.go` — function compilation
- `core/ir_compiler.go` — loop/expression lowering
- `core/ir_diagnostics.go` — diagnostics and opcode naming
- `core/ir_exec.go` — boxed interpreter
- `core/ir_exec_typed_nb.go` — typed/nanbox execution
- `core/ir_exported.go`, `core/ir_exported2.go` — exported artifact APIs
- `core/ir_fn_cache.go` — function IR cache
- `core/ir_frame_detect.go`, `core/ir_frame_stack.go`, `core/ir_typed_frame_stack.go` — frame/stack helpers
- `core/ir_inline.go` — inline compilation/fast paths
- `core/ir_map_diagnostics.go` equivalent tests only; no standalone file
- `core/ir_nanbox.go` — nanbox value helpers
- `core/ir_native_helper.go` — native numeric helpers
- `core/ir_profile.go` — now adapter only; state moved to `core/trace`
- `core/ir_typed.go`, `core/ir_typed_exec.go` — typed IR metadata/executor
- `core/ir_value_accessors.go` — value access helpers
- many `core/ir*_test.go` files

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
   - Move opcode definitions, opcode naming, `IRProgram` shape, `IRAnalysis`, and diagnostic/export helpers into `core/internal/ir`.
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
core/internal/ir/
├── model.go              # opcodes, Program, constants/captures metadata
├── opcode.go             # opcode names, widths, iteration helpers
├── diagnostics/          # explain/export helpers once cycles are gone
├── analysis/             # IR analysis summaries
├── compile/              # expression lowering adapters
├── exec/                 # boxed interpreter
├── exec/typed/           # typed/nanbox executor
├── profile/              # adapters to core/trace IRProfile
└── wasm/                 # later, if kept under IR rather than sibling package
```

## First executable boundary candidate

`irOpcodeName` and opcode-width/iteration helpers are the safest first move because they only depend on opcode constants. However, the opcode constants and `IRProgram` are currently in `core/ir.go`, so the actual first code move should extract a `core/internal/ir` package containing:

- opcode constants
- `OpcodeName(op byte) string`
- `OpWidth(code []byte, pc int) int` or an iterator helper

Then `core/ir_diagnostics.go`, `core/ir_profile.go`, and render/export paths can call into that package while the compiler/executor still live in `core`.

## IRProgram split note

The next concrete R3 step is documented in `ir-program-split.md`. The planned shape is a package-neutral `core/internal/ir.Program` for bytecode/slot/analysis/arity metadata, while root `core.IRProgram` temporarily remains the executable envelope for `Object` constants, `bindingKey` captures, `FnExpr` references, native helpers, escape analysis, and execution failure caches.

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
- [x] Implement the initial `IRProgram` model/envelope split: `core/internal/ir.Program` now carries the package-neutral bytecode/slot/analysis shape while root `core.IRProgram` remains the executable envelope for core-owned runtime metadata.
