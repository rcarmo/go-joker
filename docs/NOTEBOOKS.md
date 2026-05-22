# Joker notebooks

`joker notebook` is a Mathematica/Observable-inspired local notebook interface for trusted Joker code.

V1 goals:

- local browser notebook server;
- EDN notebook files with inline outputs;
- Markdown and Joker code cells;
- headless execution for agents/CI;
- Markdown export with fenced blocks for text, EDN values, chart specs, Mermaid/DOT diagrams, and graph JSON;
- manual dependency metadata for future reactive execution;
- rich-output envelope for charts, images, SVG, Mermaid, DOT, and graph JSON.

## Commands

```bash
joker notebook [file.edn] [-p 8080] [--open]
joker notebook serve [file.edn] [-p 8080]
joker notebook serve [file.edn] --addr 127.0.0.1:8080
joker notebook new file.edn --title "Example"
joker notebook run file.edn
joker notebook status file.edn
joker notebook deps file.edn
joker notebook snapshots file.edn
joker notebook restore file.edn snapshot.bak.edn
joker notebook export file.edn -o report.md
```

The server binds to `127.0.0.1` when `-p`/`--port` is used. Pass `--open` to launch the local notebook URL in the default browser. Use `--addr` only when you explicitly want another interface; notebooks are trusted local code execution surfaces. The CLI prints a warning when `--addr` does not bind to `127.0.0.1` or `localhost`. Responses set `Cache-Control: no-store` and `X-Content-Type-Options: nosniff`.

Before overwriting an existing notebook, the server writes a recovery snapshot under `.joker-notebook-snapshots/` next to the notebook file.

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
(joker.notebook/chart "{\"data\":[1,2,3]}")
(joker.notebook/svg "<svg>...</svg>")
(joker.notebook/mermaid "graph TD; A-->B")
(joker.notebook/dot "digraph { A -> B }")
(joker.notebook/graph "{\"nodes\":[{\"id\":\"A\"}],\"edges\":[]}")
(joker.notebook/image "image/png" "base64...")
```

```clojure
{:notebook/output :chart
 :renderer :echarts
 :spec "{\"data\":[1,2,3]}"}

{:type :chart
 :renderer :echarts
 :spec "{...}"}

{:type :diagram
 :renderer :mermaid
 :source "graph TD; A-->B"}

{:type :diagram
 :renderer :dot
 :source "digraph { A -> B }"}

{:type :graph
 :renderer :graph-json
 :source "{\"nodes\":[],\"edges\":[]}"}

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
joker notebook export demo.edn -o demo.md
```

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

## Fixtures

Example notebooks live under `tests/notebooks/`:

- `basic.edn` — markdown + simple code value output;
- `rich_outputs.edn` — chart and Mermaid helper outputs;
- `dependencies.edn` — named cells and manual dependency metadata.

They are used by the notebook load/run/export tests and are good starting points for manual UI checks.

## Current implementation status

The current slices provide:

- EDN load/save/roundtrip;
- code-cell returned value capture as `:value` outputs;
- code-cell rich-output map normalization via `:notebook/output`/`:type`;
- headless `notebook run`;
- Markdown export with rich output fallbacks;
- local web UI with automatic OS color-scheme support plus explicit Light/Dark/Auto theme buttons, keyboard shortcuts (`Ctrl/Cmd+S`, `Ctrl/Cmd+Enter`, `Shift+Enter`), unsaved-change tracking and before-unload warning, local save snapshot listing/restoring, Mathematica-like cell chrome (`In[n]`/`Out[n]`, state pills, collapsible outputs), action/error log, editable notebook title, size/status warning, output pruning controls, add/delete/reorder controls, raw EDN import, editable cell metadata (`kind`, `name`, `depends-on`), and lightweight Joker syntax highlighting;
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
- dependency-free browser-side bar-chart rendering from simple chart specs (`{:data [...]}` or an ECharts-like first series);
- Mermaid/DOT diagram source blocks with renderer labels;
- graph JSON circular-layout rendering for `{:nodes [...] :edges [...]}`-style JSON payloads;
- port parsing and CLI plumbing;
- unit tests for schema, execution capture, Markdown export, dependencies, web rendering, HTTP APIs (load/export/save/evaluate/add/delete/reorder/downstream), and CLI parsing.

Further slices should improve browser editing ergonomics, add vendored ECharts/Mermaid/DOT renderers for fully interactive charts and diagrams, and support richer save/update payloads from the browser.
