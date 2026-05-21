# Root kernel inventory

Updated: 2026-05-21

The root `core` package has been coalesced to the smallest practical file set without introducing fake package moves or compatibility shims. This document records what remains in root and what must happen before each remaining domain can move to a real owning package.

## Current root files

| File | Build tags | Ownership status | Why it remains root-owned |
|---|---|---|---|
| `a_generated_bootstrap_payloads.go` | `!gen_code` | Generated fast-start bootstrap payloads | Mutates root `Var`, namespace, function, metadata, source-info, and runtime bootstrap state directly. |
| `bootstrap_gen_code.go` | `gen_code` | Generator/runtime bootstrap helpers | Used only while generating fast-start payloads; depends on root runtime objects and constructors. |
| `runtime_kernel.go` | none | Handwritten root runtime kernel | Coalesces evaluator, parser/reader integration, root object model (`Nil`, `Var`, `Proc`, `Fn`, `ExInfo`), proc/env registration, protocol/record/hierarchy glue, transducer/reduce fast paths, IR cache/executor/lowering, and WASM compile glue. |
| `runtime_kernel_contracts_test.go` | test | Root boundary/contract guards | Needs root-package access to unexported construction/evaluator/kernel details. |
| `runtime_kernel_spew_enabled.go` | `go_spew` | Optional debug override | Overrides the default `procGoSpew` only when the optional `go_spew` dependency is enabled. |

## Next real extraction targets

| Priority | Domain inside `runtime_kernel.go` | Target package | Main blockers to resolve first | Non-goal |
|---:|---|---|---|---|
| 1 | `Runtime`, `EvalError`, current expression, goroutine root glue | `core/runtime` | Started: call stack frame storage/formatting now lives in `core/runtime.Callstack` behind the root-independent `core/runtime.Traceable` interface, per-goroutine interpreter state now lives in `core/runtime.GoroutineRT` with root `Expr` held as `any`, the typed state pool lives in `core/runtime.InterpreterStatePool`, and `Nil`, nil checks/truthiness, `EvalError`, `Reduced`, file-info maps, stdlib I/O, and process/version plumbing live in `core/runtime`. Remaining blockers are root `Expr`, `Fn`, `Proc`, `Var`, namespace state, and root runtime/type construction. | Do not add hook shims that simply call back into root. |
| 2 | IR program/cache/compiler/boxed executor/WASM lowering cluster | `core/ir` plus `core/wasm` for WASM leaves | Move or contract root `Fn`, `Var`, `Expr`, local env/frame slots, call dispatch, errors, and tracing-facing names. | Do not move `boxed_exec` alone. |
| 3 | Reader/parser orchestration now in the kernel | `core/reader` for reader mechanics; parser remains root/eval until expression contracts exist | Finish construction/tag/metadata/error adapters so reader code does not need root object/evaluator state. | Do not let `core/reader` import root `core`. |
| 4 | Namespace/env/proc registration glue | `core/runtime` or a dedicated env package | Started: namespace map locking now lives in `core/runtime.NamespaceMu`. Remaining blockers are `GLOBAL_ENV`, `Namespace`, `Var`, `Proc`, `STRINGS`, `TYPE`, generated bootstrap mutation, and `referToUser`. | Do not create aliases/wrappers for old root env APIs. |
| 5 | Generated bootstrap payload mutation | `core/generated` or a generated bootstrap package | Generator must emit a real package contract and install through explicit root/runtime APIs instead of direct root variable mutation. | Do not place `package core` files in subdirectories. |

## Recent small extractions

- Generic root-independent object predicates (`IsSymbol`, `IsKeyword`, `IsVector`, `IsSeq`), extraction helpers (`ExtractString`, `ExtractMap`, etc.), and default number equality now live in `core/types`.
- String sequence descriptor and pure character-to-string fast path now live in `core/types/string`; string cursor/transient-string object wrappers remain in `core/types` because moving them into `core/types/string` would reintroduce a `core/types` ↔ `core/types/string` import cycle.
- String vector construction now lives in `core/types/collections`.
- Generic formatting hooks (`PprintObject`, `FormatObject`, indentation/comment/newline helpers) now live in `core/runtime`.
- Process exit callback plumbing, the runtime version constant, and version-map construction now live in `core/runtime`.

## Guardrails

- `tests/layout_guard.sh` is the authoritative guard for the allowed root file set.
- `tools/coalesce-core-files.py` reports the root file set and fails if old split files reappear.
- Any future extraction must move a coherent ownership family and delete or update the corresponding root section in `runtime_kernel.go`; do not recreate standalone root helper files.
