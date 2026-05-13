# IR program split design note

Updated: 2026-05-11

## Purpose

This note narrows the next R3 step: split the root `core.IRProgram` into a small IR-owned model plus root-core execution metadata. The goal is a clean package boundary, not a wrapper layer.

## Current problem

`core.IRProgram` mixes three concerns:

1. **IR model** — bytecode, constants, slot counts, opcode analysis, arity subprogram shape.
2. **Compiler/runtime captures** — `bindingKey`, captured `Object` values, capture slot indices, self-call metadata.
3. **Execution caches and helpers** — typed/boxed failure bits, native helper functions, `FnExpr` references, trace naming, escape analysis.

That prevents moving the program model into `core/ir` without exporting broad root-core internals.

## Proposed split

### `core/ir.Program`

Owns the package-neutral IR representation:

```go
type Program struct {
    Code []byte
    NumSlots int
    ConstantsLen int
    CaptureSlotIdxs []int
    CaptureSlotSet []bool
    HasSelf bool
    FloatConsts []float64
    ArityPrograms map[int]*Program
    VariadicProgram *Program
    VariadicMinArgs int
    Analysis *Analysis
}
```

Notes:

- `ConstantsLen` can be enough for diagnostics initially; moving actual `[]Object` constants waits on object boundaries.
- `Analysis` should be the `core/ir` shape-analysis type, not root `core.IRAnalysis`.
- Fields should be exported only if root-core callers need direct access during migration; prefer constructor/accessor helpers once usage stabilizes.

### Root `core.IRProgram`

Temporarily remains the executable envelope:

```go
type IRProgram struct {
    model *ir.Program
    constants []Object
    captureKeys []bindingKey
    captureSlots []Object
    escapeInfo *EscapeInfo
    typedFailed bool
    execFailed bool
    memNthFailed bool
    nativeHelper nativeF64Fn
    nativeHelper2 nativeF64Fn2
    nativeChecked bool
    arityPrograms map[int]*IRProgram
    variadicProg *IRProgram
    fnExprs []*FnExpr
    traceName string
}
```

This lets the compiler/executor continue to use root-core object/runtime details while diagnostics, profiling, export shape, and WASM eligibility start reading the neutral model. Diagnostics and exported bytecode/slot accessors already do this; profile and WASM lowering still need migration.

## Migration sequence

1. Add `core/ir.Program` and conversion/accessor helpers. **Done: initial neutral model exists in `core/ir/model.go`.**
2. Populate `IRProgram.model` during compilation while leaving old fields in place. **Done: root executable envelopes now refresh a neutral model.**
3. Move analysis fields/helpers to the internal model. **Started: `AnalyzeIRProgram` writes analysis back into the neutral model.**
4. Update diagnostics/export/profile/WASM eligibility to read the model. **Done for diagnostics, exported bytecode/slot accessors, native helper eligibility/lowering, and pure/imported/memory WASM eligibility/lowering. Profile paths are opcode-stream based and do not own program shape. Executor and escape-analysis paths stay on the root executable envelope until runtime/object execution metadata is split.**
5. Remove duplicated root fields once no callers use them.
6. Only then consider moving compiler or executor pieces.

## Guardrails

- Keep `go test ./core/internal/...` in `make docs-check`.
- Keep benchmark correctness tests before performance work.
- Do not export `Object`, `FnExpr`, `bindingKey`, or native helper internals just to move files.
- Do not add long-term compatibility wrappers; root `IRProgram` is a temporary executable envelope.
