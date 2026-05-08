# go-joker

![icon](docs/icon-256.png)

An optimized fork of [Joker](https://github.com/candid82/joker) (Clojure-like Lisp interpreter) for inclusion in [gi](https://github.com/rcarmo/gi), a self-hosted coding agent.

## Performance

### Performance — Joker vs Python vs Goja (CLBG benchmarks)

<img src="benchmarks/benchmark-transposed.svg" alt="benchmark comparison" width="100%">

### vs. original Joker

<img src="benchmarks/benchmark-speedup.svg" alt="improvements" width="100%">

### Highlights

| What | Result |
|------|--------|
| **Mandelbrot** | **~3.97 ms** — beats Python (~4.97 ms) via per-instance fn cache + typed inline dispatch |
| **N-body** | **~1.76 ms** — beats Goja (~5.20 ms) and let-go (~2.26 ms) |
| **Fannkuch** | **~33.7 ms** — 2.5× faster than original via IR callSlot caching |
| **Binary trees** | **~78.3 ms** — beats Goja (~148 ms) and let-go (~131 ms) |
| **Pidigits** | **~0.016 ms** — beats Python (~0.13 ms), Goja (~0.23 ms), and let-go (~0.34 ms) |
| **Arithmetic loop** | **~0.237 ms** — faster than Bun/JSC (~0.37 ms), Goja, Python, and let-go |
| **Allocations** | **51% fewer** across all benchmarks (Int/Double 8-byte structs) |
| **Beat Python** | 7/13 CLBG benchmarks |
| **Beat Goja** | 12/13 CLBG benchmarks |
| **Beat let-go** | 14/15 cross-language benchmarks, 5/7 let-go suite (reduce 13.4×, loop-recur 15.6×, fib 5.6×, tak 4.3×) |
| **Language compliance** | **269/269 parity tests passing** (`docs/DIVERGENCE_MATRIX.md`) |

## What's different from upstream Joker

### Native integer codegen for recursive fns
Pure-integer recursive `defn` bodies (fib, tak) are compiled to fixed-arity native Go closures, eliminating all Object boxing and interface dispatch. fib(35) runs in 0.5s (53× faster than tree-walker).

### IR bytecode interpreter (typed + boxed paths)
Hot loops and functions compile to a flat bytecode. Eligible primitive/string loops now run on a typed IR value stack, while collection-heavy or unsupported cases fall back to the boxed IR interpreter and then to the tree-walker.

### WASM/wazero native compilation
Pure numeric loops compile further to WASM bytecode and execute via [wazero](https://github.com/tetratelabs/wazero)'s native code compiler. This achieves JIT-level performance (matching Bun/JSC) with zero CGo dependencies.

### Generic tail-call optimization
Self-recursive functions in tail position are automatically rewritten to `recur` at parse time, eliminating stack growth. A runtime trampoline handles cases the rewriter can't catch.

### Transient vectors and maps
Loops that update non-escaping vectors or maps via `assoc` automatically use in-place mutation (Clojure-style transients), eliminating persistent copy/update overhead while preserving persistent results at loop return.

### StringCursor native type
A zero-alloc O(1) string iterator with IR opcodes (`irCursorChar`, `irCursorNext`, `irCursorDone`). Cursor-based parsers run 3-3.5× faster than equivalent index-based code by eliminating per-character nth scanning and position arithmetic.

### IntRange + seq-walking reduce
`(range n)` with integer arguments returns an `IntRange` that implements reduce directly without seq allocation. Reduce-over-range is 18× faster. Lazy seqs (`LazySeq`, `ConsSeq`, `MappingSeq`) also support fast reduce.

### Full transducer semantics
`map`/`filter`/`take` transducer arities, `transduce` (3/4 arity), `reduced`/`reduced?`/`ensure-reduced`/`unreduced`, `completing`, `eduction`, and `sequence` 2-arity are implemented using a dedicated `Reduced` runtime type (not map-tag shims).

### Evaluator fast paths
Numeric operations, binding resolution, and function dispatch all have type-specialized fast paths that avoid the generic Joker evaluation machinery.

### Runtime introspection (`joker.runtime`)
Full IR/WASM/profiling introspection from Joker scripts: `disassemble`, `analyze`, `wasm-diagnostic`, `escape-analysis`, `profile`, `benchmark`, `mem-stats`, `gc`.

### Additional namespaces
- `joker.imaging` — image processing (resize, crop, blur, overlay) via pure Go
- `joker.svg` — SVG generation + raster rendering
- `joker.pdf` — PDF document generation

### Clojure parity surface now implemented
- Protocols: public `defprotocol`, `extend-type`, `extend-protocol`, `satisfies?`, protocol method dispatch
- Records: public `defrecord`, generated `->Type`/`map->Type` constructors, `record?`, protocol clauses
- Hierarchies: `derive`, `underive`, `isa?`, `parents`, `ancestors`, `descendants`, `make-hierarchy`
- Tagged literals/readers: `#inst`, `#uuid`, `default-data-readers`, `*data-readers*`, `*default-data-reader-fn*`
- Sorted collection API: `sorted-map`, `sorted-set`, `sorted?`, `comparator`, `subseq`, `rsubseq`
- Atom mutation parity: `set-validator!`, `get-validator`, `add-watch`, `remove-watch`, `compare-and-set!`
- Chunked seq API: `chunk-buffer`, `chunk-append`, `chunk`, `chunk-cons`, `chunk-first`, `chunk-rest`, `chunk-next`, `chunked-seq?`
- Unchecked arithmetic + primitive array helpers: `unchecked-*`, `int-array`, `long-array`, `aget`, `aset`, `alength`, `aclone`, `make-array`

## Architecture

<img src="benchmarks/architecture.svg" alt="architecture" width="100%">

- **WASM path**: pure integer/float loops → wazero JIT → native code (~0.2ms)
- **Typed IR path**: primitive/string/cursor loops → irValue stack, zero-boxing (~2–8ms)
- **Boxed IR path**: collections, fn calls, transients → []Object interpreter (~10–40ms)
- **Tree-walker**: full Clojure semantics (macros, special forms, I/O)
- **Fallback chain**: WASM → Typed IR → Boxed IR → Tree-walker (automatic)

## Building & testing

```bash
go test ./core              # run all tests
go test ./core -bench .     # run all benchmarks
make parity                 # run language parity suite + refresh divergence matrix
```

## Benchmarks

> **Note:** The CLBG programs were chosen as a starting point for optimizing the IR and WASM compilation pipeline, not because they represent realistic workloads. They stress specific interpreter bottlenecks (arithmetic loops, recursion, allocation, string processing) that guided the optimization work. Real-world gi scripts will have different profiles — the gains here prove the execution machinery works, not that every Joker program runs 500× faster.

```bash
# Full CLBG suite + micro benchmarks
go test ./core -run '^$' -bench 'BenchmarkCLBG|BenchmarkEval|BenchmarkWasm' -benchmem -benchtime=5x

# Cross-language comparison
python3 benchmarks/cross_lang_bench.py
bun benchmarks/cross_lang_bench.js

# Regenerate charts
go run ./benchmarks/generate_svg.go ./benchmarks
```

## Documentation

- [`docs/OPTIMIZATION_REPORT.md`](docs/OPTIMIZATION_REPORT.md) — full technical report (phases, trade-offs, outcomes, suggested git history)
- [`benchmarks/README.md`](benchmarks/README.md) — benchmark data and chart regeneration
- [`docs/PARITY_STATUS.md`](docs/PARITY_STATUS.md) — let-go benchmark parity + language compliance status
- [`docs/DIVERGENCE_MATRIX.md`](docs/DIVERGENCE_MATRIX.md) — latest compliance matrix (**269/269 pass**)
- [`PERFORMANCE_PLAN.md`](PERFORMANCE_PLAN.md) — optimization roadmap and milestones

## Upstream

Based on [candid82/joker](https://github.com/candid82/joker) v1.7.1.  
Original README preserved as [`ORIGINAL_README.md`](ORIGINAL_README.md).

## Why v42?

Because 42 is the answer, and we didn't want to collide with upstream version numbers.

## License

Same as upstream Joker (EPL-1.0).
