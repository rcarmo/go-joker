# API stability matrix

Updated: 2026-06-10

This document classifies go-joker public namespaces and major user-facing surfaces by stability. It is intentionally conservative: a namespace can be useful and well-tested while still being marked beta if its API shape may change. Its presence is guarded by `make docs-check` so new public surfaces have a canonical place to be classified.

## Stability levels

| Level | Meaning |
|---|---|
| Stable | Expected to remain source-compatible across patch releases. Changes should be additive or clearly documented. |
| Beta | Usable and tested, but API shape may still evolve as examples and real usage harden edge cases. |
| Experimental | Useful for advanced users, diagnostics, or performance experiments. Breaking changes are acceptable with release notes. |
| Internal/diagnostic | Public enough to inspect or debug the runtime, but not a general application API commitment. |

## Core language/runtime

| Surface | Stability | Notes |
|---|---|---|
| `joker.core` Clojure-like basics | Stable | Curated parity matrix is currently `271/271 pass`. |
| Reader syntax and core data literals | Stable | Still receives edge-case hardening; breaking syntax changes should be avoided. |
| Persistent collections and scalar types | Stable | Object/protocol contracts and collection contracts guard behavior. |
| Concurrency primitives (`future`, `promise`, `agent`, channels, `alts!`, `timeout`) | Stable | GIL-free/runtime concurrency behavior is guarded by runtime contract tests. |
| Tail-call/recur behavior | Stable | Important scripting/runtime behavior; regression sensitive. |

## Standard namespaces

| Namespace | Stability | Notes |
|---|---|---|
| `joker.os` | Stable | Core scripting surface; native-boundary tests cover process/filesystem behavior. |
| `joker.filepath` | Stable | Core scripting surface. |
| `joker.io` | Stable | Core scripting surface. |
| `joker.string` | Stable | Core scripting surface; string bounds/version parsing are guarded. |
| `joker.math` | Stable | Numeric helper surface; native-boundary contracts exist. |
| `joker.strconv` | Stable | Parse/format helpers; native-int behavior audited. |
| `joker.time` | Stable | Duration/time conversion guardrails exist. |
| `joker.json` | Stable | Common serialization surface. |
| `joker.yaml` | Beta | Useful std surface; less central than JSON/EDN. |
| `joker.edn` / `edn` | Stable | Core data interchange for Joker workflows. |
| `joker.transit` | Beta | Useful and tested; bounded fuzz smoke covers decode robustness, but edge cases depend on Transit semantics. |
| `joker.base64` | Stable | Small RFC 4648 encoding/decoding API. |
| `joker.hex` | Stable | Small hexadecimal encoding/decoding API. |
| `joker.crypto` | Stable | Common helper surface; avoid semantic churn. |
| `joker.uuid` | Stable | Small focused API. |
| `joker.random` | Stable | Common scripting helper. |
| `joker.log` | Stable | Small focused API. |
| `joker.csv` | Stable | Native-boundary options are guarded. |
| `joker.url` | Stable | URL parsing behavior guarded. |
| `joker.http` | Beta | Useful Ring-style client/server surface with WebSocket/SSE extensions; request-map fuzzing exists, but long-running service hardening should remain explicit. |
| `joker.http.router` | Beta | Pure Joker routing layer; useful but not a core compatibility surface. |
| `joker.markdown` | Stable | Small GFM conversion API. |
| `joker.html` | Stable | Small HTML escape/unescape API. |
| `joker.hiccup` | Beta | Useful markup rendering surface; syntax expectations should track examples and docs. |
| `joker.imaging` | Beta | Broad and actively extended; procedural raster helpers, WebP decode, hashes, and native image operations are useful but API growth should be curated. |
| `joker.svg` | Beta | Useful generation/raster surface; coordinate guards exist. |
| `joker.pdf` | Beta | Useful document generation surface; native-boundary arity/dimension guards exist. |
| `joker.term` | Beta | Raw terminal/ANSI/key API is useful and example-backed; keep flags/key semantics source-compatible where possible, but terminal behavior remains platform-sensitive. |
| `joker.bolt` | Beta | Embedded database API; useful but depends on bbolt semantics and filesystem behavior. |
| `joker.git` | Beta | Repository manipulation API; useful but depends on go-git behavior and external repository shape. |
| `joker.set` | Stable | Clojure-compatible set helpers; core data-structure behavior should remain compatible. |
| `joker.walk` | Stable | Generic data walker; small Clojure-compatible utility surface. |
| `joker.pprint` | Stable | Pretty-printing utilities; output formatting should avoid gratuitous patch-release churn. |
| `joker.better-cond` | Stable | Macro utility surface ported from common Clojure usage. |
| `joker.template` | Stable | Macro template helpers; small compatibility-style surface. |
| `joker.test` | Beta | Useful in-repo/user testing framework; keep common assertions stable, but runner/reporting details may evolve. |
| `joker.tools.cli` | Beta | Command-line parsing helpers; option semantics should be documented before marking stable. |
| `joker.repl` | Internal/diagnostic | Interactive REPL utilities; useful at the prompt, not a broad application API commitment. |
| `joker.system` | Stable | System/runtime helper surface. |
| `joker.runtime` | Internal/diagnostic | Runtime inspection/profiling/memory controls; useful but not a portable app API commitment. |
| `joker.jit` | Experimental | IR/WASM compiler surface. Powerful, but compiler patterns, diagnostics, host imports, and fallback behavior are still maturing. |
| `pods` / `babashka.pods` | Beta | Compatibility-oriented surface; depends on external pod protocols. |

## CLI surfaces

| Command/surface | Stability | Notes |
|---|---|---|
| `joker` script execution | Stable | Primary CLI mode. |
| No-argument REPL startup | Stable | Regression tested after startup namespace fixes. |
| `joker doc` | Stable | Markdown/JSON lookup and local docs server. |
| `joker notebook` | Beta | Rich and heavily tested; trusted local execution model, browser UI, auth/readonly flags, and recovery/snapshot behavior are still evolving. |
| `joker notebook run/export/status/deps/snapshots` | Beta | Useful automation surface; preserve flags where possible and treat EDN notebook file compatibility as user-visible. |
| `joker --lint` | Stable | Important development tool. |
| `joker --compile` / standalone compile paths | Beta | More specialized; short-write and runtime behavior are guarded. |

## Examples as supported surfaces

| Example | Stability | Notes |
|---|---|---|
| `examples/graphics/fractal-flame.joke` | Beta | Demonstrates `joker.jit/compile-wasm` + `joker.imaging/from-rgba32-domain-fn`; also a WASM bridge smoke surface. |
| `examples/games/tetris.joke` | Beta | Demonstrates `joker.term`; interactive by nature, only lint/syntax is easily automated. |
| `examples/wiki/static.joke` | Beta | Demonstrates static build and dynamic serving; smoke-tested through `examples-check`. |
| `examples/notebooks/*.edn` | Beta | Notebook checks validate sample notebooks. |

## Policy

- New public namespaces should be added here before release.
- Stable namespaces should avoid breaking changes in patch releases.
- Beta namespaces may change, but changes should be documented in release notes.
- Experimental namespaces can change when compiler/runtime work requires it, but regressions should become tests.
- Internal/diagnostic namespaces should not be advertised as general application APIs.
