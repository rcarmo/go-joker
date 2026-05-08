# Parity Status: let-go runtime + Clojure language compliance

_Last updated: 2026-05-08_

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
| fib | 3124.6 | 511.2 | **go-joker** (6.1×) |
| loop-recur | 87.1 | 7.61 | **go-joker** (11.4×) |
| map-filter | 3.80 | 5.72 | let-go (1.5×) |
| persistent-map | 17.5 | 18.5 | let-go (1.06×) |
| reduce | 107.3 | 6.61 | **go-joker** (16.2×) |
| tak | 3497.5 | 579.6 | **go-joker** (6.0×) |
| transducers | 4.20 | 7.02 | let-go (1.7×) |

**Score:** go-joker wins 4/7. The remaining gaps are map-filter (~1.5×), transducers (~1.7×), and persistent-map (near parity).

---

## 2) Language compliance parity

Primary compliance artifact:

- `docs/DIVERGENCE_MATRIX.md`

Current result:

- **271/271 pass (100%)**, 0 fail, 0 error.
- Imported `jank-lang/clojure-test-suite` smoke subset: **7/7 pass** with local harness.

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
- Added GIL-free concurrency primitives: `alts!`, `timeout`, `future`/`promise`, `agent`, `pmap`, `pcalls`.
- Added `joker.random` namespace (int/float/choice/shuffle/uuid/secure values).
- Added `joker.log` namespace (debug/info/warn/error + level control).
- Added HTTP response extensions for WebSocket upgrade and SSE streaming (`:websocket`, `:stream`).
- Added pure Clojure `joker.http.router` library (routing + middleware).

---

## Repro commands

```bash
# Runtime parity reports
make compare-bench

# Language parity + divergence matrix refresh
make parity

# Imported jank-lang/clojure-test-suite smoke subset
make jank-subset

# Full fast audit
make audit-fast
```
