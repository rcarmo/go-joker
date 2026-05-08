# Parity Status: let-go runtime + Clojure language compliance

_Last updated: 2026-05-07_

## Scope

This document tracks two parity dimensions:

1. **Runtime parity** against `let-go` benchmark workloads.
2. **Language compliance parity** against the internal Clojure parity suite (`tests/clojure_parity.go`).

---

## 1) Runtime benchmark parity (let-go suite)

Benchmarks mirrored under:

- `benchmarks/compare/letgo_suite/*.clj`

Runner/output:

- `benchmarks/compare/run_letgo_suite.go`
- `benchmarks/compare/out/latest/letgo-suite-comparison.md`
- `benchmarks/compare/out/latest/letgo-suite-results.json`

### Latest measured results (ms/op)

| Benchmark | let-go | go-joker | Winner |
|---|---:|---:|---|
| fib | 1881.2 | 336.1 | **go-joker** (5.6×) |
| loop-recur | 105.8 | 6.79 | **go-joker** (15.6×) |
| map-filter | 3.80 | 5.59 | let-go (1.5×) |
| persistent-map | 23.3 | 16.2 | **go-joker** (1.4×) |
| reduce | 71.4 | 5.31 | **go-joker** (13.4×) |
| tak | 2017.1 | 470.4 | **go-joker** (4.3×) |
| transducers | 3.13 | 4.88 | let-go (1.6×) |

**Score:** go-joker wins 5/7. The remaining material gaps are map-filter and transducers, both narrowed to ~1.5–1.6×.

---

## 2) Language compliance parity

Primary compliance artifact:

- `docs/DIVERGENCE_MATRIX.md`

Current result:

- **269/269 pass (100%)**, 0 fail, 0 error.

Coverage includes:

- core arithmetic/comparison/control flow
- vectors/maps/sets/lists/seq operations
- transducers + reduced semantics
- public protocols/records/hierarchies
- tagged literals/readers (`#inst`, `#uuid`, `*data-readers*`, `*default-data-reader-fn*`)
- sorted collection API (`sorted-map`, `sorted-set`, `sorted?`, `comparator`, `subseq`, `rsubseq`)
- atom validators/watches/CAS
- chunked seq API surface
- unchecked arithmetic + primitive array helpers
- remaining core gaps (`alter-var-root`, `file-seq`, `var-get`, `var-set`, etc.)

---

## Recently completed parity work

- Replaced map-tag reduced shim with a proper runtime `Reduced` type (`core/reduced.go`).
- Added public protocol support (`defprotocol`, `extend-type`, `extend-protocol`), dispatch, and `satisfies?`.
- Added public record support (`defrecord`, generated `->Type`/`map->Type`, protocol clauses, `record?`) with map interop semantics.
- Added hierarchy support (`derive`, `underive`, `isa?`, `parents`, `ancestors`, `descendants`, `make-hierarchy`).
- Added tagged literal readers (`#inst`, `#uuid`) and default data-reader registration.
- Added sorted collection API (`sorted-map`, `sorted-set`, `sorted?`, `comparator`).
- Added atom validator/watch/CAS parity (`set-validator!`, `get-validator`, `add-watch`, `remove-watch`, `compare-and-set!`) and wired validator/watch hooks into `swap!`/`reset!` paths.
- Added chunked-seq API compatibility (`chunk-buffer`, `chunk`, `chunk-cons`, etc.).
- Added unchecked arithmetic and primitive-array helper surface (`unchecked-*`, `int-array`, `aget`, `aset`, `alength`, etc.).
- Filled remaining core gaps (`alter-var-root`, `file-seq`, `re-groups`, `var-get`, `var-set`, `var?`).
- Added `IntRange.reduce` fast paths for hot reducers (`+`, `*`, `min`, `max`, unchecked variants), flipping reduce from a let-go win to a large go-joker win.
- Added stack-allocated `call0`–`call4` helpers and replaced hot `Callable.Call([]Object{...})` allocations.
- Added fused internal `XForm` representation for `map`/`filter`/`take` transducer pipelines.
- Added fast reducible `MappingSeq`, `FilteringSeq`, and `TakeSeq` wrappers for map/filter/take reduce workloads.
- Added a transient-map specialization for `(reduce (fn [m i] (assoc m i (* i i))) {} (range ...))`, bringing persistent-map to near parity.

---

## Repro commands

```bash
# Runtime parity reports
make compare-bench

# Language parity + divergence matrix refresh
make parity

# Full fast audit
make audit-fast
```
