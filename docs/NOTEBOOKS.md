# Joker notebooks

`joker notebook` is a Mathematica/Observable-inspired local notebook interface for trusted Joker code.

V1 goals:

- local browser notebook server;
- EDN notebook files with inline outputs;
- Markdown and Joker code cells;
- headless execution for agents/CI;
- Markdown export;
- manual dependency metadata for future reactive execution;
- rich-output envelope for charts, images, SVG, Mermaid, DOT, and graph JSON.

## Commands

```bash
joker notebook [file.edn] [-p 8080] [--open]
joker notebook serve [file.edn] [-p 8080]
joker notebook serve [file.edn] --addr 127.0.0.1:8080
joker notebook run file.edn
joker notebook export file.edn -o report.md
```

The server binds to `127.0.0.1` when `-p`/`--port` is used. Use `--addr` only when you explicitly want another interface; notebooks are trusted local code execution surfaces.

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

Code can return or future renderers can store normalized outputs such as:

```clojure
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

## Dependency metadata

Reactive execution starts with manual dependencies:

```clojure
{:id "cell-3"
 :kind :code
 :name "chart"
 :depends-on ["data"]
 :source "(make-chart data)"}
```

The current implementation includes downstream dependency calculation and keeps the schema ready for later runtime dependency tracking.

## Current implementation status

The first slice provides:

- EDN load/save/roundtrip;
- headless `notebook run`;
- Markdown export;
- minimal local web UI and API;
- port parsing and CLI plumbing;
- unit tests for schema, execution capture, Markdown export, dependencies, and CLI parsing.

Further slices should improve browser editing, syntax highlighting, lazy rich renderers, and richer cell-level evaluate/save APIs.
