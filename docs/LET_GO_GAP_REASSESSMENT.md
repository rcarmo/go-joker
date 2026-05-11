# let-go gap reassessment

Updated: 2026-05-11 after Babashka pods/Transit/EDN and compatibility fixture work.

## Summary

Recent work closed the previously high-priority Babashka/let-go runtime gaps around pods, Transit payloads, and EDN:

- `pods` and `babashka.pods` compatibility namespaces are installed.
- Pod bencode protocol helpers, process lifecycle, request IDs, response routing, shutdown, cache discovery, `load-pod`, `invoke`, and dynamic namespace/var installation are implemented.
- Pod payloads support `json`, `edn`, and `transit+json`, with end-to-end fake-pod tests for all three formats.
- `joker.transit` now covers the practical Transit+JSON subset needed by pods: rolling cache refs, set/list/quote/cmap tags, BigInt/BigDecimal/BigFloat/ratio/keyword/symbol handling, verbose writer, and pod helper functions.
- `joker.edn` and alias `edn` provide `read-string` and `write-string` over Joker's reader/printer without evaluating forms.
- A portable Babashka-style fixture suite (`make bb-compat`) exercises core data, codecs, filesystem, HTTP, and explicit non-goal failures.

The main remaining let-go/Babashka work is now script-driven namespace shims and performance follow-up, not foundational runtime support.

## High-level parity table

| Area | go-joker status | let-go status | Assessment |
|---|---|---|---|
| Core language parity | 271/271 internal parity pass; jank subset pass | broad Clojure-like runtime | go-joker strong/ahead for tracked suite |
| Runtime benchmarks | go-joker wins 5/7 let-go-suite benchmarks | wins map-filter/transducers | remaining perf gaps are map-filter and transducer pipelines |
| `core.async`/async | `clojure.core.async` namespace plus core channel primitives | `async` namespace | go-joker ahead in surface, though goroutine-backed rather than IOC |
| HTTP server/client | `joker.http` server, router, WebSocket/SSE, persistent client sessions | `http` serve/get/post/request, stream opts | go-joker ahead on server/runtime features; short-name adapters are optional |
| Transit | `joker.transit` fuller pod-oriented Transit+JSON subset | `transit` with cache, tags, set/list/cmap, pod helpers | practical payload gap closed; deepen only from fixtures |
| System namespace | `System` shim implemented | `System` shim | broadly closed |
| BigInt/BigDecimal | overflow promotion + BigFloat precision improvements | BigInt/BigDecimal support | closer; continue mixed numeric edge-case tests |
| Babashka pods | `pods`/`babashka.pods`, lifecycle, cache discovery, dynamic vars, JSON/EDN/Transit payloads | similar pod model | foundational gap closed; remaining gaps are edge-case fixtures |
| EDN namespace | `joker.edn` and `edn` provide `read-string`/`write-string`; pod EDN wired | `edn` runtime file/API | baseline gap closed |
| Babashka fixture suite | `make bb-compat` positive + expected-failure fixtures | n/a | new guardrail in place |
| nREPL/editor integration | missing | ecosystem feature | optional later editor integration |
| `syscall`/`unix`/`term` | missing/partial via std/os/io | let-go has shims | optional platform compatibility gap |
| `walk`/`zip`/`dump`/`test` helpers | some core/test alternatives; exact names missing | let-go has helper namespaces | low/medium; implement only from real script demand |
| Docs/metadata hygiene | docs warnings fail build; EDN/pods/transit docs checked | n/a | guardrail improved |

## Completed gap-closure phases

### Phase 1 — Pods foundation

- [x] Add bencode encode/decode support for pod protocol messages.
- [x] Implement pod registry, lifecycle, shutdown, request IDs, and response routing.
- [x] Implement `pods/load-pod` for command paths and Babashka pod cache discovery.
- [x] Implement `pods/invoke` with synchronous result handling.
- [x] Install compatibility namespaces `pods` and `babashka.pods`.
- [x] Dynamically install namespaces/vars returned by pod `describe`.
- [x] Start with JSON payloads and then wire EDN/Transit as subsequent phases landed.
- [x] Add fake-pod subprocess tests.

### Phase 2 — Fuller Transit protocol

- [x] Add rolling cache read/write support (`^` refs).
- [x] Add tags: `~#set`, `~#list`, quote, `cmap`.
- [x] Add BigInt, BigDecimal/BigFloat, ratio, keyword, and symbol golden tests.
- [x] Add `write-verbose` / non-cached output mode.
- [x] Expose helper functions for pod Transit args/results.

### Phase 3 — Standalone EDN namespace

- [x] Add `joker.edn` and `edn` namespaces with `read-string` and `write-string`.
- [x] Reuse reader/printer semantics without evaluating forms.
- [x] Add pod-required options/tag handling baseline through existing data-reader behavior.
- [x] Add golden tests for primitives, collections, tagged-reader-compatible values, BigInt, ratios, BigFloat.
- [x] Wire EDN payload format into pods.

### Phase 4 — Babashka compatibility fixture suite

- [x] Add a small portable Babashka-script fixture suite.
- [x] Include positive fixtures for core data, JSON/YAML/HTTP/filesystem basics.
- [x] Include expected-failure fixtures for Java interop, `bb.edn` tasks/deps, SCI internals, and unsupported library catalog APIs.
- [x] Ensure unsupported Babashka-specific features fail with explicit non-goal messages.
- [x] Document namespace aliases/workarounds for portable scripts.

### Phase 5 — Integrated pod compatibility and coverage

- [x] Add end-to-end fake pods for JSON, EDN, Transit+JSON.
- [x] Add dynamic namespace/var smoke tests.
- [x] Add docs-check coverage for pods, babashka.pods, Transit additions, EDN.
- [x] Extend coverage report tracking for pods/transit/edn packages.
- [x] Run compatibility fixtures, coverage, docs-check, full tests, and vet.

## Remaining work

### Phase 6 — Practical Babashka namespace shims, script-driven only

- [ ] Assess real script demand for `babashka.fs`-like helpers.
- [ ] Assess real script demand for `babashka.process`-like helpers.
- [ ] Assess real script demand for `clojure.java.io` convenience wrappers that map cleanly to Go IO.
- [ ] Implement only small shims required by actual workloads or fixture failures.
- [ ] Document explicitly omitted Babashka library catalog areas (`babashka.curl`, full `cheshire`, `clojure.data.*`, `selmer`, `rewrite-clj`, etc.).

### Phase 7 — Remaining let-go runtime namespace gaps

- [ ] Assess `async` namespace alias/wrapper need beyond `clojure.core.async`.
- [ ] Assess `http` namespace compatibility wrappers over `joker.http` (`http/get`, `http/post`, `http/request`, `http/serve` shape).
- [ ] Assess `math`, `json`, `os`, `io`, `string`, `set`, and `pprint` namespace naming/API aliases where let-go scripts use unqualified short namespaces.
- [ ] Assess `data`, `dump`, `types`, `lang`, and `iort` runtime helper namespaces for real script demand.
- [ ] Implement only thin aliases/adapters when they map cleanly to existing go-joker std/core APIs.

### Phase 8 — let-go performance parity gaps

- [ ] Re-run `make compare-bench` after each runtime batch.
- [ ] Investigate remaining let-go wins: `map-filter` benchmark.
- [ ] Investigate remaining let-go wins: `transducers` benchmark.
- [ ] Add correctness tests before any new benchmark-specific optimization.
- [ ] Keep portable benchmark results separate from best-Joker/native variants.

### Phase 9 — Optional portability shims

- [ ] Assess demand for `term`, `syscall`, `unix`.
- [ ] Implement platform-gated shims only if real scripts require them.
- [ ] Assess demand for `walk`, `zip`, `dump`, `test`.
- [ ] Implement helpers only for actual workload/fixture failures.

## Explicit non-goals to preserve

- [ ] Do not implement arbitrary JVM/Java class loading, reflection, constructors, static members, or JVM classpath semantics.
- [ ] Do not implement full `bb.edn` task runner, `bb tasks`, Babashka deps, Maven/Clojars resolution, or classpath assembly.
- [ ] Do not chase SCI analyzer/evaluator internals or SCI-specific extension hooks.
- [ ] Do not clone the full Babashka bundled library catalog without script-driven justification.
- [ ] Do not clone exact `bb` CLI flag/UX behavior; keep go-joker's CLI identity.
- [ ] Do not implement broad low-level syscall/unix surfaces without concrete use cases.

## Validation commands

```bash
make bb-compat
make docs-check
make coverage TEST_TIMEOUT=120s
go test ./core ./std/... ./tests -timeout 120s -count=1
go vet ./core ./std/... ./tests
```
