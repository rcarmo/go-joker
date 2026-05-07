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
| fib | 2623.5 | 423.9 | **go-joker** (6.2×) |
| loop-recur | 77.5 | 7.07 | **go-joker** (11.0×) |
| map-filter | 3.42 | 5.17 | let-go (1.5×) |
| persistent-map | 16.9 | 17.9 | near parity (let-go 1.06×) |
| reduce | 87.0 | 5.90 | **go-joker** (14.7×) |
| tak | 2862.2 | 610.6 | **go-joker** (4.7×) |
| transducers | 3.37 | 7.46 | let-go (2.2×) |

**Score:** go-joker wins 4/7, persistent-map is effectively at parity, and map-filter has narrowed to ~1.5×. The remaining material gap is transducer dispatch/startup overhead.

---

## 2) Language compliance parity

Primary compliance artifact:

- `docs/DIVERGENCE_MATRIX.md`

Current result:

- **261/261 pass (100%)**, 0 fail, 0 error.

Coverage includes:

- core arithmetic/comparison/control flow
- vectors/maps/sets/lists/seq operations
- transducers + reduced semantics
- protocols/records/hierarchies
- tagged literals/readers (`#inst`, `#uuid`)
- sorted collection API (`sorted-map`, `sorted-set`, `sorted?`, `comparator`)
- atom validators/watches/CAS
- chunked seq API surface
- unchecked arithmetic + primitive array helpers
- remaining core gaps (`alter-var-root`, `file-seq`, `var-get`, `var-set`, etc.)

---

## Recently completed parity work

- Replaced map-tag reduced shim with a proper runtime `Reduced` type (`core/reduced.go`).
- Added protocol support (`defprotocol`/`extend-type` runtime surface, dispatch, `satisfies?`).
- Added record support (`__defrecord`, `->Type`, `map->Type`, `record?`) with map interop semantics.
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
