package notebook

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snap.edn")
	if err := os.WriteFile(path, []byte("{:format :joker/notebook}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeSnapshot(path); err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(dir, ".joker-notebook-snapshots", "snap.edn.*.bak.edn"))
	if err != nil || len(matches) != 1 {
		t.Fatalf("snapshots = %v err=%v", matches, err)
	}
	snaps, err := ListSnapshots(path)
	if err != nil || len(snaps) != 1 || snaps[0].Size == 0 {
		t.Fatalf("ListSnapshots = %#v err=%v", snaps, err)
	}
	if err := os.WriteFile(path, []byte("{:format :joker/notebook :version 1 :title \"new\" :cells []}"), 0644); err != nil {
		t.Fatal(err)
	}
	restored, err := RestoreSnapshot(path, snaps[0].Path)
	if err != nil || restored.Title == "new" {
		t.Fatalf("RestoreSnapshot = %#v err=%v", restored, err)
	}
}

func TestBrowserCommand(t *testing.T) {
	cmd, args := browserCommand("http://127.0.0.1:8080/")
	if cmd == "" || len(args) == 0 {
		t.Fatalf("browserCommand = %q %#v", cmd, args)
	}
}

func TestBuildStatus(t *testing.T) {
	nb := New("Status")
	nb.Cells = []Cell{{ID: "cell-1", Outputs: []Output{{Type: "value", Text: "1"}}}}
	status := BuildStatus(nb)
	if status.Title != "Status" || status.CellCount != 1 || status.OutputCount != 1 || status.Bytes == 0 {
		t.Fatalf("status = %#v", status)
	}
}

func TestFixtureLoad(t *testing.T) {
	for _, path := range []string{"../../tests/notebooks/basic.edn", "../../tests/notebooks/rich_outputs.edn", "../../tests/notebooks/dependencies.edn", "../../examples/notebooks/rich-demo.edn"} {
		nb, err := Load(path)
		if err != nil {
			t.Fatalf("Load(%s): %v", path, err)
		}
		if nb.Format != "joker/notebook" || len(nb.Cells) == 0 {
			t.Fatalf("Load(%s) = %#v", path, nb)
		}
	}
}

func TestEncodeLoadRoundTrip(t *testing.T) {
	nb := New("Demo")
	nb.Cells = []Cell{{ID: "cell-1", Kind: "markdown", Source: "# Hello"}, {ID: "cell-2", Kind: "code", Name: "x", Source: "(+ 1 2)", Outputs: []Output{{Type: "stdout", Text: "3\n"}}}}
	path := t.TempDir() + "/demo.edn"
	if err := Save(path, nb); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != "joker/notebook" || got.Title != "Demo" || len(got.Cells) != 2 || got.Cells[1].Name != "x" {
		t.Fatalf("roundtrip = %#v", got)
	}
}

func TestRunCapturesReturnedValue(t *testing.T) {
	nb := New("Run")
	nb.Cells = []Cell{{ID: "cell-1", Kind: "code", Source: "(+ 1 2)"}}
	Run(&nb)
	if nb.Cells[0].State != "ok" {
		t.Fatalf("state = %s", nb.Cells[0].State)
	}
	joined := ""
	for _, o := range nb.Cells[0].Outputs {
		joined += o.Text
	}
	if !strings.Contains(joined, "3") {
		t.Fatalf("outputs = %#v", nb.Cells[0].Outputs)
	}
}

func TestEvaluateCellNotebookTextHelper(t *testing.T) {
	cell := Cell{ID: "helper-text", Kind: "code", Source: `(joker.notebook/text "hello")`}
	EvaluateCell(&cell)
	if cell.State != "ok" || len(cell.Outputs) != 1 || cell.Outputs[0].Type != "text" || cell.Outputs[0].Text != "hello" {
		t.Fatalf("text output cell = %#v", cell)
	}
}

func TestEvaluateCellNotebookHTMLHelper(t *testing.T) {
	cell := Cell{ID: "helper-html", Kind: "code", Source: `(joker.notebook/html "<b>Hello</b>")`}
	EvaluateCell(&cell)
	if cell.State != "ok" || len(cell.Outputs) != 1 || cell.Outputs[0].Type != "html" || !strings.Contains(cell.Outputs[0].Source, "Hello") {
		t.Fatalf("html output cell = %#v", cell)
	}
}

func TestEvaluateCellNotebookTableHelper(t *testing.T) {
	cell := Cell{ID: "helper-table", Kind: "code", Source: `(joker.notebook/table [{:name "Ada" :score 42}])`}
	EvaluateCell(&cell)
	if cell.State != "ok" || len(cell.Outputs) != 1 || cell.Outputs[0].Type != "table" || !strings.Contains(cell.Outputs[0].Source, `"name"`) {
		t.Fatalf("table output cell = %#v", cell)
	}
}

func TestEvaluateCellNotebookHelperAcceptsMaps(t *testing.T) {
	cell := Cell{ID: "helper-map", Kind: "code", Source: `(joker.notebook/chart {:data [4 5]})`}
	EvaluateCell(&cell)
	if cell.State != "ok" || len(cell.Outputs) != 1 || cell.Outputs[0].Type != "chart" || !strings.Contains(cell.Outputs[0].Spec, `"data"`) || !strings.Contains(cell.Outputs[0].Spec, `[4,5]`) {
		t.Fatalf("helper map output cell = %#v", cell)
	}
}

func TestEvaluateCellNotebookHelper(t *testing.T) {
	cell := Cell{ID: "helper", Kind: "code", Source: `(joker.notebook/chart "{\"data\":[4,5]}")`}
	EvaluateCell(&cell)
	if cell.State != "ok" || len(cell.Outputs) != 1 || cell.Outputs[0].Type != "chart" || cell.Outputs[0].Renderer != "echarts" || !strings.Contains(cell.Outputs[0].Spec, "data") {
		t.Fatalf("helper output cell = %#v", cell)
	}
}

func TestEvaluateCellRichOutput(t *testing.T) {
	cell := Cell{ID: "rich", Kind: "code", Source: `{:notebook/output :chart :spec "{\"data\":[1,2,3]}"}`}
	EvaluateCell(&cell)
	if cell.State != "ok" || len(cell.Outputs) != 1 || cell.Outputs[0].Type != "chart" || !strings.Contains(cell.Outputs[0].Spec, "data") {
		t.Fatalf("rich output cell = %#v", cell)
	}
}

func TestEvaluateCellValueOutput(t *testing.T) {
	cell := Cell{ID: "value", Kind: "code", Source: `(+ 1 2)`}
	EvaluateCell(&cell)
	if cell.State != "ok" || len(cell.Outputs) != 1 || cell.Outputs[0].Type != "value" || cell.Outputs[0].Text != "3" {
		t.Fatalf("value output cell = %#v", cell)
	}
}

func TestExportMarkdown(t *testing.T) {
	nb := New("Export")
	nb.Cells = []Cell{{ID: "cell-1", Kind: "code", Source: "(+ 1 2)", Outputs: []Output{{Type: "stdout", Text: "3\n"}, {Type: "value", Text: "{:ok true}"}, {Type: "chart", Spec: `{"data":[1,2]}`}, {Type: "diagram", Renderer: "mermaid", Source: "graph TD; A-->B"}, {Type: "graph", Source: `{"nodes":[]}`}, {Type: "table", Source: `[{"name":"Ada"}]`}, {Type: "html", Source: `<b>Hello</b>`}}}}
	var b bytes.Buffer
	if err := ExportMarkdown(&b, nb); err != nil {
		t.Fatal(err)
	}
	md := b.String()
	for _, want := range []string{"```clojure", "```text", "```edn", "```json", "```mermaid", "graph TD; A-->B"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown missing %q:\n%s", want, md)
		}
	}
}

func TestFindCell(t *testing.T) {
	nb := New("Find")
	nb.Cells = []Cell{{ID: "cell-1"}}
	if _, ok := findCell(&nb, "cell-1"); !ok {
		t.Fatal("cell not found")
	}
	if _, ok := findCell(&nb, "missing"); ok {
		t.Fatal("missing cell found")
	}
}

func TestSameOrigin(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/save", nil)
	if !sameOrigin(req) {
		t.Fatal("empty origin should be accepted")
	}
	req.Header.Set("Origin", "http://127.0.0.1:8080")
	if !sameOrigin(req) {
		t.Fatal("matching origin should be accepted")
	}
	req.Header.Set("Origin", "http://evil.example")
	if sameOrigin(req) {
		t.Fatal("cross origin should be rejected")
	}
}

func TestNotebookHTTPHandlerReadOnlyRejectsMutation(t *testing.T) {
	old := ReadOnly
	ReadOnly = true
	defer func() { ReadOnly = old }()
	path := t.TempDir() + "/api.edn"
	nb := New("API")
	nb.Cells = []Cell{{ID: "cell-1", Kind: "code", Source: "(+ 1 2)"}}
	if err := Save(path, nb); err != nil {
		t.Fatal(err)
	}
	h := Handler(path)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/evaluate-all", nil))
	if w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "read-only") {
		t.Fatalf("read-only mutation code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/notebook", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("read-only GET code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestNotebookHTTPHandlerRequiresTokenWhenConfigured(t *testing.T) {
	old := AuthToken
	AuthToken = "secret"
	defer func() { AuthToken = old }()
	path := t.TempDir() + "/api.edn"
	nb := New("API")
	nb.Cells = []Cell{{ID: "cell-1", Kind: "code", Source: "(+ 1 2)"}}
	if err := Save(path, nb); err != nil {
		t.Fatal(err)
	}
	h := Handler(path)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/evaluate-all", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("missing token code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/evaluate-all", nil)
	req.Header.Set("X-Joker-Notebook-Token", "secret")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("token request code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestNotebookHTTPHandlerRejectsCrossOriginMutation(t *testing.T) {
	path := t.TempDir() + "/api.edn"
	nb := New("API")
	nb.Cells = []Cell{{ID: "cell-1", Kind: "code", Source: "(+ 1 2)"}}
	if err := Save(path, nb); err != nil {
		t.Fatal(err)
	}
	h := Handler(path)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/api/evaluate-all", nil)
	req.Header.Set("Origin", "http://evil.example")
	h.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-origin mutation code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestNotebookHTTPHandler(t *testing.T) {
	path := t.TempDir() + "/api.edn"
	nb := New("API")
	nb.Cells = []Cell{{ID: "cell-1", Kind: "code", Name: "data", Source: "(+ 1 2)"}, {ID: "cell-2", Kind: "code", Name: "summary", DependsOn: []string{"data"}, Source: "(+ 3 4)"}}
	if err := Save(path, nb); err != nil {
		t.Fatal(err)
	}
	h := Handler(path)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/notebook", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), ":format :joker/notebook") || w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("GET notebook code=%d headers=%v body=%s", w.Code, w.Header(), w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/status", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "cellCount") || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("status code=%d content-type=%s body=%s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/snapshots", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("snapshots code=%d content-type=%s body=%s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/restore-snapshot?path=missing", nil))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("restore missing code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/export/markdown", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "# API") || !strings.Contains(w.Header().Get("Content-Type"), "text/markdown") {
		t.Fatalf("export markdown code=%d content-type=%s body=%s", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(Encode(Notebook{Format: "joker/notebook", Version: 1, Title: "Replaced", Cells: []Cell{{ID: "cell-1", Kind: "code", Source: "(+ 9 1)"}, {ID: "cell-2", Kind: "code", Name: "summary", DependsOn: []string{"data"}, Source: "(+ 3 4)"}}}))))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Replaced") || !strings.Contains(w.Body.String(), "(+ 9 1)") {
		t.Fatalf("save replace code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/save-sources", strings.NewReader(`{"cells":[{"id":"cell-1","kind":"code","name":"data","source":"(+ 2 3)"}]}`)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "(+ 2 3)") {
		t.Fatalf("save-sources code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/evaluate-cell?id=cell-1", strings.NewReader("(+ 2 3)")))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), ":text \"5\"") {
		t.Fatalf("evaluate-cell code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/dependencies", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "cycles") || !strings.Contains(w.Body.String(), "graph") {
		t.Fatalf("dependencies code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/evaluate-downstream?name=data", strings.NewReader(`{"cells":[{"id":"cell-2","kind":"code","name":"summary","dependsOn":["data"],"source":"(+ 4 5)"}]}`)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), ":text \"9\"") {
		t.Fatalf("evaluate-downstream code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/clear-outputs?id=cell-1", nil))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), ":text \"5\"") {
		t.Fatalf("clear outputs code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/cell?kind=markdown", nil))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), ":kind :markdown") {
		t.Fatalf("add cell code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/reorder", strings.NewReader(`{"ids":["cell-3","cell-2","cell-1"]}`)))
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "cell-3") {
		t.Fatalf("reorder code=%d body=%s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodDelete, "/api/cell?id=cell-3", nil))
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "Markdown") {
		t.Fatalf("delete cell code=%d body=%s", w.Code, w.Body.String())
	}
}

func TestNotebookPageReflectsReadOnlyMode(t *testing.T) {
	old := ReadOnly
	ReadOnly = true
	defer func() { ReadOnly = old }()
	nb := New("ReadOnlyPage")
	var w bytes.Buffer
	if err := page.Execute(&w, nb); err != nil {
		t.Fatal(err)
	}
	html := w.String()
	if !strings.Contains(html, "NOTEBOOK_READONLY") || !strings.Contains(html, "true") || !strings.Contains(html, "read-only-mode") || !strings.Contains(html, "applyReadOnly") {
		t.Fatalf("read-only page missing UI wiring:\n%s", html)
	}
}

func TestNotebookPageIncludesTokenWhenConfigured(t *testing.T) {
	old := AuthToken
	AuthToken = "secret"
	defer func() { AuthToken = old }()
	nb := New("TokenPage")
	var w bytes.Buffer
	if err := page.Execute(&w, nb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.String(), `NOTEBOOK_TOKEN="secret"`) || !strings.Contains(w.String(), "X-Joker-Notebook-Token") {
		t.Fatalf("token page missing token wiring:\n%s", w.String())
	}
}

func TestNotebookPageRenders(t *testing.T) {
	nb := New("Web")
	nb.Cells = []Cell{{ID: "cell-1", Kind: "code", Name: "data", Source: "(+ 1 2)", Outputs: []Output{{Type: "chart", Spec: `{"data":[1,2,3]}`}, {Type: "graph", Source: `{"nodes":[{"id":"A"}],"edges":[]}`}}}}
	var w bytes.Buffer
	if err := page.Execute(&w, nb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.String(), "table-filter") || !strings.Contains(w.String(), "filterRows") || !strings.Contains(w.String(), "Show all") || !strings.Contains(w.String(), "showAll") || !strings.Contains(w.String(), "Showing ") || !strings.Contains(w.String(), "truncated") || !strings.Contains(w.String(), "sortTable") || !strings.Contains(w.String(), "data-col") || !strings.Contains(w.String(), "updateRaw") || !strings.Contains(w.String(), "renderTables") || !strings.Contains(w.String(), "table-output") || !strings.Contains(w.String(), "markdown-preview") || !strings.Contains(w.String(), "function md") || !strings.Contains(w.String(), "showSnapshots") || !strings.Contains(w.String(), "restoreSnapshot") || !strings.Contains(w.String(), "snapshot-list") || !strings.Contains(w.String(), "beforeunload") || !strings.Contains(w.String(), "Unsaved changes") || !strings.Contains(w.String(), "keydown") || !strings.Contains(w.String(), "activeCell") || !strings.Contains(w.String(), "data-theme") || !strings.Contains(w.String(), "setTheme") || !strings.Contains(w.String(), "notebook-title") || !strings.Contains(w.String(), "notebook-status") || !strings.Contains(w.String(), "notebook-log") || !strings.Contains(w.String(), "apiText") || !strings.Contains(w.String(), "cell-header") || !strings.Contains(w.String(), "state-pill") || !strings.Contains(w.String(), "Out[") || !strings.Contains(w.String(), "Evaluate all") || !strings.Contains(w.String(), "Export Markdown") || !strings.Contains(w.String(), "Load raw EDN") || !strings.Contains(w.String(), "Clear all outputs") || !strings.Contains(w.String(), "Clear outputs") || !strings.Contains(w.String(), "Evaluate downstream") || !strings.Contains(w.String(), "Check deps") || !strings.Contains(w.String(), "Show dependency graph") || !strings.Contains(w.String(), "Add code") || !strings.Contains(w.String(), "cell-name") || !strings.Contains(w.String(), "cell-deps") || !strings.Contains(w.String(), "deleteCell") || !strings.Contains(w.String(), "moveCell") || !strings.Contains(w.String(), "highlight") || !strings.Contains(w.String(), "renderCharts") || !strings.Contains(w.String(), "renderGraphs") || !strings.Contains(w.String(), "save-sources") {
		t.Fatalf("page missing expected UI:\n%s", w.String())
	}
}

func TestCellMutationHelpers(t *testing.T) {
	nb := New("Mutate")
	nb.Cells = []Cell{{ID: "cell-1"}, {ID: "cell-3"}}
	if got := nextCellID(nb); got != "cell-4" {
		t.Fatalf("nextCellID = %s", got)
	}
	if !deleteCell(&nb, "cell-1") || len(nb.Cells) != 1 || nb.Cells[0].ID != "cell-3" {
		t.Fatalf("delete result = %#v", nb.Cells)
	}
}

func TestApplyReorder(t *testing.T) {
	nb := New("Order")
	nb.Cells = []Cell{{ID: "a"}, {ID: "b"}}
	if err := applyReorder(strings.NewReader(`{"ids":["b","a"]}`), &nb); err != nil {
		t.Fatal(err)
	}
	if nb.Cells[0].ID != "b" || nb.Cells[1].ID != "a" {
		t.Fatalf("reorder = %#v", nb.Cells)
	}
}

func TestApplySourceUpdate(t *testing.T) {
	nb := New("Update")
	nb.Cells = []Cell{{ID: "cell-1", Source: "old"}}
	if err := applySourceUpdate(strings.NewReader(`{"title":"Updated","cells":[{"id":"cell-1","kind":"markdown","name":"intro","dependsOn":["data"],"source":"new"}]}`), &nb); err != nil {
		t.Fatal(err)
	}
	if nb.Title != "Updated" || nb.Cells[0].Source != "new" || nb.Cells[0].Kind != "markdown" || nb.Cells[0].Name != "intro" || len(nb.Cells[0].DependsOn) != 1 || nb.Cells[0].DependsOn[0] != "data" {
		t.Fatalf("cell = %#v", nb.Cells[0])
	}
}

func TestBuildDependencyGraph(t *testing.T) {
	nb := New("Graph")
	nb.Cells = []Cell{{ID: "1", Name: "data"}, {ID: "2", Name: "chart", DependsOn: []string{"data"}}}
	graph := BuildDependencyGraph(nb)
	if len(graph.Nodes) != 2 || len(graph.Edges) != 1 || graph.Edges[0].From != "data" || graph.Edges[0].To != "chart" {
		t.Fatalf("graph = %#v", graph)
	}
}

func TestDependencyCycles(t *testing.T) {
	nb := New("Cycles")
	nb.Cells = []Cell{{ID: "1", Name: "a", DependsOn: []string{"b"}}, {ID: "2", Name: "b", DependsOn: []string{"a"}}}
	if cycles := DependencyCycles(nb); len(cycles) == 0 {
		t.Fatalf("expected dependency cycle")
	}
}

func TestEvaluateDownstream(t *testing.T) {
	nb := New("EvalDeps")
	nb.Cells = []Cell{{ID: "1", Name: "data", Kind: "code", Source: "(def x 1)"}, {ID: "2", Name: "chart", Kind: "code", DependsOn: []string{"data"}, Source: "(+ 1 2)"}}
	ids := EvaluateDownstream(&nb, "data")
	if len(ids) != 1 || ids[0] != "2" || nb.Cells[1].State != "ok" {
		t.Fatalf("EvaluateDownstream ids=%v cells=%#v", ids, nb.Cells)
	}
}

func TestDownstreamManualDependencies(t *testing.T) {
	nb := New("Deps")
	nb.Cells = []Cell{{ID: "1", Name: "data"}, {ID: "2", Name: "chart", DependsOn: []string{"data"}}, {ID: "3", Name: "summary", DependsOn: []string{"chart"}}}
	got := Downstream(nb, "data")
	if len(got) != 2 {
		t.Fatalf("downstream = %#v", got)
	}
}
