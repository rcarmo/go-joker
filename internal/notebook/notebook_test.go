package notebook

import (
	"bytes"
	"strings"
	"testing"
)

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

func TestExportMarkdown(t *testing.T) {
	nb := New("Export")
	nb.Cells = []Cell{{ID: "cell-1", Kind: "code", Source: "(+ 1 2)", Outputs: []Output{{Type: "stdout", Text: "3\n"}}}}
	var b bytes.Buffer
	if err := ExportMarkdown(&b, nb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), "```clojure") || !strings.Contains(b.String(), "3") {
		t.Fatalf("markdown:\n%s", b.String())
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

func TestNotebookPageRenders(t *testing.T) {
	nb := New("Web")
	nb.Cells = []Cell{{ID: "cell-1", Kind: "code", Name: "data", Source: "(+ 1 2)", Outputs: []Output{{Type: "chart", Spec: `{"data":[1,2,3]}`}, {Type: "graph", Source: `{"nodes":[{"id":"A"}],"edges":[]}`}}}}
	var w bytes.Buffer
	if err := page.Execute(&w, nb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(w.String(), "Evaluate all") || !strings.Contains(w.String(), "Evaluate downstream") || !strings.Contains(w.String(), "Add code") || !strings.Contains(w.String(), "deleteCell") || !strings.Contains(w.String(), "moveCell") || !strings.Contains(w.String(), "highlight") || !strings.Contains(w.String(), "renderCharts") || !strings.Contains(w.String(), "renderGraphs") || !strings.Contains(w.String(), "save-sources") {
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
	if err := applySourceUpdate(strings.NewReader(`{"cells":[{"id":"cell-1","source":"new"}]}`), &nb); err != nil {
		t.Fatal(err)
	}
	if nb.Cells[0].Source != "new" {
		t.Fatalf("source = %q", nb.Cells[0].Source)
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
