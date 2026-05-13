# Std native-boundary contracts

This note records the focused std namespace audit/guardrail work that supports the broader refactor plan. The goal is to keep Go-backed std namespaces from leaking raw Go panics, unchecked native integer conversions, ignored close/write errors, or unchecked argument indexing.

## Guardrail

`make std-contract-check` runs focused tests for native-boundary behavior and is part of `make docs-check`.

Current coverage:

- `std/http` — HTTP response/body handling, WebSocket lifecycle, SSE streaming write errors.
- `std/io` — copy-count native-int promotion and close behavior.
- `std/strconv` — parse-int range behavior.
- `std/time` — duration/native-int promotion.
- `std/markdown` — conversion error reporting.
- `std/os` — read-dir metadata size/time promotion and watcher close behavior.
- `std/system` — large system time values.
- `std/runtime` — arity checks, runtime profile/benchmark/mem-stat native-int promotion.
- `std/imaging` — image constructor/info arity and vector shape checks.
- `std/pdf` — document proc arity checks and missing-document guardrails.
- `std/svg` — canvas guardrails, polygon/polyline coordinate shape checks, raw write error handling.
- `std/random` — random range overflow checks and wrapped crypto-random errors.
- `std/bolt` — BoltDB argument guards and sequence native-int promotion.
- `std/url` — malformed query/escape errors surface as runtime errors.
- `std/git` — GitRepo argument guards and config map shape regressions.

## Audit rules of thumb

- Native procs should call `CheckArity` before indexing `args` unless they deliberately implement variadic behavior and guard all indexes.
- Helpers that extract wrapped native objects should guard missing indexes before type assertions.
- Native counts, durations, file sizes, and timestamps should return `Int` only when values fit the native int range; otherwise use `BigInt`.
- Close/write/process errors should be surfaced as runtime errors or reported to stderr for best-effort diagnostic paths.
- Shape-dependent indexed data such as color vectors and coordinate vectors should validate `Indexed`/`Counted` and expected lengths before reading elements.

## Current status

The std boundary work remains intentionally focused. It does not imply complete API parity with Babashka or Clojure library catalogs. New namespaces should be added to `std-contract-check` when audits find meaningful native-boundary contracts, not as broad smoke coverage.
