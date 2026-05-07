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

| Benchmark | let-go | go-joker | Status |
|---|---:|---:|---|
| fib | 2974.5 | 24177.2 | both ran |
| loop-recur | 60.2 | 6.64 | both ran |
| map-filter | 3.72 | 7.74 | both ran |
| persistent-map | 16.9 | 48.8 | both ran |
| reduce | 82.4 | 3220.9 | both ran |
| tak | 2360.0 | 25678.7 | both ran |
| transducers | 3.24 | 47.7 | both ran |

## Runtime fixes applied

The earlier failures are now fixed in go-joker runtime:

- `fib` / `tak` parse panic fixed by hardening macro-origin checks (`core/parse.go`)
- `transducers` benchmark unblocked by adding compatibility support for:
  - one-arg transducer-style `map`, `filter`, `take`
  - `transduce`
  - `reduced`, `reduced?`, `ensure-reduced`, `unreduced`
  - `completing`, `eduction`, and `sequence` 2-arity
  (`core/transducer_compat.go`)

The parity runner now attempts all mirrored workloads and reports failures directly instead of policy skipping.

## Clojure language parity implications

This was the missing dimension in the earlier analysis. Runtime benchmark parity alone is insufficient; language-feature parity must be tracked explicitly.

Observed parity signals from this run:

- Core language/runtime compatibility now executes all mirrored let-go suite workloads.
- Remaining gap is primarily **performance parity**, not immediate feature absence for these cases.

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
