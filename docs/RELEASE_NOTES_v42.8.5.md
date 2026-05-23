# go-joker v42.8.5 Release Notes

`v42.8.5` is a patch release focused on the notebook editor experience, Mermaid flow rendering, and CI/release workflow hardening after `v42.8.4`.

## Version

- Bumped runtime version from `v42.8.4` to `v42.8.5`.
- `joker --version` now reports `v42.8.5`.

## Notebook editor

The notebook browser now uses vendored CodeMirror 5 assets for cell source editing:

- Clojure/Joker highlighting for code cells;
- Markdown highlighting for Markdown cells;
- automatic close-bracket editing;
- `Ctrl-Space` / `Cmd-Space` autocomplete;
- first-pass Joker symbol autocomplete list, including common core forms/functions and `joker.notebook/*` helpers;
- mode switching when the cell kind changes;
- proper source synchronization before save/evaluate/export;
- read-only mode support for CodeMirror instances.

Vendored CodeMirror files are embedded and served locally under `/assets/codemirror/...`; no CDN dependencies are used.

## Notebook browser assets

The notebook asset embed now covers nested asset directories, so CodeMirror, ECharts, and Mermaid assets are all served through the same local `/assets/` path.

## Mermaid flow rendering

Simple Mermaid flowcharts now intentionally bypass Mermaid's default curved-link renderer and use the local picker-style renderer instead:

- rounded node boxes (`rx`/`ry` set on SVG rects);
- rounded orthogonal connector paths using vertical/horizontal segments plus quadratic corner curves;
- rounded line caps/joins;
- explicit arrow markers.

Vendored Mermaid remains available for diagram shapes the simple local parser does not understand.

## CI/release hardening

- Notebook CLI tests now use per-test temporary directories instead of hardcoded `/workspace/tmp`, making them portable to GitHub Actions runners.
- The release workflow now sets `DOCS_JOKER_BIN` to a writable workspace cache path.
- CI builds the smoke-test Joker binary under `.cache/tmp/` instead of creating a root-level `joker` artifact that violates the layout guard.

## Visual review

A fresh full-page notebook screenshot with CodeMirror enabled was captured using:

```bash
make notebook-screenshot
```

## Verified checks

Validated locally with targeted commands:

```bash
go test ./internal/notebook -run TestNotebookPageRenders -count=1
make notebook-browser-smoke
make notebook-check
git diff --check
```

Observed results:

- Notebook page-render smoke checks include CodeMirror assets/autocomplete wiring.
- Browser smoke test passes in Chromium.
- Notebook fixture/demo checks pass.
