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
│  │ irExecTypedNB│  │ irExecTyped  │             │
│  │ (8B NaN-box  │  │ (32B irValue │             │
│  │  stack)      │  │  stack)      │             │
│  └──────┬───────┘  └──────┬───────┘             │
│         │                 │                     │
│         ▼                 ▼                     │
│  ┌──────────────┐  ┌──────────────┐             │
│  │ Native f64   │  │   irExec     │             │
│  │ closures     │  │ ([]Object)   │             │
│  └──────────────┘  └──────────────┘             │
│                                                 │
│  Fallback: tree-walker (Fn.Call → Eval)         │
│  + irDispatchFnCall for self-recursive fns      │
└─────────────────────────────────────────────────┘
```

## Execution Tiers

### Tier 0: NaN-boxed Typed Executor (`irExecTypedNB`)
- **Stack:** `[]uint64` — 8 bytes per entry
- **When:** Pure numeric loops (no collections, strings, fn calls, self-recursion)
- **Impact:** pidigits 2× faster, fasta 1.3× faster vs 32-byte irValue
- **Opcodes:** Arithmetic, comparisons, jumps, recur, return, nth, count

### Tier 1: Typed IR Executor (`irExecTyped`)
- **Stack:** `[]irValue` — 32 bytes per entry (was 120 before optimization)
- **When:** Numeric loops, call-slot loops with nth, self-recursive tree walkers, string loops
- **Zero-alloc:** Int, Double, Boolean, Char, Keyword stored inline
- **Frame stack:** `irTypedFrameStack` for self-recursive calls (depth ≤ 256)
- **Opcodes:** All arithmetic + comparisons + nth + first + conj + assoc + buildVec + callSlot + callSelf + str1/str2 + count + intCast + subs + toTransient/assocBang/toPersistent

### Tier 2: Native f64 Closures
- **Type:** `func([]float64) float64` — compiled Go closure
- **When:** Pure arithmetic helper functions (spectral-norm's A function)
- **noescape64:** Prevents f64 slice from escaping to heap
- **Dispatch:** Called from irCallSlot when `fnProg.nativeHelper != nil`

### Tier 3: Boxed IR Executor (`irExec`)
- **Stack:** `[]Object` — heap-allocated interface values
- **When:** Programs that fail typed eligibility (complex collection ops, map ops)
- **Escape analysis:** Converts safe-mutable ArrayVector/ArrayMap slots to transients
- **Frame stack:** `irFrameStack` for self-recursive calls (depth ≤ 256)

### Tier 4: Tree-Walker (`Fn.Call` / `Eval`)
- **When:** Expressions that can't compile to IR (atoms, higher-order fns, try/catch)
- **irDispatchFnCall:** Self-recursive compiled fns called from tree-walker are dispatched through IR (depth-limited at 64)

## IR Opcodes

### Arithmetic
`irAdd`, `irSub`, `irMul`, `irDiv`, `irRem`, `irInc`, `irDec`, `irSqrt`

### Comparison
`irLt`, `irEq`, `irIsZero`

Note: `>=`, `>`, `<=` are NOT yet IR opcodes due to a correctness bug with `or` macro expansion. They compile as `irLt` with swapped args for `>`, but `>=` and `<=` still use tree-walker.

### String
`irStr1`, `irStr2`, `irNthStringASCII`, `irIntCast` (char→int), `irSubs` (substring)

### Collection
`irNth`, `irFirst`, `irGet`, `irGet3`, `irAssoc`, `irConj`, `irCount`, `irBuildVec`

### Transient
`irToTransient`, `irAssocBang`, `irToPersistent`

### Control Flow
`irJumpIfNot`, `irJump`, `irRecur`, `irReturn`

### Dispatch
`irCallSlot` (fn in slot), `irCallSelf` (self-recursive), `irFallback`

## Key Data Structures

### irValue (32 bytes)
```go
type irValue struct {
    tag irValueTag      // 1 byte
    i   int             // int value, rune count, bool (0/1), char (as int)
    f   float64         // double value, ASCII flag for strings
    p   unsafe.Pointer  // → string | []byte | map | []int | *ArrayVector | *Fn | *Keyword
}
```

Dedicated tags: `irValInt`, `irValDouble`, `irValBool`, `irValChar`, `irValString`, `irValStringBuilder`, `irValStringIntMap`, `irValIntVector`, `irValObject`, `irValNil`, `irValKeyword`

### irMakeObject — direct pointer storage
For `*ArrayVector`, `*TransientVector`, `*Fn`: stores Go pointer directly in `irValue.p` with sub-tag in `irValue.i`. Avoids Object interface boxing allocation.

### NaN-boxing (8 bytes)
```
Double:  IEEE 754 bits (non-quiet-NaN)
Int:     0x7FF8_0001_XXXX_XXXX
Bool:    0x7FF8_0002_0000_000X
Nil:     0x7FF8_0003_0000_0000
Object:  0x7FF8_0004_XXXX_XXXX (side-table index)
```

## Compilation Pipeline

### Frame Detection
- `guessLoopFrame`: RecurExpr binding frame
- `guessFnParamFrame`: smallest frame with ALL param indices 0..N-1
- `findLetFrame`: known-binding exclusion for precise let frame identification

### Function Compilation (`irCompileFn`)
- Captures resolved from `fn.env` chain at compile time
- Self-recursive calls emit `irCallSelf`
- Cached on `*Fn` via atomic flag (`irGetFnProg`)
- Depth limit: 8 (enables n-body advance, fannkuch heap-perm)

### Escape Analysis
- Converts safe-mutable `*ArrayVector` → `*TransientVector`
- Converts safe-mutable `*ArrayMap` → `*TransientMap`
- String slots promoted to `*TransientString` (byte builder)

## Performance Results

### CLBG Benchmarks — Session Progress

| Benchmark | Session Start | Final | Speedup |
|---|---:|---:|---:|
| mandelbrot | 159ms | **14ms** | **11×** |
| binary-trees | 528ms | **98ms** | **5.4×** |
| spectral-norm | 70ms | **16ms** | **4.4×** |
| pidigits | 0.10ms | **0.020ms** | **5×** |
| fasta | 0.22ms | **0.048ms** | **4.6×** |

### vs Python 3.13 (CPython)
- **Beat Python: 6/13** (tail-rec, arith, fib, pidigits, spectral, fasta)
- Near-parity: regex-redux (1.07×), binary-trees (1.81×)
- Gap: mandelbrot (3×), fannkuch (22×), n-body (59×)

### vs Goja (Go JavaScript)
- **Beat Goja: 11/13**
- Losses: fannkuch (4.6×), n-body (8.2×) — persistent data structure overhead

### Parser Benchmarks (pure Clojure vs pure Python)
| Parser | Joker | Python | Ratio |
|---|---:|---:|---:|
| JSON small | 337µs | 17.9µs | 19× |
| HTML small | 85µs | 4.8µs | 18× |
| XML medium | 2041µs | 46.6µs | 44× |
| YAML medium | 478µs | 7.2µs | 66× |

String-heavy parsing is 18-67× slower than Python. Bottleneck: `(nth s i)` O(n) for non-cached strings, `(str buf c)` allocation per concat, tree-walker dispatch for uncached fns.

## User-Facing API: `joker.jit` Namespace

```clojure
(require 'joker.jit)

(def fast-fn (joker.jit/compile (fn [x y] (+ (* x x) (* y y)))))
(fast-fn 3.0 4.0)  ; => 25.0 — runs as native Go closure

(joker.jit/info (fn [x y] (* x y)))
; => {:compiled true, :path "native-f64", :slots 2, :captures 0, :self-recursive false}

(joker.jit/compiled? (fn [x] (+ x 1)))  ; => true
```

Compiles: arithmetic, comparisons, string ops, collection ops, loops, recursion.
Does NOT compile: atom deref, higher-order calls, try/catch, interop.

## File Organization

| File | Lines | Purpose |
|---|---:|---|
| `ir.go` | ~100 | Opcodes, cache, IRProgram struct |
| `ir_compile_fn.go` | ~120 | Function compilation with capture resolution |
| `ir_compiler.go` | ~780 | IR compiler (compileExpr, let/loop, frame detection) |
| `ir_inline.go` | ~470 | Helper inlining + call compilation + >=/>/<= |
| `ir_exec.go` | ~810 | Boxed IR interpreter |
| `ir_typed.go` | ~270 | irValue type, helpers, eligibility |
| `ir_typed_exec.go` | ~720 | Typed IR interpreter |
| `ir_exec_typed_nb.go` | ~310 | NaN-boxed typed interpreter |
| `ir_typed_frame_stack.go` | ~55 | irValue frame stack for self-recursion |
| `ir_frame_stack.go` | ~60 | Object frame stack for boxed executor |
| `ir_frame_detect.go` | ~85 | Precise let/loop frame detection |
| `ir_native_helper.go` | ~170 | Native f64 closure compiler |
| `ir_fn_cache.go` | ~155 | Fn→IRProgram caching + loop wrapper |
| `ir_analysis.go` | ~155 | IR program analysis |
| `ir_value_accessors.go` | ~140 | irValue unsafe.Pointer accessors |
| `ir_nanbox.go` | ~100 | NaN-boxing encode/decode |
| `ir_call_dispatch.go` | ~35 | IR-aware fn dispatch from tree-walker |
| `ir_arena.go` | — | (removed — arena caused corruption) |
| `noescape.go` | ~22 | unsafe.Pointer noescape trick |
| `escape_analysis.go` | ~230 | Transient auto-promotion |
| `wasm_*.go` | ~2500 | WASM compilation + runtime (10 files) |
| `std/jit/` | ~200 | joker.jit namespace |

## Known Issues

1. **`>=`/`>`/`<=` compilation** deferred — `or` macro's `let` temporary bindings get wrong slot assignments when enclosing loops newly compile to IR
2. **NaN-box for boxed executor** — object side-table causes correctness bugs for fn calls; only works in the typed executor
3. **Arena allocation** — shared backing arrays corrupt data when vectors are modified by transient assoc
4. **Transient auto-promotion in typed executor** — breaks spectral-norm's conj pattern
