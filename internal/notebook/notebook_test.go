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

func TestDownstreamManualDependencies(t *testing.T) {
	nb := New("Deps")
	nb.Cells = []Cell{{ID: "1", Name: "data"}, {ID: "2", Name: "chart", DependsOn: []string{"data"}}, {ID: "3", Name: "summary", DependsOn: []string{"chart"}}}
	got := Downstream(nb, "data")
	if len(got) != 2 {
		t.Fatalf("downstream = %#v", got)
	}
}
