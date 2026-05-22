# go-joker v42.8.3 Release Notes

`v42.8.3` is a patch release focused on runtime documentation lookup, CI parity fixes, and refreshed benchmark documentation.

## Version

- Bumped runtime version from `v42.8.2` to `v42.8.3`.
- `joker --version` now reports `v42.8.3`.

## Runtime documentation lookup

Added a pydoc-style documentation frontend:

```bash
joker doc
joker doc joker.string
joker doc joker.core/first
joker doc search websocket
joker doc --format json joker.core/first
joker doc serve --addr 127.0.0.1:8080
```

Highlights:

- Markdown output is the default for terminal and agent-friendly use.
- JSON output is available for tools.
- `joker doc serve` starts a local HTTP browser/search UI.
- The implementation indexes live runtime namespace/var metadata after normal runtime initialization instead of embedding the static generated HTML/PNG documentation tree.

See [`docs/RUNTIME_DOCS.md`](RUNTIME_DOCS.md).

## CI/parity fixes

- Restored string seqability so Clojure parity `reader/string-unicode` passes again.
- Fixed syntax-quote treatment of special forms so jank portability fixtures no longer rewrite bare `do` to a namespace-qualified symbol.
- Moved native var metadata completion late enough in initialization for docs generation to avoid `<internal>` warning output.
- Added `docs-command-check` to validate `joker doc` Markdown/JSON smoke behavior.

## Benchmark documentation

This release includes the refreshed benchmark docs/charts committed before the version bump:

- updated benchmark README table;
- regenerated benchmark SVGs;
- refreshed Python/Bun/Goja comparative values from the latest validated comparative run.

## Verified checks

Validated locally with CI-equivalent commands:

```bash
go build -o joker ./cmd/joker
./joker --version

go test ./core ./std/... ./cmd/joker -timeout 10m -count=1
make docs-check
go run tests/clojure_parity.go -joker ./joker -out docs/DIVERGENCE_MATRIX.md
JOKER_BIN=./joker tests/run_jank_subset.sh
go vet ./core ./std/... ./cmd/joker
```

Observed results:

- Runtime reports `v42.8.3`.
- Core/std/cmd tests pass.
- `make docs-check` passes, including docs command smoke checks.
- Clojure parity: `271/271 pass`, `0 fail`, `0 error`.
- Imported jank smoke subset: `7 pass`, `0 fail`.
- `go vet` passes.
