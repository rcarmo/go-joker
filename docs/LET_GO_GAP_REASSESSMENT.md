# let-go gap reassessment

Generated: 2026-05-10 after go-joker commits through coverage/test cleanup.

## Summary

Recent work closed several previously high-priority gaps:

- `clojure.core.async` compatibility namespace: now broader than let-go's `async` shim in several areas (`mult`, `pub`, `sub`, callbacks, aliases).
- Persistent HTTP client sessions: go-joker now has explicit reusable `joker.http/client` objects and keep-alive reuse tests; let-go uses `http.DefaultClient` with simpler `http/get`, `http/post`, `http/request`.
- Transit: go-joker now has `joker.transit` with Transit+JSON primitives, keywords, symbols, arrays, escaped strings, and array-map representation.
- JVM-shaped `System`: go-joker now has a `System` compatibility namespace with properties, env, time, line separator, and exit.
- Numeric tower: go-joker now promotes integer overflow to `BigInt` and improves `BigFloat`/BigDecimal precision behavior.

The main remaining let-go ecosystem lead is **Babashka pods**, backed by fuller Transit/EDN payload support. Secondary gaps are standalone EDN namespace/API shape, terminal/syscall/unix compatibility shims, and let-go's small `zip`/`walk`/`dump`/`test` helper namespaces.

## High-level parity table

| Area | go-joker status | let-go status | Assessment |
|---|---|---|---|
| Core language parity | 271/271 internal parity pass; jank subset pass | broad Clojure-like runtime | go-joker strong/ahead for tracked suite |
| Runtime benchmarks | go-joker wins 5/7 let-go-suite benchmarks | wins map-filter/transducers | remaining perf gaps are map/filter and transducer pipelines |
| `core.async`/async | `clojure.core.async` namespace plus core channel primitives | `async` namespace | go-joker now ahead in API surface, although semantics are goroutine-backed rather than IOC |
| HTTP server/client | `joker.http` server, router, WebSocket/SSE, persistent client sessions | `http` serve/get/post/request, stream opts | go-joker ahead on server/runtime features and persistent sessions |
| Transit | `joker.transit` subset | `transit` with cache, tags, set/list/cmap, pod helpers | let-go still ahead in full Transit protocol depth |
| System namespace | `System` shim implemented | `System` shim | broadly closed |
| BigInt/BigDecimal | overflow promotion + BigFloat precision improvements | BigInt/BigDecimal support | closer; still needs deeper mixed numeric edge-case parity tests |
| Babashka pods | missing | `pods` and `babashka.pods`, subprocess protocol, bencode routing | largest remaining ecosystem gap |
| EDN namespace | reader exists in core; no standalone `edn` std namespace | `edn` runtime file/API | gap remains, important for pods/tooling |
| nREPL/editor integration | missing | not a major let-go runtime namespace in inspected rt set | still ecosystem gap, but not a direct let-go lead |
| `syscall`/`unix`/`term` | missing/partial via std/os/io | let-go has shims | optional platform compatibility gap |
| `walk`/`zip`/`dump`/`test` | missing or core/test alternatives | let-go has small helper namespaces | low/medium gap depending target scripts |
| Docs/metadata hygiene | warnings now fail docs build | n/a | go-joker guardrail improved |

## Details by remaining gap

### 1. Babashka pods — highest priority remaining gap

let-go has a substantial `pods` implementation:

- namespaces: `pods`, `babashka.pods`
- API: `load-pod`, `invoke`
- pod subprocess lifecycle and registry
- bencode message protocol
- response routing with pending IDs
- support for pod formats: `json`, `edn`, `transit+json`
- cache discovery under `~/.babashka/pods` / `BABASHKA_PODS_DIR`
- dynamic namespace/var installation from pod descriptions

Recommended go-joker plan:

1. Add minimal bencode dependency/helpers.
2. Implement pod process lifecycle and registry.
3. Support `json` first for invocation payloads.
4. Add standalone `joker.edn`/`edn` helpers or internal EDN encode/decode path.
5. Upgrade `joker.transit` to the fuller tag/cache subset needed by pod payloads.
6. Install both `pods` and `babashka.pods` compatibility namespaces.

### 2. Transit protocol depth

go-joker now has a useful Transit+JSON subset, but let-go supports more of the protocol:

- rolling string/key cache (`^` refs)
- keyword/symbol string caching
- tagged values (`~#set`, `~#list`, quote, `cmap`)
- BigInt/BigDecimal and ratio-oriented payload handling
- pod helper functions for Transit-encoded argument/result payloads

Recommended next steps:

- Add set and list tags.
- Add verbose/non-cached write mode for debugging parity.
- Add cache read/write support for compatibility with pod outputs.
- Add tests using known Transit payloads from let-go.

### 3. Standalone EDN API

Core read/pr-str behavior exists, but let-go has an `edn` runtime component used by pod payloads and ecosystem code. go-joker should expose a small namespace rather than forcing callers through general reader/eval paths.

Recommended API:

- `joker.edn/read-string` or `edn/read-string`
- `joker.edn/write-string` / `pr-str` equivalent
- options for keywordization/tag readers only if needed

### 4. Platform namespaces: `term`, `syscall`, `unix`

let-go has compatibility shims for:

- terminal raw mode, restore, read-key, size, cursor movement
- syscall constants/functions
- unix constants/functions

These are optional for server/automation workloads but useful for script portability. Implement only if target scripts require them.

### 5. Helper namespaces: `walk`, `zip`, `dump`, `test`

let-go includes these small runtime/helper namespaces. go-joker has strong core and std coverage, but these namespace names may matter for script portability.

Recommendation: add small compatibility namespaces only when real scripts fail to load.

## Areas where go-joker is now ahead

- Web runtime: WebSocket, SSE/chunked streaming, router/middleware.
- Persistent HTTP clients with explicit client/session objects.
- Generated documentation coverage and warning guardrails.
- Benchmark correctness validation before timing comparisons.
- IR/JIT/WASM internals and artifact export.
- Broader recent `clojure.core.async` compatibility surface than let-go's inspected `async` namespace.
- Built-in extra std namespaces (`imaging`, `pdf`, `svg`, `random`, `log`, `jit`, etc.).

## Recommended next implementation order

1. **Pods foundation** (`pods` + `babashka.pods`) with JSON payloads and bencode routing.
2. **Fuller Transit** needed by pods (`set`, `list`, `cmap`, cache refs, BigInt/BigDecimal tags).
3. **Standalone EDN namespace** with read/write helpers.
4. **Coverage pass for pods/transit/edn** with golden payload tests.
5. Optional platform namespaces (`term`, `syscall`, `unix`) only after ecosystem scripts demand them.


## Execution plan

### Phase 1 — Pods foundation

- [ ] Add bencode encode/decode support for pod protocol messages.
- [ ] Implement pod registry, lifecycle, shutdown, request IDs, and response routing.
- [ ] Implement `pods/load-pod` for explicit command paths and cached Babashka pod discovery.
- [ ] Implement `pods/invoke` low-level invocation with synchronous result handling.
- [ ] Install compatibility namespaces `pods` and `babashka.pods`.
- [ ] Dynamically install namespaces/vars returned by pod `describe` messages.
- [ ] Start with JSON payload format, with clear errors for unsupported `edn`/`transit+json` until phases 2–3 land.
- [ ] Add unit tests with a local fake pod subprocess fixture.

### Phase 2 — Fuller Transit protocol

- [ ] Add Transit rolling cache read/write support (`^` refs) for strings and map keys.
- [ ] Add tagged value support for `~#set`, `~#list`, quote, and `cmap`.
- [ ] Add BigInt, BigDecimal/BigFloat, ratio, keyword, and symbol golden payload tests.
- [ ] Add `write-verbose`/non-cached output mode for debugging and parity with let-go.
- [ ] Expose internal helper functions for pod argument/result Transit payloads.
- [ ] Cross-check known let-go Transit fixtures and pod payload samples.

### Phase 3 — Standalone EDN namespace

- [ ] Add `joker.edn` and/or `edn` namespace with `read-string` and `write-string`.
- [ ] Reuse Joker reader/pr-str semantics without evaluating forms.
- [ ] Add options only where needed for pod compatibility/tag handling.
- [ ] Add golden tests for primitives, collections, symbols, keywords, tagged literals, BigInt, ratios, and BigFloat.
- [ ] Wire EDN payload format into pods.

### Phase 4 — Integrated compatibility and coverage

- [ ] Add end-to-end fake pods for JSON, EDN, and Transit+JSON formats.
- [ ] Add namespace-resolution smoke tests for dynamically installed pod vars.
- [ ] Add docs pages and docs-check coverage for `pods`, `babashka.pods`, Transit additions, and EDN.
- [ ] Extend coverage report tracking for these new packages.
- [ ] Run parity, jank subset, coverage, docs-check, full tests, and vet.

### Phase 5 — Optional script-portability shims

- [ ] Assess real script demand for `term`, `syscall`, and `unix` namespaces.
- [ ] If needed, implement small platform-gated compatibility shims with conservative APIs.
- [ ] Assess real script demand for helper namespaces `walk`, `zip`, `dump`, and `test`.
- [ ] Implement only the helpers required by actual workloads or failing compatibility fixtures.
## let-go / Babashka gap closure plan

### Phase 1 — Pods foundation
- [x] Add bencode encode/decode support for pod protocol messages
- [ ] Implement pod registry, lifecycle, shutdown, request IDs, and response routing
- [ ] Implement `pods/load-pod` for command paths and Babashka pod cache discovery
- [ ] Implement `pods/invoke` with synchronous result handling
- [ ] Install compatibility namespaces `pods` and `babashka.pods`
- [ ] Dynamically install namespaces/vars returned by pod `describe`
- [ ] Start with JSON payloads; clear errors for EDN/Transit until later phases
- [ ] Add fake-pod subprocess tests

### Phase 2 — Fuller Transit protocol
- [ ] Add rolling cache read/write support (`^` refs)
- [ ] Add tags: `~#set`, `~#list`, quote, `cmap`
- [ ] Add BigInt, BigDecimal/BigFloat, ratio, keyword, and symbol golden tests
- [ ] Add `write-verbose` / non-cached output mode
- [ ] Expose helper functions for pod Transit args/results

### Phase 3 — Standalone EDN namespace
- [ ] Add `joker.edn` and/or `edn` namespace with `read-string` and `write-string`
- [ ] Reuse reader/pr-str without evaluating forms
- [ ] Add pod-required options/tag handling
- [ ] Add golden tests for primitives, collections, tagged literals, BigInt, ratios, BigFloat
- [ ] Wire EDN payload format into pods

### Phase 4 — Babashka compatibility fixture suite
- [ ] Add a small portable Babashka-script fixture suite
- [ ] Include positive fixtures for core data, JSON/YAML/HTTP/filesystem basics
- [ ] Include expected-failure fixtures for Java interop, `bb.edn` tasks/deps, SCI internals, and unsupported library catalog APIs
- [ ] Ensure unsupported Babashka-specific features fail with explicit non-goal messages
- [ ] Document namespace aliases/workarounds for portable scripts

### Phase 5 — Integrated pod compatibility and coverage
- [ ] Add end-to-end fake pods for JSON, EDN, Transit+JSON
- [ ] Add dynamic namespace/var smoke tests
- [ ] Add docs-check coverage for pods, babashka.pods, Transit additions, EDN
- [ ] Extend coverage report tracking for pods/transit/edn packages
- [ ] Run parity, jank subset, coverage, docs-check, full tests, vet

### Phase 6 — Practical Babashka namespace shims, script-driven only
- [ ] Assess real script demand for `babashka.fs`-like helpers
- [ ] Assess real script demand for `babashka.process`-like helpers
- [ ] Assess real script demand for `clojure.java.io` convenience wrappers that map cleanly to Go IO
- [ ] Implement only small shims required by actual workloads or fixture failures
- [ ] Document explicitly omitted Babashka library catalog areas (`babashka.curl`, full `cheshire`, `clojure.data.*`, `selmer`, `rewrite-clj`, etc.)

### Phase 7 — Optional portability shims
- [ ] Assess demand for `term`, `syscall`, `unix`
- [ ] Implement platform-gated shims only if real scripts require them
- [ ] Assess demand for `walk`, `zip`, `dump`, `test`
- [ ] Implement helpers only for actual workload/fixture failures

### Explicit non-goals to preserve
- [ ] Do not implement arbitrary JVM/Java class loading, reflection, constructors, static members, or JVM classpath semantics
- [ ] Do not implement full `bb.edn` task runner, `bb tasks`, Babashka deps, Maven/Clojars resolution, or classpath assembly
- [ ] Do not chase SCI analyzer/evaluator internals or SCI-specific extension hooks
- [ ] Do not clone the full Babashka bundled library catalog without script-driven justification
- [ ] Do not clone exact `bb` CLI flag/UX behavior; keep go-joker's CLI identity
- [ ] Do not implement broad low-level syscall/unix surfaces without concrete use cases

## let-go / Babashka gap closure plan

### Phase 1 — Pods foundation
- [x] Add bencode encode/decode support for pod protocol messages
- [ ] Implement pod registry, lifecycle, shutdown, request IDs, and response routing
- [ ] Implement `pods/load-pod` for command paths and Babashka pod cache discovery
- [ ] Implement `pods/invoke` with synchronous result handling
- [ ] Install compatibility namespaces `pods` and `babashka.pods`
- [ ] Dynamically install namespaces/vars returned by pod `describe`
- [ ] Start with JSON payloads; clear errors for EDN/Transit until later phases
- [ ] Add fake-pod subprocess tests

### Phase 2 — Fuller Transit protocol
- [ ] Add rolling cache read/write support (`^` refs)
- [ ] Add tags: `~#set`, `~#list`, quote, `cmap`
- [ ] Add BigInt, BigDecimal/BigFloat, ratio, keyword, and symbol golden tests
- [ ] Add `write-verbose` / non-cached output mode
- [ ] Expose helper functions for pod Transit args/results

### Phase 3 — Standalone EDN namespace
- [ ] Add `joker.edn` and/or `edn` namespace with `read-string` and `write-string`
- [ ] Reuse reader/pr-str without evaluating forms
- [ ] Add pod-required options/tag handling
- [ ] Add golden tests for primitives, collections, tagged literals, BigInt, ratios, BigFloat
- [ ] Wire EDN payload format into pods

### Phase 4 — Babashka compatibility fixture suite
- [ ] Add a small portable Babashka-script fixture suite
- [ ] Include positive fixtures for core data, JSON/YAML/HTTP/filesystem basics
- [ ] Include expected-failure fixtures for Java interop, `bb.edn` tasks/deps, SCI internals, and unsupported library catalog APIs
- [ ] Ensure unsupported Babashka-specific features fail with explicit non-goal messages
- [ ] Document namespace aliases/workarounds for portable scripts

### Phase 5 — Integrated pod compatibility and coverage
- [ ] Add end-to-end fake pods for JSON, EDN, Transit+JSON
- [ ] Add dynamic namespace/var smoke tests
- [ ] Add docs-check coverage for pods, babashka.pods, Transit additions, EDN
- [ ] Extend coverage report tracking for pods/transit/edn packages
- [ ] Run parity, jank subset, coverage, docs-check, full tests, vet

### Phase 6 — Practical Babashka namespace shims, script-driven only
- [ ] Assess real script demand for `babashka.fs`-like helpers
- [ ] Assess real script demand for `babashka.process`-like helpers
- [ ] Assess real script demand for `clojure.java.io` convenience wrappers that map cleanly to Go IO
- [ ] Implement only small shims required by actual workloads or fixture failures
- [ ] Document explicitly omitted Babashka library catalog areas (`babashka.curl`, full `cheshire`, `clojure.data.*`, `selmer`, `rewrite-clj`, etc.)

### Phase 7 — Remaining let-go runtime namespace gaps
- [ ] Assess `async` namespace alias/wrapper need beyond `clojure.core.async`
- [ ] Assess `http` namespace compatibility wrappers over `joker.http` (`http/get`, `http/post`, `http/request`, `http/serve` shape)
- [ ] Assess `math`, `json`, `os`, `io`, `string`, `set`, and `pprint` namespace naming/API aliases where let-go scripts use unqualified short namespaces
- [ ] Assess `data`, `dump`, `types`, `lang`, and `iort` runtime helper namespaces for real script demand
- [ ] Implement only thin aliases/adapters when they map cleanly to existing go-joker std/core APIs

### Phase 8 — let-go performance parity gaps
- [ ] Re-run `make compare-bench` after each runtime batch
- [ ] Investigate remaining let-go wins: `map-filter` benchmark
- [ ] Investigate remaining let-go wins: `transducers` benchmark
- [ ] Add correctness tests before any new benchmark-specific optimization
- [ ] Keep portable benchmark results separate from best-Joker/native variants

### Phase 9 — Optional portability shims
- [ ] Assess demand for `term`, `syscall`, `unix`
- [ ] Implement platform-gated shims only if real scripts require them
- [ ] Assess demand for `walk`, `zip`, `dump`, `test`
- [ ] Implement helpers only for actual workload/fixture failures

### Explicit non-goals to preserve
- [ ] Do not implement arbitrary JVM/Java class loading, reflection, constructors, static members, or JVM classpath semantics
- [ ] Do not implement full `bb.edn` task runner, `bb tasks`, Babashka deps, Maven/Clojars resolution, or classpath assembly
- [ ] Do not chase SCI analyzer/evaluator internals or SCI-specific extension hooks
- [ ] Do not clone the full Babashka bundled library catalog without script-driven justification
- [ ] Do not clone exact `bb` CLI flag/UX behavior; keep go-joker's CLI identity
- [ ] Do not implement broad low-level syscall/unix surfaces without concrete use cases
