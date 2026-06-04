# Joker notebooks

`joker notebook` is a Mathematica/Observable-inspired local notebook interface for trusted Joker code.

V1 goals:

- local browser notebook server;
- EDN notebook files with inline outputs;
- Markdown and Joker code cells;
- headless execution for agents/CI;
- Markdown export with fenced blocks for text, EDN values, chart specs, Mermaid diagrams, and graph JSON;
- manual dependency metadata for future reactive execution;
- rich-output envelope for charts, tables, images, trusted HTML, SVG, Mermaid, and graph JSON.

## Commands

```bash
joker notebook [file.edn] [-p 8080] [--open] [--token secret] [--readonly]
joker notebook serve [file.edn] [-p 8080] [--token secret] [--readonly]
joker notebook serve [file.edn] --addr 127.0.0.1:8080
joker notebook new file.edn --title "Example"
joker notebook new file.edn --title "Example" --serve --open -p 8080
joker notebook new file.edn --title "Example" --serve --readonly --token secret
joker notebook demo rich-demo.edn
joker notebook run file.edn
joker notebook run file.edn --no-save
joker notebook run file.edn --no-save --summary
joker notebook run file.edn --no-save --summary --fail-on-error
joker notebook format file.edn [...]
joker notebook validate file.edn
joker notebook status file.edn
joker notebook deps file.edn
joker notebook snapshots file.edn
joker notebook restore file.edn snapshot.bak.edn
joker notebook export file.edn -o report.md
```

The server binds to `127.0.0.1` when `-p`/`--port` is used. Pass `--open` to launch the local notebook URL in the default browser. Use `--addr` only when you explicitly want another interface; notebooks are trusted local code execution surfaces. The CLI prints a warning when `--addr` does not bind to `127.0.0.1` or `localhost`, and recommends `--token` for non-local binds. When `--token` is set, mutating requests must include `X-Joker-Notebook-Token: <token>` or `?token=<token>`; the served browser UI automatically attaches the token to mutating same-page API calls. Use `--readonly` to serve the notebook and export/status APIs without allowing save/evaluate/delete/restore mutations; the browser UI disables mutation controls and guards keyboard shortcuts/actions. Responses set `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`; mutating HTTP requests with a non-matching `Origin` header are rejected.

Before overwriting an existing notebook, the server writes a recovery snapshot under `.joker-notebook-snapshots/` next to the notebook file. Snapshot directories are ignored by git.

Use `joker notebook format file.edn [...]` to rewrite notebooks with Joker's stable pretty-printed EDN layout without evaluating cells or changing outputs.

The browser UI avoids a full page reload for save and Markdown export: those actions update the raw/status panes in place. Evaluation and structural mutations still reload after success so rendered outputs and ordering stay consistent. Cell source editing uses vendored CodeMirror with Clojure/Joker and Markdown highlighting, close-bracket editing, and Joker symbol autocomplete. Rendered table outputs have a client-side filter box and sortable column headers. They cap the initial display to 100 rows with a visible row-count/truncation summary plus a `Show all` toggle.

## File format

Notebooks are regular EDN maps. The extension can be plain `.edn`; the format marker identifies notebook files.

```clojure
{:format :joker/notebook
 :version 1
 :title "Example"
 :created-at "2026-05-22T20:00:00Z"
 :updated-at "2026-05-22T20:10:00Z"
 :cells [{:id "cell-1"
          :kind :markdown
          :source "# Demo"
          :outputs []}

         {:id "cell-2"
          :kind :code
          :name "data"
          :depends-on []
          :source "(+ 1 2)"
          :execution-count 1
          :state :ok
          :outputs [{:type :stdout
                     :text "3\n"}]}]}
```

## Rich output envelope

Code cells can use helper functions in `joker.notebook` or return a map with `:notebook/output`/`:type` to create a rich output directly. Renderers can also store normalized outputs in the same shape:

```clojure
(joker.notebook/text "plain text output")
(joker.notebook/chart {:data [1 2 3]})
(joker.notebook/chart "{\"data\":[1,2,3]}")
(joker.notebook/svg "<svg>...</svg>")
(joker.notebook/html "<b>trusted local HTML</b>")
(joker.notebook/mermaid "graph TD; A-->B")
(joker.notebook/graph {:nodes [{:id "A"}] :edges []})
(joker.notebook/graph "{\"nodes\":[{\"id\":\"A\"}],\"edges\":[]}")
(joker.notebook/table [{:name "Ada" :score 42}])
(joker.notebook/image "image/png" "base64...")
```

Bitmap outputs can be generated directly inside Joker with `joker.imaging`.
Use `joker.imaging/new` to allocate an image, `joker.imaging/set-pixel!` and
`joker.imaging/pixel` for pixel-level access, `joker.imaging/resize` for display
scaling, and `joker.imaging/from-rgba32` to build an image from row-major packed
RGBA integer pixels (`0xRRGGBBAA`). Use `joker.imaging/from-rgba32-fn` when a
cell can compute pixels procedurally; it calls a supplied `(fn [x y] ...)` and
fills the image directly, which avoids building a large intermediate pixel
vector. Use `joker.imaging/from-rgba32-domain-fn` when a numeric kernel works
in a continuous coordinate space; it passes `xmin + x*dx` and `ymin + y*dy` to
the pixel function and is a good fit for `joker.jit/compile-wasm` kernels.
Notebook cells may return a `joker.imaging` image object directly; the notebook
renderer encodes it at the output boundary. Use `joker.imaging/encode` plus
`joker.base64/encode-string` only when a cell needs to construct an explicit
`joker.notebook/image` envelope itself.

Hot numeric kernels can be moved to the in-process WASM path with
`joker.jit/compile-wasm` when the function is pure numeric and WASM-eligible.
The complex demo uses this for Mandelbrot color calculation, then rasterizes
packed RGBA32 pixels through `joker.imaging/from-rgba32-domain-fn`. The same
pattern is used by `examples/fractal-flame.joke`, which keeps the raster
algorithm in Joker/Clojure code while handing each pixel to a WASM-compiled
numeric kernel. Current WASM support handles binary arithmetic, loop/recur,
comparisons, and value-producing `if` expressions such as
`(let [a (if (< x 0.0) (- 0.0 x) x)] ...)`.

```clojure
{:type :text
 :text "plain text output"}

{:notebook/output :chart
 :renderer :echarts
 :spec "{\"data\":[1,2,3]}"}

{:type :chart
 :renderer :echarts
 :spec "{...}"}

{:type :diagram
 :renderer :mermaid
 :source "graph TD; A-->B"}

{:type :graph
 :renderer :graph-json
 :source "{\"nodes\":[],\"edges\":[]}"}

{:type :table
 :source "[{\"name\":\"Ada\",\"score\":42}]"}

{:type :html
 :renderer :trusted
 :source "<b>trusted local HTML</b>"}

{:type :svg
 :source "<svg>...</svg>"}

{:type :image
 :mime "image/png"
 :encoding :base64
 :data "..."}
```

Outputs are inline by default so notebooks are self-contained.

## Quick rich-output example

Create `demo.edn`:

```clojure
{:format :joker/notebook
 :version 1
 :title "Notebook rich output demo"
 :cells [{:id "cell-1"
          :kind :markdown
          :source "# Rich output demo"
          :outputs []}

         {:id "cell-2"
          :kind :code
          :name "chart"
          :depends-on []
          :source "(joker.notebook/chart \"{\\\"xAxis\\\":{\\\"data\\\":[\\\"A\\\",\\\"B\\\",\\\"C\\\"]},\\\"series\\\":[{\\\"data\\\":[4,7,3]}]}\")"
          :execution-count 0
          :state :idle
          :outputs []}

         {:id "cell-3"
          :kind :code
          :name "flow"
          :depends-on ["chart"]
          :source "(joker.notebook/mermaid \"graph TD; Load-->Transform; Transform-->Render\")"
          :execution-count 0
          :state :idle
          :outputs []}]}
```

Run headlessly and export:

```bash
joker notebook run demo.edn
joker notebook run demo.edn --no-save
joker notebook export demo.edn -o demo.md
```

Use `--no-save` for CI/smoke checks that should execute cells without rewriting the notebook EDN. Add `--summary` for a compact JSON report of cell states/output counts, including a top-level `success` boolean, aggregate `ok`, `errors`, and `idle` counts, plus per-cell `error` text for failed cells. Add `--fail-on-error` to exit non-zero when any evaluated cell ends in `:error`.

Or browse it locally:

```bash
joker notebook demo.edn -p 8080
```

## Dependency metadata

Reactive execution starts with manual dependencies:

```clojure
{:id "cell-3"
 :kind :code
 :name "chart"
 :depends-on ["data"]
 :source "(make-chart data)"}
```

The current implementation includes downstream dependency calculation, dependency cycle detection, a dependency graph endpoint/UI, and `evaluate-downstream` support from explicit `:depends-on` metadata. Downstream evaluation rejects cyclic metadata. It keeps the schema ready for later runtime dependency tracking.

## Demo and fixtures

A user-facing rich-output demo lives at:

```bash
examples/notebooks/rich-demo.edn
examples/notebooks/complex-demo.edn
```

Try it with:

```bash
joker notebook demo /tmp/rich-demo.edn
joker notebook run /tmp/rich-demo.edn
joker notebook /tmp/rich-demo.edn -p 8080 --open
```

`examples/notebooks/complex-demo.edn` is a broader smoke/demo notebook with
charts, tables, Mermaid, graph JSON, trusted SVG/HTML, a generated Mandelbrot
image using a WASM escape kernel plus packed RGBA32 bitmap data, and a projected
SVG 3D scene with depth sorting and back-face culling.

Test fixtures live under `tests/notebooks/`:

- `basic.edn` — markdown + simple code value output;
- `rich_outputs.edn` — chart and Mermaid helper outputs;
- `dependencies.edn` — named cells and manual dependency metadata.

They are used by the notebook load/run/export tests, validated by `make notebook-check`, and are good starting points for manual UI checks. `make notebook-check` also runs the rich demo headlessly with `--no-save --summary --fail-on-error` to catch execution regressions without rewriting the example.

Browser-level smoke coverage lives in `scripts/notebook_smoke.ts` and runs through Playwright:

```bash
make notebook-browser-smoke
```

The smoke script creates a temporary rich demo notebook, runs it, starts a localhost notebook server with a token, opens Chromium, verifies rendered cells/table output, filters/sorts a table, exports Markdown, displays the dependency graph, saves a title change, and checks the status API.

Use `make notebook-screenshot` to capture a full-page rich demo screenshot under `.cache/screenshots/` for visual review.

## Current implementation status

The current slices provide:

- EDN load/save/roundtrip;
- code-cell returned value capture as `:value` outputs;
- code-cell rich-output map normalization via `:notebook/output`/`:type`;
- headless `notebook run`;
- Markdown export with rich output fallbacks;
- local web UI with automatic OS color-scheme support plus explicit Light/Dark/Auto theme buttons, keyboard shortcuts (`Ctrl/Cmd+S`, `Ctrl/Cmd+Enter`, `Shift+Enter`), unsaved-change tracking and before-unload warning, local save snapshot listing/restoring, Mathematica-like cell chrome (`In[n]`/`Out[n]`, state pills, collapsible outputs), action/error log, editable notebook title, size/status warning, output pruning controls, add/delete/reorder controls, raw EDN import, editable cell metadata (`kind`, `name`, `depends-on`), Markdown previews for markdown cells, and vendored CodeMirror editing with Clojure/Joker + Markdown modes, close brackets, and Joker symbol autocomplete;
- `GET /api/notebook`;
- `GET /api/status` for cell/output counts, encoded EDN size, and the >10 MB inline-output warning;
- `GET /api/snapshots` to list local recovery snapshots;
- `POST /api/restore-snapshot?path=<snapshot>` to restore a listed recovery snapshot;
- `POST /api/clear-outputs` or `POST /api/clear-outputs?id=<cell-id>` to prune inline outputs;
- `GET /api/export/markdown` for browser/API Markdown export;
- `POST /api/save` for full EDN replacement/import from the raw pane;
- `POST /api/save-sources` for browser source/metadata edits as JSON;
- `POST /api/cell?kind=code|markdown` to append cells;
- `DELETE /api/cell?id=<cell-id>` to delete cells;
- `POST /api/reorder` with `{"ids":[...]}` to reorder cells;
- `POST /api/evaluate-cell?id=<cell-id>` with optional plain-text source body;
- `GET /api/dependencies` to report manual dependency cycles and graph JSON;
- `POST /api/evaluate-downstream?name=<cell-name>` to evaluate cells that manually depend on a named cell;
- `POST /api/evaluate-all` with optional JSON source update body;
- inline SVG and base64 image rendering;
- vendored ECharts rendering for chart specs, with a dependency-free SVG bar-chart fallback for simple specs (`{:data [...]}` or an ECharts-like first series);
- dependency-free browser-side table rendering from JSON row arrays;
- Mermaid rendering for diagrams; simple flowcharts intentionally use the local picker-style renderer with rounded boxes and rounded orthogonal SVG arrow paths;
- graph JSON circular-layout rendering for `{:nodes [...] :edges [...]}`-style JSON payloads;
- port parsing and CLI plumbing;
- unit tests for schema, execution capture, Markdown export, dependencies, web rendering, HTTP APIs (load/export/save/evaluate/add/delete/reorder/downstream), and CLI parsing.

Further slices should support richer no-reload evaluate/update payloads from the browser and deeper runtime dependency tracking. The vendored CodeMirror/ECharts/Mermaid assets are served locally by the notebook server; simple Mermaid flowcharts intentionally bypass Mermaid's default curved links and use the picker-style renderer with rounded boxes plus rounded orthogonal arrows.
