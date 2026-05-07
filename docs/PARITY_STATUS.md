# Parity Status: let-go benchmark suite + Clojure language parity

_Last updated: 2026-05-07_

## Scope

This note records parity findings requested during the let-go comparison work:

1. **Runtime benchmark parity** against the benchmark set documented in
   `nooga/let-go/benchmark/results.md`.
2. **Clojure language parity** gaps surfaced by those runs.

## Runtime benchmark parity (let-go suite)

Benchmarks mirrored into this repo under:

- `benchmarks/compare/letgo_suite/*.clj`

Runner:

- `benchmarks/compare/run_letgo_suite.go`

Generated outputs:

- `benchmarks/compare/out/latest/letgo-suite-comparison.md`
- `benchmarks/compare/out/latest/letgo-suite-results.json`

### Latest measured results (ms/op)

| Benchmark | let-go | go-joker | Winner |
|---|---:|---:|---|
| fib | 3361.6 | 575.9 | **go-joker** (5.8×) |
| loop-recur | 77.2 | 6.45 | **go-joker** (12.0×) |
| map-filter | 3.52 | 6.54 | let-go (1.9×) |
| persistent-map | 16.7 | 24.1 | let-go (1.4×) |
| reduce | 104.0 | 214.5 | let-go (2.1×) |
| tak | 3610.3 | 705.8 | **go-joker** (5.1×) |
| transducers | 3.98 | 6.18 | let-go (1.6×) |

**Score: go-joker wins 3/7 (by 5–12×), let-go wins 4/7 (by 1.4–2.1×).**

### Cross-language comparison (15-benchmark suite)

| Benchmark | Joker | Python | Bun/Node | Goja | let-go | Winner |
|---|---:|---:|---:|---:|---:|---|
| arithmetic-loop | 0.237 | 8.07 | 0.430 | 26.2 | 15.8 | Joker |
| recursive-fib | 0.959 | 16.6 | 1.44 | 107.7 | 60.2 | Joker |
| tail-recursive-sum | 0.062 | 5.48 | 0.330 | 15.8 | 12.5 | Joker |
| map-update-loop | 0.733 | 0.550 | 0.100 | 3.11 | 4.31 | Bun/Node |
| word-frequency | 13.6 | 1.30 | 0.130 | 2.25 | 35.9 | Bun/Node |
| nbody | 1.76 | 0.570 | 0.190 | 7.16 | 4.86 | Bun/Node |
| spectral-norm | 17.4 | 15.8 | 0.650 | 105.2 | 64.4 | Bun/Node |
| binary-trees | 78.3 | 35.8 | 6.34 | 175.7 | 160.4 | Bun/Node |
| fannkuch | 33.7 | 3.59 | 0.590 | 14.6 | 19.0 | Bun/Node |
| mandelbrot | 3.97 | 5.24 | 0.400 | 27.9 | 20.7 | Bun/Node |
| fasta | 0.066 | 0.200 | 0.040 | 0.650 | 0.320 | Bun/Node |
| knucleotide | 0.251 | 0.040 | 0.110 | 0.570 | 0.680 | Python |
| reverse-complement | 0.043 | 0.020 | 0.040 | 0.110 | 0.190 | Python |
| regex-redux | 0.083 | 0.120 | 0.080 | 0.180 | 0.140 | Bun/Node |
| pidigits | 0.016 | 0.130 | 0.020 | 0.200 | 0.390 | Joker |

**Joker wins 4/15, beats Python 6/15, beats Goja 13/15, beats let-go 12/15.**

## Key optimizations applied

### Native Go codegen for pure-integer recursive fns
Pure-integer recursive fns (fib, tak) are compiled to fixed-arity native Go
closures at runtime, eliminating all Object boxing, interface dispatch, and
slice allocation per call. This gave **fib 53×** and **tak 35×** speedups.

### IR executor improvements
- Frame stack depth increased to 512 for deep recursion
- Skip slot clearing when no captures exist
- Var-based self-call support (`defVar` tracking on `Fn`)

### IntRange + fast reduce
`(range n)` returns `IntRange` for integer args, which implements reduce
directly without seq allocation. Reduce-over-range is **18× faster**.

### Transducer compatibility layer
One-arg transducer arities for `map`/`filter`/`take`, plus `transduce`,
`reduced`/`reduced?`/`ensure-reduced`/`unreduced`, `completing`, `eduction`,
and `sequence` 2-arity support. All let-go transducer workloads now execute.

### Parser hardening
Fixed nil-deref panic in `isCreatedByMacro` (`core/parse.go`) that prevented
`fib` and `tak` from parsing.

## Runtime fixes applied

- `fib` / `tak` parse panic fixed by hardening macro-origin checks (`core/parse.go`)
- `transducers` benchmark unblocked by adding compatibility support for transducer primitives (`core/transducer_compat.go`)
- `reduce-kv` nil-safe guard for `with-bindings*`/`println` path
- Take transducer state bug fixed (fresh counter per builder invocation)
- IntRange step=0 guard added (panic instead of silent zero)

The parity runner now attempts all mirrored workloads and reports failures directly instead of policy skipping.

## Clojure language parity implications

Observed parity signals from this run:

- Core language/runtime compatibility now executes all mirrored let-go suite workloads.
- Remaining gap is primarily **performance parity** for collection-heavy workloads (map-filter, reduce, persistent-map, transducers), not immediate feature absence.

Required parity workstream (tracked in plan):

- Integrate a subset of `jank-lang/clojure-test-suite`
- Maintain a divergence matrix (`implemented` / `partial` / `missing`)
- Add CI parity gate with release-over-release improvement targets

## Repro commands

```bash
# Full compare pipeline (cross-language + let-go suite parity)
make compare-bench

# Direct smoke-check of let-go suite files under go-joker
for b in fib loop-recur map-filter persistent-map reduce tak transducers; do
  /workspace/tmp/go-joker benchmarks/compare/letgo_suite/${b}.clj
done
```
