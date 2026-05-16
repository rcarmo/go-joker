# Std native-boundary contracts

Updated: 2026-05-16

This note records the focused std namespace audit/guardrail work that supports the broader refactor plan. The goal is to keep Go-backed std namespaces from leaking raw Go panics, unchecked native integer conversions, ignored close/write errors, or unchecked argument indexing.

## Guardrail

`make std-contract-check` runs focused tests for native-boundary behavior and is part of `make docs-check`.

Current coverage:

- `std/http` — HTTP response/body handling, status bounds, WebSocket lifecycle, SSE streaming write errors, client option bounds, and IPv4/IPv6 address parsing.
- `std/io` — copy-count native-int promotion and close behavior.
- `std/strconv` — parse-int range behavior.
- `std/time` — duration/native-int promotion and timezone/parse error context.
- `std/markdown` — conversion error reporting.
- `std/os` — read-dir metadata size/time promotion, process output/handle lifecycle, and watcher close behavior.
- `std/system` — large system time values.
- `std/runtime` — arity checks, runtime profile/benchmark/mem-stat native-int promotion.
- `std/imaging` — image constructor/info arity, geometry/dimension/color bounds, finite float adjustment/filter checks, opacity bounds, and vector shape checks.
- `std/pdf` — document proc arity checks, page-size selection, finite numeric/geometry bounds, page/line/color bounds, and missing-document guardrails.
- `std/svg` — canvas/viewbox/render/shape dimension guardrails, finite transform floats, polygon/polyline coordinate shape checks, and raw write error handling.
- `std/random` — random range overflow checks and wrapped crypto-random errors.
- `std/bolt` — BoltDB argument guards and sequence native-int promotion.
- `std/url` — malformed query/escape errors surface as runtime errors.
- `std/git` — GitRepo argument guards and config map shape regressions.
- `std/log` — level parsing and variadic logging arity contracts.
- `std/csv` — delimiter/comment validation and writer flush/error propagation for CSV output.
- `std/json` — lazy stream decode errors surface as runtime errors.
- `std/filepath` — file tree walk errors surface with namespace context.
- `std/crypto` — HMAC helper boundary coverage for supported algorithm output.
- `std/math` — native helper vector/error boundary coverage for `modf` and precision validation.
- `std/string` — native helper boundary coverage for rune-width padding, blank detection, and whitespace splitting.
- `std/uuid` — UUID v4 version/variant generation with deterministic random source coverage.

## Audit rules of thumb

- Native procs should call `CheckArity` before indexing `args` unless they deliberately implement variadic behavior and guard all indexes.
- Helpers that extract wrapped native objects should guard missing indexes before type assertions.
- Native counts, durations, file sizes, and timestamps should validate domain bounds before conversion; return `Int` only when values fit the native int range, otherwise use `BigInt`.
- Close/write/process errors should be surfaced as runtime errors or reported to stderr for best-effort diagnostic paths; started processes that are not waited on should release handles and discard unconsumed output explicitly; lazy sequence decode/walk errors should be wrapped with namespace context before panicking.
- Shape-dependent indexed data such as color vectors and coordinate vectors should validate `Indexed`/`Counted` and expected lengths before reading elements.
- Native networking helpers should avoid ad-hoc `host:port` parsing; use `net.SplitHostPort` with explicit fallback semantics for IPv6 and hostless listen addresses.

## Current status

The std boundary work remains intentionally focused. It does not imply complete API parity with Babashka or Clojure library catalogs. New namespaces should be added to `std-contract-check` when audits find meaningful native-boundary contracts, not as broad smoke coverage. Minimal boundary tests for `crypto`, `math`, `string`, and `uuid` now provide a template for low-risk direct native-helper tests where full namespace smoke coverage would be excessive.
