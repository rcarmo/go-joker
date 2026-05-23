# go-joker v42.8.4 Release Notes

`v42.8.4` is a patch release that stabilizes the new EDN notebook subsystem, adds browser-level smoke coverage, vendors local chart/diagram renderers, and polishes the notebook UI/security/CI workflow.

## Version

- Bumped runtime version from `v42.8.3` to `v42.8.4`.
- `joker --version` now reports `v42.8.4`.

## Notebook UI and browser workflow

The `joker notebook` browser UI is now more compact and output-focused:

- collapsed source/metadata by default;
- compact cell headers that show user-facing names rather than raw IDs/dependency metadata;
- SVG icon buttons with tooltips/ARIA labels;
- top-right global controls for dependency checks, snapshots, and theme selection;
- collapsed raw EDN/Markdown export pane;
- tighter spacing and reduced vertical clutter;
- trusted HTML/SVG outputs render correctly instead of being escaped.

Browser-level tooling was added:

```bash
make notebook-browser-smoke
make notebook-screenshot
```

The Playwright smoke test creates a temporary rich demo notebook, runs it, starts a token-protected localhost server, opens Chromium, verifies rendered cells/table output, filters/sorts a table, exports Markdown, displays the dependency graph, saves a title change, and checks the status API.

## Notebook security and serving

- Mutating HTTP requests reject non-matching `Origin` headers.
- `--token` protects mutating notebook APIs and is automatically attached by the served browser UI.
- `--readonly` serves a notebook without allowing save/evaluate/delete/restore mutations.
- Read-only mode disables mutation controls and guards keyboard shortcuts/actions.
- Non-local binds warn and recommend `--token`.

## Notebook headless and CI workflow

Headless notebook execution gained CI/agent-friendly flags:

```bash
joker notebook run file.edn --no-save
joker notebook run file.edn --no-save --summary
joker notebook run file.edn --no-save --summary --fail-on-error
```

The JSON summary now includes:

- top-level `success` boolean;
- aggregate `ok`, `errors`, and `idle` counts;
- per-cell state/output counts;
- per-cell error text for failed cells.

`make notebook-check` now validates checked-in notebook fixtures, validates the rich demo, and executes the rich demo headlessly with `--no-save --summary --fail-on-error`.

## Rich outputs and renderers

Notebook rich output support was expanded and cleaned up:

- added explicit `joker.notebook/text` helper;
- table outputs now support filtering, sortable headers, 100-row initial cap, row-count/truncation summary, and a `Show all` toggle;
- vendored local ECharts asset for chart rendering, with the previous dependency-free SVG bar chart fallback retained;
- vendored local Mermaid asset for diagrams, with a dependency-free simple-flow fallback retained;
- Mermaid fallback uses rounded orthogonal arrows for simple flows;
- DOT/Graphviz output support was removed from the notebook scope.

Vendored assets are embedded and served locally by the notebook server:

```text
/assets/echarts.min.js
/assets/mermaid.min.js
```

No CDN dependencies are used.

## Documentation and examples

- Updated `docs/NOTEBOOKS.md` with the finalized command set, security behavior, smoke test/screenshot tooling, vendored renderers, and DOT removal.
- Updated the rich demo notebook and embedded demo asset to cover current helper/output paths.
- Added browser screenshot and smoke scripts under `scripts/`.

## CI/release workflow

- CI and release workflows now install Bun/Playwright dependencies and run `make notebook-browser-smoke` in addition to `make notebook-check`.
- Playwright browsers and Bun dependencies are kept out of the repository via `.gitignore`.

## Verified checks

Validated locally with targeted commands:

```bash
go test ./internal/notebook ./cmd/joker -count=1
make notebook-check
make notebook-browser-smoke
make notebook-screenshot
git diff --check
```

Observed results:

- Notebook model/CLI/browser checks pass.
- Rich demo validates and runs headlessly without errors.
- Browser smoke test passes in Chromium.
- Screenshot generation succeeds.
