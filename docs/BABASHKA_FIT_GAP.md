# Babashka fit-gap assessment

Generated: 2026-05-10

## Scope

This assesses go-joker as a **Babashka-compatible scripting/runtime target**, not as a full Babashka replacement. The practical target is to run portable Babashka-style Clojure scripts and Babashka pod clients where they fit go-joker's architecture.

Babashka itself is a GraalVM-native Clojure environment with a large curated SCI-compatible library set, Java interop compatibility, task/deps tooling, pod support, nREPL, filesystem/process helpers, and many batteries. go-joker is a Go-native Joker/Clojure runtime with strong standalone scripting, web/runtime extensions, IR/JIT/WASM work, and a different host interop model.

## Executive summary

go-joker is increasingly viable for **portable Babashka-style scripts** that rely on Clojure data, core functions, HTTP/JSON/YAML/CSV, filesystem/process basics, channels, and pods. It should not try to become a byte-for-byte Babashka clone.

The best fit is:

- scripts with mostly portable Clojure code
- CLIs and automation that use data formats and HTTP
- pod consumers using `pods`/`babashka.pods` with JSON, EDN, or Transit+JSON payloads
- server/runtime scripts that benefit from go-joker's HTTP/WebSocket/SSE/router capabilities

The poor fit/non-goal is:

- Java classpath/interop-heavy Babashka scripts
- full `clojure.java.*` and JVM object compatibility
- Babashka task/deps/project manager parity
- complete bundled library parity
- SCI interpreter hooks/macros/execution model compatibility

## Fit-gap table

| Babashka area | go-joker status | Fit | Plan |
|---|---|---:|---|
| Core Clojure data/code | Strong parity suite; records/protocols/hierarchies/tagged literals added | High | Continue parity tests |
| EDN data | `joker.edn` and `edn` namespaces provide `read-string`/`write-string` over the reader/printer without eval | High | Expand options/tag tests as real pod/script payloads require |
| JSON/YAML/CSV/base64/hex/url/string/time | Available with growing direct tests | High | Continue coverage hardening |
| HTTP client/server | Available; go-joker has persistent clients, WebSocket/SSE/router | High | Keep go-joker API; add adapters if needed |
| `clojure.core.async` | Broad compatibility shim over Go goroutines | Medium/High | Maintain practical API parity, not IOC internals |
| Pods | `pods`/`babashka.pods`, lifecycle, cache discovery, invoke, dynamic vars, JSON/EDN/Transit+JSON payloads | High | Add edge-case fixtures only when real pod clients require them |
| Transit | Fuller pod-oriented Transit+JSON subset with cache refs, tags, numeric payloads, helpers | Medium/High | Expand only from pod/script fixtures |
| nREPL | Missing | Medium | Optional later editor integration |
| File/process scripting | Partial via std namespaces and core helpers | Medium | Fill only script-driven gaps |
| Terminal/syscall/unix | Missing/partial | Low/Medium | Optional platform shims only when real scripts need them |
| Babashka tasks (`bb.edn`, `bb tasks`) | Missing | Low | Non-goal unless explicitly requested |
| Babashka deps/classpath tooling | Missing | Low | Non-goal |
| Java interop/classes | Not compatible by design | Low | Explicit non-goal |
| SCI-specific APIs | Missing | Low | Explicit non-goal |
| Built-in Babashka library catalog | Partial/different | Low/Medium | Add small shims only for target scripts |

## Areas we should implement

### 1. Pods compatibility

Target: run common Babashka pod clients and pod-backed scripts.

Required:

- `pods` and `babashka.pods` namespaces
- `load-pod`
- `invoke`
- bencode subprocess protocol
- pod registry/lifecycle/shutdown
- request IDs and response routing
- dynamic namespace/var installation from `describe`
- JSON, EDN, and Transit+JSON payload formats
- end-to-end fake-pod tests for all supported formats

Out of scope for first pass:

- asynchronous streaming handler parity beyond simple success/error/done
- every pod edge-case from Babashka's implementation
- pod installation/download management beyond cache discovery and explicit command paths

### 2. EDN namespace

Target: portable data IO for scripts and pod payload support.

Implemented baseline:

- `edn/read-string` and `joker.edn/read-string`
- `edn/write-string` and `joker.edn/write-string`
- no evaluation of forms; reuses Joker reader/printer
- tagged literal behavior follows existing Joker data-reader handling
- pod EDN payload encode/decode is wired

### 3. Fuller Transit

Target: pod payload interoperability and common Babashka data exchange.

Implemented baseline:

- rolling cache refs
- set/list/cmap tags
- quote tag
- BigInt/BigDecimal/BigFloat/ratio payload handling
- verbose writer for debugging
- pod argument/result helper functions

### 4. Practical namespace shims

Implement only when scripts require them:

- `babashka.fs`-like helpers if filesystem scripts fail
- `babashka.process`-like helpers if process scripts fail
- `clojure.java.io`-like convenience wrappers where they map cleanly to Go IO
- `term`, `unix`, `syscall` minimal shims for terminal/system scripts

## Areas we explicitly should not address

These are non-parity areas by design, unless the project direction changes.

### 1. JVM/Java interop parity

We should not implement:

- arbitrary Java class loading
- Java reflection
- Java constructors/static methods/fields
- JVM classpath semantics
- Java object identity/lifecycle compatibility
- `clojure.java.*` full behavior

Reason: go-joker is Go-native. A fake Java layer would be brittle and high-maintenance.

### 2. Babashka task runner/deps parity

We should not implement full:

- `bb.edn` task runner
- `bb tasks`
- `babashka.deps`
- dependency resolution/classpath assembly
- Maven/Clojars resolution behavior

Reason: this is project/tooling scope, not runtime language parity. Existing Go/native packaging and shell workflows are a better fit.

### 3. SCI compatibility internals

We should not chase:

- SCI analyzer/evaluator hooks
- SCI namespaces as implementation detail
- SCI-specific macro/runtime extension APIs
- exact Babashka evaluation quirks from SCI

Reason: go-joker has its own parser/evaluator/IR pipeline. Behavior parity matters; SCI internals do not.

### 4. Full bundled library catalog

We should not attempt one-shot parity with every Babashka bundled namespace/library.

Examples to avoid unless demanded by workloads:

- full `babashka.curl` compatibility
- complete `babashka.fs` clone
- complete `babashka.process` clone
- complete `cheshire`, `clojure.data.*`, `selmer`, `rewrite-clj`, etc. catalog parity

Reason: library catalog parity is unbounded. Add targeted shims only when real scripts fail and the implementation is small/maintainable.

### 5. Perfect CLI flag/UX parity with `bb`

We should not clone all `bb` CLI behaviors:

- exact `bb` command-line parser
- task discovery UX
- dependency CLI UX
- compatibility aliases that only matter for `bb` tool invocation

Reason: go-joker has its own CLI/runtime identity.

### 6. Platform-specific low-level parity unless needed

We should not proactively implement broad syscall/unix surfaces.

Allowed:

- minimal constants/functions needed by real scripts
- platform-gated shims with tests

Avoid:

- large low-level OS API clone
- unsafe terminal/process/session behavior without a concrete use case

## Recommended compatibility policy

1. **Behavior over branding**: implement portable behavior that lets scripts run, not every Babashka implementation detail.
2. **Small shims only**: each new namespace should have a narrow contract and direct tests.
3. **Script-driven expansion**: add compatibility only when a real script/fixture demonstrates need.
4. **Clear errors**: unsupported Babashka-specific features should fail with explicit messages, not silent partial behavior.
5. **Document non-goals**: every compatibility namespace should state which Babashka features are intentionally omitted.

## Near-term execution order

Completed foundation:

1. `pods`/`babashka.pods` foundation.
2. Standalone EDN namespace.
3. Transit payload support sufficient for pod fixtures.
4. JSON/EDN/Transit+JSON fake-pod end-to-end tests.
5. Portable Babashka-style fixture suite with expected unsupported-feature failures.

Next work is script-driven only: optional shims (`babashka.fs`, `babashka.process`, `clojure.java.io` convenience wrappers, `term`, `unix`, `syscall`, `walk`, `zip`, `dump`, `test`) are assessed from actual fixture failures or user workloads. The current Phase 6 review found no runtime fixture demand for new Babashka namespace shims; see `docs/BABASHKA_SHIM_ASSESSMENT.md`. The Phase 7 optional portability review likewise found no current demand for new optional shims; see `docs/PORTABILITY_SHIM_ASSESSMENT.md`.

## Suggested acceptance criteria

A Babashka compatibility slice is successful when:

- a JSON-format fake pod can be loaded and invoked through `pods/load-pod` and dynamically installed vars
- EDN and Transit+JSON fake pods pass golden tests
- portable scripts using core data, JSON/YAML/HTTP/filesystem basics run unchanged or with documented namespace aliases (`make bb-compat`)
- unsupported Java/task/deps/SCI features fail clearly and are listed as non-goals
- docs and coverage checks pass with no warnings
