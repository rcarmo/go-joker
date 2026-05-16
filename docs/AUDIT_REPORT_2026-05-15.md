# Audit report — 2026-05-15

## Scope

Follow-up audit pass during the `core` breakup work, focused on:

- root `core/` decomposition guardrails;
- collection/reader helper extraction correctness;
- static-analysis and vulnerability posture;
- resource lifecycle and unchecked write/close paths;
- WASM memory bounds/write behavior;
- CI/docs/benchmark path drift.

## Validation matrix

All checks were run with workspace temp directories because `/tmp` is noexec in this environment:

```sh
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/go-build-tmp make docs-check
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/go-build-tmp make bb-compat
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/go-build-tmp go test ./...
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/go-build-tmp go vet ./...
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/go-build-tmp make staticcheck-sa
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/go-build-tmp make vuln
```

Additional targeted checks included race tests for leaf packages:

```sh
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/go-build-tmp \
  go test -race ./core/reader ./core/collections ./core/string ./core/cursor
```

Current status: all above checks pass, and `govulncheck` reports no vulnerabilities.

## High-value issues found and fixed

1. **Reader lexical helper edge cases**
   - `core/reader.IsValidUnicodeRune` now rejects negative runes.
   - `AnalyzeNumberToken` now rejects empty tokens and bare `N`/`M` helper inputs instead of producing misleading tokens or risking slice edge cases.

2. **HTTP response write errors**
   - `std/http` response body writes now check `io.WriteString` errors.
   - The server panic fallback logs failure to write the internal-error response.
   - Regression coverage verifies response write failures surface as core errors.

3. **WASM array allocation and memory writes**
   - WASM array allocation rejects negative and overflowing sizes.
   - Allocation checks memory-zeroing writes before advancing `nextOffset`.
   - The memory-backed vector nth backend now checks vector-copy writes and falls back cleanly on failure.

4. **Home directory fallback and close handling**
   - `core/osutil.HomeDir` now prefers non-empty `$HOME`, falls back to non-empty `$USERPROFILE`, then `os.UserHomeDir()`.
   - Tests now check close errors in affected reader/osutil helpers.

5. **Static-analysis/code-smell cleanup**
   - Removed redundant `for k, _ := range ...` patterns in CLI/codegen paths.
   - Removed duplicate Go-name translation table entries.
   - Staticcheck SA and vet remain clean.

6. **Native/std logic hardening**
   - `std/string` padding and index helpers now handle empty pads and out-of-range `from` indexes without Go panics.
   - Version parsing now handles malformed numeric components explicitly.
   - `Transit` cmap decoding rejects odd payloads instead of silently dropping dangling entries.
   - External source URL cache-path parsing now rejects malformed URLs instead of indexing a missing split component.

7. **Collection/concurrency edge cases**
   - `sorted-map` and `sorted-map-by` now replace duplicate keys deterministically, including comparator-equal keys.
   - Transient vector assoc now reports bad key types as Joker argument errors.
   - Transient maps and `MapSet` have nil/zero-value-safe behavior where practical.
   - `alts!` now rejects odd option lists instead of ignoring dangling option keys.
   - `(timeout ms)` now rejects negative values and millisecond values that would overflow `time.Duration`.

8. **Std native argument/domain hardening**
   - `std/imaging` constructors, geometry helpers, and float adjustment/filter/compositing parameters now reject invalid dimensions, non-finite values, bad gamma/sigma domains, invalid opacity, and out-of-range color components.
   - `std/pdf` now validates custom page dimensions, page-size selection, font sizes, finite coordinates, line widths, color components, and related geometry/color bounds before passing values to native libraries.
   - `std/svg` now validates canvas, viewbox, render, basic shape dimensions/radii, and finite transform floats, and shares the render image conversion path.
   - `std/http` now validates response/SSE status codes, rejects negative/overflowing client duration options, and parses IPv4/IPv6 listen and remote addresses with `net.SplitHostPort` rather than ad-hoc colon splitting.
   - `std/pods` now validates `pods/invoke` `:timeout-ms` options instead of silently ignoring malformed values, and rejects overflow-prone millisecond durations.
   - `std/csv` now validates delimiter/comment options before configuring Go's CSV reader/writer.
   - `std/os` process start now discards unused output streams and releases started process handles when no wait is expected.
   - Additional boundary fixes reject malformed reader arg literal indexes, protocol extension method-pair arity mistakes, odd Transit cmap entries, invalid external source URLs, and short standalone compile writes.

## Refactor documentation state

- `core/collections` is no longer only a marker package. It owns root-independent slice, pair-array, bitmap/hash-index, and opaque trie node/path mechanics.
- `core/reader` owns root-independent character classes, whitespace/comment/line scanning, identifier token scanning/validation/issue enumeration, unicode/string escape parsing, number-token classification, dispatch/form/prefix helpers, line rune reader, and raw IO mechanics.
- Concrete collection Object/protocol behavior and concrete reader Object/tagged-literal/parser behavior remain root-bound until dependencies are explicit and acyclic.
- Go benchmarks live under `benchmarks/core`; root `core` should not regain `Benchmark*` functions.

## Residual items

- Continue audit cadence after each structural batch: `staticcheck-sa`, `vuln`, `go vet ./...`, full tests, and targeted race tests for touched leaf packages.
- Add parser/eval fuzz targets when the reader/parser extraction surface becomes less root-coupled.
- Keep generated runtime-mutating files root-bound until equivalence tests and generator seams make real package movement safe.
