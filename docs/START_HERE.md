# Start Here

This repository is a maintained, performance-oriented fork of Joker with extra namespaces, notebook tooling, examples, and compatibility checks. If you are new to this tree, use this page as the shortest path from clone to useful local validation.

## 1. Build the CLI

```bash
go build -o joker ./cmd/joker
./joker --version
```

The command-line entrypoint is `cmd/joker`. The version string is defined in `core/runtime/version.go` and cross-checked by the release hygiene guard.

## 2. Run a small confidence check

For day-to-day local work, prefer focused checks over running everything:

```bash
make test-short
make docs-paths-check
make examples-check
```

Before sending changes that affect public docs, examples, release metadata, generated docs, or runtime contracts, run:

```bash
make docs-check
```

`docs-check` regenerates API docs and runs the high-value documentation, example, release hygiene, generated-file, layout, error-handling, runtime-contract, and std native-boundary guards.

## 3. Try the examples

Examples are grouped by purpose:

- `examples/graphics/fractal-flame.joke` — pure Joker graphics example.
- `examples/games/tetris.joke` — terminal UI example using `joker.term`.
- `examples/wiki/static.joke` — static/dynamic wiki site example.
- `examples/notebooks/*.edn` — local Joker notebook files.

Use `examples/README.md` for exact commands.

## 4. Find the right documentation

- `README.md` — project overview, feature highlights, and benchmark summary.
- `docs/API_STABILITY.md` — stability classification for public namespaces and user-facing surfaces.
- `docs/DEVELOPER.md` — internals, generated docs, and development checks.
- `docs/RELEASE_CHECKLIST.md` — patch-release validation and tagging hygiene.
- `docs/NOTEBOOKS.md` — local notebook format, CLI, and browser UI.
- `docs/TRACING.md` — runtime tracing/profiling support.

Generated namespace documentation lives in `docs/*.html` and is refreshed by `make docs-check`.

## 5. Repository conventions

- Keep planning and work tracking outside versioned roadmap documents unless a durable user-facing doc is required.
- Keep examples under their grouped directories; `tests/docs_paths_guard.sh` rejects stale pre-reorganization paths.
- Keep temporary files under `.cache/` or test-owned temp directories, not fixed `/tmp` or workspace-specific paths.
- Public API additions should be classified in `docs/API_STABILITY.md` and covered by focused tests or smoke guards.

## 6. Common commands

```bash
make help                  # list the curated targets
make test-repro            # reproducible test subset
make bb-compat             # Babashka compatibility fixtures
make notebook-check        # notebook parser/runner checks
make release-hygiene-check # version, README, release note, checklist consistency
make pretag-check          # local pre-tag release gate before pushing a version tag
```

For parser/codec boundary changes, run the relevant bounded fuzz smoke target for a short interval:

```bash
go test ./core/reader -run '^$' -fuzz=FuzzScanStringLiteral -fuzztime=10s
go test ./std/edn -run '^$' -fuzz=FuzzEDNDecodeAll -fuzztime=10s
go test ./std/transit -run '^$' -fuzz=FuzzTransitDecodeValue -fuzztime=10s
go test ./std/http -run '^$' -fuzz=FuzzReqToMapRemoteAddr -fuzztime=10s
```

If in doubt, start with `make help`, then choose the narrowest target that covers your change.
