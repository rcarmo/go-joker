# Parity Status: let-go runtime + Clojure language compliance

_Last updated: 2026-05-11 (post Babashka pods/EDN/Transit compatibility pass)_

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
| fib | 1934.9 | 1269.6 | **go-joker** (1.5×) |
| loop-recur | 73.8 | 8.70 | **go-joker** (8.5×) |
| map-filter | 5.37 | 7.48 | let-go (1.39×) |
| persistent-map | 29.9 | 26.2 | **go-joker** (1.1×) |
| reduce | 109.1 | 7.81 | **go-joker** (14.0×) |
| tak | 2804.1 | 2547.6 | **go-joker** (1.1×) |
| transducers | 3.15 | 5.40 | let-go (1.71×) |

**Score:** go-joker wins 5/7. The remaining gaps are map-filter (~1.39×) and transducers (~1.71×).

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

- Added `pods` and `babashka.pods` compatibility namespaces with pod lifecycle, bencode routing, cache discovery, `load-pod`, `invoke`, and dynamic var installation.
- Added JSON, EDN, and Transit+JSON pod payload coverage with end-to-end fake pod tests.
- Added `joker.edn` and alias `edn` with `read-string`/`write-string` over reader/printer semantics without eval.
- Expanded `joker.transit` with cache refs, set/list/quote/cmap tags, BigInt/BigDecimal/BigFloat/ratio/keyword/symbol payloads, verbose writing, and pod helper functions.
- Added `make bb-compat` portable Babashka-style fixtures plus expected non-goal failure fixtures for Java interop, `bb.edn` tasks/deps, SCI internals, and broad bundled-library catalog APIs.
- Replaced map-tag reduced shim with a proper runtime `Reduced` type, now co-located with transducer/reduce compatibility in `core/eval.go`.
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
