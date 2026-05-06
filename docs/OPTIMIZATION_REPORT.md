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
`irLt`, `irGte`, `irGt`, `irLte`, `irEq`, `irIsZero`

### String & Cursor
`irStr1`, `irStr2`, `irNthStringASCII`, `irIntCast` (char→int), `irSubs` (substring)
`irCursorChar`, `irCursorNext`, `irCursorDone` (StringCursor ops)

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
| `string_cursor.go` | ~110 | StringCursor type (O(1) advance) |
| `string_cursor_procs.go` | ~55 | Cursor procs (string-cursor, cursor-char, etc.) |
| `string_cursor_init.go` | ~30 | Runtime proc registration |
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

1. ~~**`>=`/`>`/`<=` compilation** deferred~~ — **FIXED**: opcodes `irGte`/`irGt`/`irLte` appended at end of iota + scope save/restore in compiler
2. **NaN-box for boxed executor** — object side-table causes correctness bugs for fn calls; only works in the typed executor
3. **Arena allocation** — shared backing arrays corrupt data when vectors are modified by transient assoc
4. **Transient auto-promotion in typed executor** — breaks spectral-norm's conj pattern
5. ~~**Inline frame collision**~~ — **FIXED**: `tryInlineCall` now clears conflicting frame bindings before compiling inlined body

## Recent Breakthroughs

- **IR compiler scope save/restore** — N-body 41.7ms → 9.3ms (4.5×). Parser reuses frame numbers across sibling scopes; save/restore in `compileLetBody` and `compileNestedLoop` prevents slot collision.
- **StringCursor type + IR opcodes** — Cursor-based JSON parser 3-3.5× faster than nth-based. `irCursorChar`, `irCursorNext`, `irCursorDone` run zero-alloc in typed executor.
- **irCallSlot captured fn fix** — Fannkuch 118ms → 78ms. Typed executor now resolves captured fns from `slots` array instead of `initSlots`.

## PersistentVector (HAMT trie)

A 32-way persistent vector trie with tail optimization is implemented in
`core/persistent_vector.go`. It provides O(log32 n) assoc with structural
sharing between versions.

### Performance characteristics

| Operation | ArrayVector (flat) | PersistentVector (trie) |
|---|---|---|
| Nth | O(1) direct | O(log32 n) traverse |
| Assoc | O(n) full clone | O(log32 n) path copy |
| Conj | O(n) clone+append | O(1) amortized (tail) |
| Memory/version | O(n) | O(log32 n) shared |

### Crossover point

- n ≤ 64: ArrayVector faster (flat clone is cache-friendly)
- n > 64: PersistentVector faster (structural sharing dominates)

For n=35 (n-body): ArrayVector 1.9µs vs PV 2.3µs — flat clone wins.
For n=200: ArrayVector 5.2µs vs PV 2.4µs — PV 2.1× faster.

### Integration status

Standalone implementation, not yet replacing ArrayVector in the runtime.
Future work: auto-promote to PV when vector size exceeds threshold, or
use PV for map implementations (HAMT for persistent hash maps).

## Transient Collections

Joker exposes Clojure-compatible transient operations for batch mutations:

```clojure
(let [v [1 2 3 4 5]
      tv (transient v)        ;; create mutable version
      tv (assoc! tv 2 99)     ;; in-place mutation (zero copy)
      tv (conj! tv 6)         ;; in-place append
      result (persistent! tv)] ;; freeze back to immutable
  result)  ;; => [1 2 99 4 5 6]
```

### API

| Function | Description |
|---|---|
| `(transient coll)` | Create mutable transient from persistent vector/map |
| `(assoc! tv k v)` | Mutate in-place, return tv |
| `(conj! tv v)` | Append in-place, return tv |
| `(persistent! tv)` | Freeze to immutable, invalidate transient |

### When to use

Transients help when doing **many mutations in a bounded scope**:
```clojure
;; Good: 1000 mutations, one persistent! at the end
(persistent!
  (loop [i 0 v (transient [])]
    (if (= i 1000) v
      (recur (+ i 1) (conj! v i)))))

;; Bad: few mutations per transient lifecycle (overhead > savings)
(loop [p perm ...]
  (persistent! (assoc! (transient p) 0 x)))  ;; worse than (assoc p 0 x)
```

### Contract
- Single-threaded use only (no sharing across goroutines)
- Must not use transient after `persistent!`
- Must not use the original persistent collection after `transient`

### IR integration

The boxed IR executor (`irExec`) automatically promotes safe loop
variables to transients via escape analysis — no manual transient
calls needed for simple loop patterns. The auto-promotion is
transparent and produces the same persistent result at loop exit.
