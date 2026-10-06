# Start Here

This repository is a maintained, performance-oriented fork of Joker with extra namespaces, notebook tooling, examples and compatibility checks. Start here for a local build, a small validation pass and the current WASM workflow.

## 1. Build the CLI

```bash
source scripts/project-env.sh
make cli
"$CLI_BIN" --version
```

`cmd/joker` is the CLI entrypoint. On this host `CLI_BIN` defaults to `/workspace/tmp/go-joker/build/joker`.

## 2. Run a focused local check

For ordinary development, run the narrowest useful target:

```bash
make test-short
make docs-paths-check
make examples-check
make ai-check               # lint and run the offline joker.ai fixture suite
```

Before sending changes that affect public docs, examples, release metadata, generated docs or runtime contracts, run:

```bash
make docs-check
```

`docs-check` regenerates API docs and runs the high-value documentation, example, release hygiene, generated-file, layout, error-handling, runtime-contract and std native-boundary guards.

## 3. Try the WASM engines and standalone workflow

```bash
source scripts/project-env.sh
make cli
"$CLI_BIN" --wasm-engine=interpreter examples/wasm/native-sum.joke
"$CLI_BIN" --wasm-engine=compiler examples/wasm/native-sum.joke
"$CLI_BIN" compile --native --run examples/wasm/native-sum.joke \
  -o "$PROJECT_TMP_ROOT/build/native-sum"
"$PROJECT_TMP_ROOT/build/native-sum" --wasm-engine=interpreter
```

`--wasm-engine` accepts `auto`, `interpreter` or `compiler`; `native` aliases `compiler`. `compile` produces a platform executable that contains the Joker runtime plus bundled source. `--native` stores compiler selection for eligible `joker.jit/compile-wasm` functions. Runtime flags or `JOKER_WASM_ENGINE` can still override that saved default. The resulting program remains source-bundled; eligible functions compile to WASM at runtime.

Use `docs/WASM_EXECUTION.md` for the full engine, override and standalone notes.

## 4. Try the examples

Examples are grouped by purpose:

- `examples/wasm/native-sum.joke` — small `joker.jit/compile-wasm` example that prints the actual WASM engine.
- `examples/graphics/fractal-flame.joke` — pure Joker graphics example with a WASM-compiled numeric kernel.
- `examples/games/tetris.joke` — terminal UI example using `joker.term`.
- `examples/wiki/static.joke` — static/dynamic wiki site example.
- `examples/notebooks/*.edn` — local Joker notebook files.
- `examples/ai/joker/ai.joke` — experimental provider-neutral AI client with offline fixtures.

Use `examples/README.md` for exact commands.

## 5. Find the right documentation

- `README.md` — project overview, feature highlights and benchmark summary.
- `docs/WASM_EXECUTION.md` — WASM engine selection, `joker.jit/compile-wasm` limits and standalone executable workflow.
- `docs/API_STABILITY.md` — stability classification for public namespaces and user-facing surfaces.
- `docs/DEVELOPER.md` — internals, generated docs and development checks.
- `docs/RELEASE_CHECKLIST.md` — patch-release validation and tagging hygiene.
- `docs/NOTEBOOKS.md` — local notebook format, CLI and browser UI.
- `docs/TRACING.md` — targeted tracing and pre-release profiling workflow.
- `examples/ai/README.md` — provider-neutral AI client contract, credentials, security boundaries and tests.

Generated namespace documentation lives in `docs/*.html` and is refreshed by `make docs-check`.

## 6. Repository conventions

- Source `scripts/project-env.sh` before direct commands that build binaries or create scratch output.
- Keep temporary files under the project-owned `$PROJECT_TMP_ROOT` hierarchy, not ad hoc `/tmp` paths or the repository root.
- Public API additions should be classified in `docs/API_STABILITY.md` and covered by focused tests or smoke guards.
- Runtime maintainability work should move cohesive same-package clusters in small slices, keep behaviour unchanged and pair each move with focused contract validation before broader checks.
- Pre-release profiling must review CPU and heap/allocation behaviour. Ordinary local tests do not need profiles unless you are investigating a problem. Delete raw profiles, matching binaries and disposable logs after review; keep short conclusions only.

## 7. Common commands

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
