# Joker Optimization Architecture

## Overview

Joker's execution engine uses a **tiered dispatch** model with five execution paths, each progressively faster and more specialized. Programs enter at the top (tree-walker) and are promoted to faster tiers as the compiler proves they're eligible.

```
┌─────────────────────────────────────────────────┐
│  Clojure source                                 │
│  ↓ parse                                        │
│  AST (Expr tree)                                │
│  ↓ irCompile / irCompileFn                      │
│  IR bytecode (IRProgram)                        │
│  ↓ dispatch                                     │
│                                                 │
│  ┌──────────────┐  ┌──────────────┐             │
│  │ irExecTyped  │  │   irExec     │             │
│  │ (32B irValue │  │ ([]Object    │             │
│  │  stack)      │  │  stack)      │             │
│  └──────┬───────┘  └──────┬───────┘             │
│         │                 │                     │
│         ▼                 ▼                     │
│  ┌──────────────┐  ┌──────────────┐             │
│  │ Native f64   │  │   WASM       │             │
│  │ closures     │  │ (wazero)     │             │
│  └──────────────┘  └──────────────┘             │
│                                                 │
│  Fallback: tree-walker (Fn.Call → Eval)         │
└─────────────────────────────────────────────────┘
```

## Tier 1: Tree-Walker (baseline)

The default execution path. Every Clojure expression has an `Eval(env)` method that walks the AST recursively. Function calls go through `Fn.Call(args)` which creates a new `LocalEnv`, binds arguments, and evaluates the body.

**Cost model:**
- Every numeric value (`Int{I: x}`, `Double{D: x}`) stored in an `Object` interface escapes to heap
- Each function call allocates an environment frame
- ~50ns per expression evaluation (interface dispatch + GC pressure)

**When used:** Expressions that can't be compiled to IR, or functions called from non-IR code paths.

## Tier 2: Boxed IR Interpreter (`irExec`)

**File:** `core/ir_exec.go` (765 lines)

The IR compiler (`irCompile` / `irCompileFn`) translates loop bodies and function bodies into a bytecode program (`IRProgram`). The bytecode uses a stack machine with ~30 opcodes covering arithmetic, comparisons, control flow, collection operations, and function dispatch.

```go
type IRProgram struct {
    code          []byte      // bytecode
    constants     []Object    // literal pool
    numSlots      int         // local variable count
    captureKeys   []bindingKey
    captureSlots  []Object    // resolved closure values
    captureSlotIdxs []int     // slot indices for captures
    hasSelf       bool        // self-recursive (irCallSelf)
    nativeHelper  nativeF64Fn // compiled Go closure
    // ...
}
```

**Cost model:**
- Stack is `[]Object` — every push allocates for numeric types
- ~5ns per opcode dispatch (Go switch statement)
- Eliminates environment frame allocation per loop iteration
- Self-recursive calls use an explicit frame stack (depth-limited at 256)

**When used:** Programs that compile to IR but aren't eligible for the typed executor. Collection-heavy programs with map operations, programs that fail typed eligibility checks.

## Tier 3: Typed IR Interpreter (`irExecTyped`)

**File:** `core/ir_typed_exec.go` (665 lines)

The key optimization: replaces `[]Object` stack with `[]irValue` where `irValue` is a 32-byte tagged union:

```go
type irValue struct {
    tag irValueTag   // 1 byte: int/double/bool/char/string/keyword/object/nil
    i   int          // int value, rune count, bool flag
    f   float64      // double value, ASCII flag
    p   unsafe.Pointer // → string | []byte | map | []int | *ArrayVector | *Fn
}
```

**Why 32 bytes matters:** The original `irValue` was 120 bytes (with inline string, []byte, map, []int, Object fields). Every push/pop copied 120 bytes. Go's `duffcopy` routine consumed 23% of CPU. At 32 bytes, copies are 4× cheaper and fit in two cache lines.

**Zero-allocation numerics:** `Int`, `Double`, `Boolean` values are stored inline in the `i`/`f` fields — no heap allocation. Only `Object`-tagged values (vectors, maps, strings) use the `p` pointer field.

**Dedicated type tags eliminate boxing for common types:**
- `irValKeyword`: stores `name *string` directly in `p` — keyword equality is pointer comparison
- `irMakeObject` for `*ArrayVector`, `*TransientVector`, `*Fn`: stores the Go pointer directly, no `Object` interface boxing

**Eligibility:** `irTypedEligible(analysis)` checks the IR program's opcode profile:
- Pure numeric loops (no strings, no collections) → always eligible
- Call-slot loops with nth (spectral-norm pattern) → eligible
- Self-recursive tree builders/walkers (binary-trees pattern) → eligible
- String-heavy loops with append/prepend patterns → eligible
- Map-building loops (word-frequency pattern) → eligible with env var

**Frame stack:** Self-recursive calls (`irCallSelf`) use an `irTypedFrameStack` that saves/restores `[]irValue` slots in a pre-allocated contiguous pool. Each frame save copies 32×N bytes instead of allocating a new Go stack frame. Depth-limited at 256 to avoid overhead for exponential recursion (fibonacci).

**When used:** Most compiled programs. The eval path tries `irExecTyped` first, falls through to `irExec` if it returns nil.

## Tier 4: Native f64 Closures

**File:** `core/ir_native_helper.go` (170 lines)

For pure arithmetic helper functions (no collections, no strings, no control flow beyond if/recur), the IR bytecode is compiled to a Go closure:

```go
type nativeF64Fn func(args []float64) float64
```

The closure interprets the same IR opcodes but operates on a `[]float64` stack — pure register-like execution with zero allocation. The `noescape64` trick (`core/noescape.go`) prevents the float64 slice argument from escaping to heap.

**Example:** Spectral-norm's `A(i,j) = 1/((i+j)(i+j+1)/2 + i+1)` compiles to a native closure that runs at near-Go speed.

**Dispatch:** When the typed executor encounters `irCallSlot` and the called function has a `nativeHelper`, it extracts float64 args from the irValue stack, calls the closure directly, and pushes the result — zero Object boxing in the entire call chain.

## Tier 5: WASM (wazero)

**Files:** `core/wasm_*.go` (~2500 lines)

Pure arithmetic helpers can also be compiled to WebAssembly modules and executed via wazero's ahead-of-time compiler. This generates native x86-64/arm64 machine code.

**Current status:** WASM is available but rarely faster than native f64 closures because:
- Each WASM function call has ~200ns boundary-crossing overhead
- The native f64 closure path avoids this entirely
- WASM modules have ~1-5ms compilation latency

**When used:** Only for spectral-norm's `A` function (which also has a native closure). The WASM path is tried after native helpers fail.

**Future potential:** WASM could be valuable for compiling entire loop bodies (not just helpers) to native code, eliminating the Go switch dispatch overhead (~5ns/op). This would require WASM codegen for collection operations (nth, assoc) via host function imports.

## IR Compilation Pipeline

### Loop Compilation (`irCompile`)

```
LoopExpr → irCompiler.compileExpr → IRProgram
```

The compiler walks the loop body, assigning bindings to numbered slots. Inner `let` bindings and nested loops get additional slots. Captures from outer scopes are resolved and stored as pre-filled slot values.

**Frame detection:** The compiler must map parse-time frame numbers to IR slot indices. Three helpers handle this:

- `guessLoopFrame`: finds the loop's own frame from RecurExpr bindings
- `guessFnParamFrame`: finds the fn parameter frame (requires ALL indices 0..N-1 present)
- `findLetFrame`: uses known-binding exclusion to find each let's frame precisely

**Depth limit:** Nested let/loop compilation is limited to depth 8. The n-body `advance` function (5 nested loops) requires this depth.

### Function Compilation (`irCompileFn`)

```
Fn → FnArityExpr → irCompiler → IRProgram (with captures)
```

Functions compile like loops but with additional handling:
- Parameters mapped to the fn's frame
- Captures resolved from `fn.env` chain at compile time
- Self-recursive calls emit `irCallSelf` opcode
- Results cached on the `*Fn` object via atomic flag (`irGetFnProg`)

**Capture resolution:** When `irCompileFn` encounters bindings outside the fn's scope, it walks `fn.env` to find the runtime value. These become `captureSlots` on the IRProgram, injected into slots at execution time. This enables letfn-bound functions (mandelbrot's pixel, binary-trees' make-tree/check-tree) to compile to IR.

### Escape Analysis

**File:** `core/escape_analysis.go` (231 lines)

Before execution, `irExec` analyzes which slots are "safe mutable" — used only as recur arguments, never passed to function calls or returned. Safe slots are converted to transient versions:

- `*ArrayVector` → `*TransientVector` (in-place assoc)
- `*ArrayMap` → `*TransientMap` (in-place assoc)
- `String` → `*TransientString` (in-place append via `[]byte`)

This enables zero-copy collection mutation in loop accumulators without changing the Clojure source code.

### Helper Inlining

**File:** `core/ir_inline.go` (408 lines)

When a loop calls a pure arithmetic helper via `irCallSlot`, the compiler can inline the helper's body directly into the loop's bytecode. This eliminates the function call overhead entirely.

**Guards:**
- Helper must be ≤32 expressions
- No collection operations in the inlined body
- `exprIsPureArithmetic` classifier gates inlining

### IR Analysis

**File:** `core/ir_analysis.go` (150 lines)

`AnalyzeIRProgram` scans the bytecode and produces an `IRAnalysis` struct describing the program's characteristics:

```go
type IRAnalysis struct {
    NumOps, NumSlots, NumCaptures int
    UsesFloat, UsesString, UsesCollection, UsesTransient bool
    HasCallSlot, HasSelfCall, HasNestedRecur, HasGenericNth bool
    HasMapOps, HasStringAppend, HasStringPrepend bool
    SuggestedPath string  // "wasm", "typed-ir", "ir-collection-builder", etc.
}
```

This drives eligibility decisions for the typed executor, WASM compilation, and inlining.

## Allocation Reduction Strategy

The optimization work follows a **progressive elimination** pattern, removing allocations from the inside out:

### Layer 1: Numeric boxing (eliminated)
`Int{I: x}` and `Double{D: x}` stored in `Object` interface → heap alloc. **Fix:** `irValue` stores numerics inline. Impact: millions of allocs eliminated for arithmetic-heavy programs.

### Layer 2: Function call boxing (eliminated for IR-compiled fns)
`Fn.Call(args)` boxes arguments into `[]Object`. **Fix:** `irCallSlot` dispatches through `irExecTyped` which uses `irValue` args. Impact: eliminated tree-walker overhead for fn calls from IR code.

### Layer 3: Collection type boxing (partially eliminated)
`*ArrayVector` and `*Fn` stored in `Object` interface via `irMakeObject` → `new(Object)` alloc. **Fix:** Direct pointer storage using `unsafe.Pointer` with sub-tag in `irValue.i`. Impact: 360K allocs eliminated for binary-trees.

### Layer 4: Keyword boxing (eliminated)
`Keyword` value type stored in `Object` interface → heap alloc for every keyword comparison. **Fix:** `irValKeyword` tag stores `name *string` pointer directly. Equality is pointer comparison. Impact: 700K allocs eliminated for binary-trees.

### Layer 5: Frame allocation (eliminated for depth ≤ 256)
Recursive `irExec(prog, args)` allocates `[16]Object` slot buffer per call. **Fix:** Explicit frame stack saves/restores slots in pre-allocated contiguous memory. Impact: binary-trees from 3.1M to 1.7M allocs.

### Remaining allocation sources (inherent)
- `make([]Object, n)` for vector construction — Go requires heap allocation for slices
- `Keyword{...}` stored in `[]Object` — Go boxes value types in interfaces
- `new(Object)` for non-pointer types (Char, String) in `irMakeObject`

These are Go language limitations. Further reduction would require either:
- A custom allocator (arena allocation for tree-building patterns)
- NaN-boxing the `[]Object` array elements (changes the ArrayVector representation)
- Compiling to a different data representation (Go structs instead of Clojure vectors)

## File Organization

| File | Lines | Purpose |
|---|---:|---|
| `ir.go` | 99 | Opcodes, cache, IRProgram struct |
| `ir_compile_fn.go` | 121 | Function compilation with capture resolution |
| `ir_compiler.go` | 773 | IR compiler (compileExpr, let/loop, frame detection) |
| `ir_inline.go` | 408 | Helper inlining + call compilation |
| `ir_exec.go` | 765 | Boxed IR interpreter |
| `ir_typed.go` | 270 | irValue type, helpers, eligibility |
| `ir_typed_exec.go` | 665 | Typed IR interpreter |
| `ir_typed_frame_stack.go` | 53 | irValue frame stack for self-recursion |
| `ir_frame_stack.go` | 60 | Object frame stack for boxed executor |
| `ir_frame_detect.go` | 82 | Precise let/loop frame detection |
| `ir_native_helper.go` | 170 | Native f64 closure compiler |
| `ir_fn_cache.go` | 153 | Fn→IRProgram caching + loop wrapper builder |
| `ir_analysis.go` | 150 | IR program analysis |
| `ir_value_accessors.go` | 136 | irValue unsafe.Pointer accessors |
| `ir_diagnostics.go` | 252 | Debug/diagnostic tools |
| `ir_exported.go` | 96 | Exported API for IR introspection |
| `escape_analysis.go` | 231 | Transient auto-promotion |
| `noescape.go` | 22 | unsafe.Pointer noescape trick |
| `wasm_*.go` | ~2500 | WASM compilation + runtime (10 files) |

## Performance Results

### vs Python 3.13 (CPython)

| Benchmark | Joker | Python | Ratio | Path |
|---|---:|---:|---:|---|
| tail-rec sum | 0.07ms | 3.6ms | **0.02×** | Typed IR + TCO |
| arithmetic loop | 0.25ms | 6.65ms | **0.04×** | Typed IR |
| recursive fib | 1.0ms | 21ms | **0.05×** | Typed frame stack |
| spectral-norm | 14ms | 24.5ms | **0.58×** | Native f64 + typed dispatch |
| pidigits | 0.04ms | 0.05ms | **0.74×** | Typed IR |
| binary-trees | 110ms | 54ms | 2.0× | Typed frame stack + keyword tag |
| mandelbrot | 15ms | 4.8ms | 3.2× | Typed IR + fn closure |
| n-body | 40ms | 0.66ms | 60× | Tree-walker fallback |

### vs Goja (Go JavaScript interpreter)

Beat Goja on 11/13 benchmarks. Goja losses: fannkuch (4.8×) and n-body (8.4×), both due to persistent data structure overhead that JavaScript's mutable arrays don't have.

## Future Directions

1. **Compile letfn wrapper loops to IR** — would route fannkuch heap-perm and n-body advance calls through IR instead of tree-walker
2. **Arena allocation for tree-building patterns** — pool-allocate `[]Object` slices for `irBuildVec` in loop contexts
3. **WASM for entire loop bodies** — compile numeric loops with nth to WASM, using host function imports for vector access
4. **Transient auto-promotion in irExecTyped** — detect safe-mutable slots in the typed executor and convert ArrayVectors to TransientVectors automatically
