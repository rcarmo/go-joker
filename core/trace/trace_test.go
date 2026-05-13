package trace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readJSONFile(t *testing.T, path string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		t.Fatalf("decode trace json: %v\n%s", err, b)
	}
	return payload
}

func TestFunctionTracerWritesEventsAndEdges(t *testing.T) {
	out := filepath.Join(t.TempDir(), "function.json")
	tr := NewFunctionTracer(true, out)
	endParent := tr.Enter("parent")
	endChild := tr.Enter("child")
	time.Sleep(time.Microsecond)
	endChild()
	endParent()

	payload := readJSONFile(t, out)
	if payload["type"] != "go-joker-function-trace" {
		t.Fatalf("unexpected type: %v", payload["type"])
	}
	if payload["total"].(float64) != 2 {
		t.Fatalf("total = %v, want 2", payload["total"])
	}
	if got := len(payload["functions"].([]any)); got != 2 {
		t.Fatalf("functions len = %d, want 2", got)
	}
	if got := len(payload["edges"].([]any)); got != 1 {
		t.Fatalf("edges len = %d, want 1", got)
	}
	if got := len(payload["events"].([]any)); got != 2 {
		t.Fatalf("events len = %d, want 2", got)
	}
}

func TestSymbolTracerWritesResolveAndDerefRows(t *testing.T) {
	out := filepath.Join(t.TempDir(), "symbol.json")
	tr := NewSymbolTracer(true, out)
	tr.Resolve("joker.core/+")
	tr.Resolve("joker.core/+")
	tr.Deref("joker.core/map")
	tr.Write()

	payload := readJSONFile(t, out)
	if payload["type"] != "go-joker-symbol-trace" {
		t.Fatalf("unexpected type: %v", payload["type"])
	}
	if payload["resolve_total"].(float64) != 2 || payload["deref_total"].(float64) != 1 {
		t.Fatalf("unexpected totals: resolve=%v deref=%v", payload["resolve_total"], payload["deref_total"])
	}
	resolves := payload["resolves"].([]any)
	if resolves[0].(map[string]any)["symbol"] != "joker.core/+" || resolves[0].(map[string]any)["count"].(float64) != 2 {
		t.Fatalf("unexpected resolves row: %#v", resolves[0])
	}
}

func TestIRProfileWritesOpsAndEdges(t *testing.T) {
	out := filepath.Join(t.TempDir(), "ir.json")
	profile := NewIRProfile(true, out)
	profile.ExecStart()
	started := time.Now().Add(-time.Millisecond)
	prevStarted := profile.Op(0, 1, false, started)
	profile.Op(1, 2, true, prevStarted.Add(-time.Millisecond))
	profile.Finish(2, true, time.Now().Add(-time.Millisecond))
	profile.Write(func(op byte) string { return map[byte]string{1: "one", 2: "two"}[op] })

	payload := readJSONFile(t, out)
	if payload["type"] != "go-joker-ir-profile" {
		t.Fatalf("unexpected type: %v", payload["type"])
	}
	if payload["execs"].(float64) != 1 {
		t.Fatalf("execs = %v, want 1", payload["execs"])
	}
	if got := len(payload["ops"].([]any)); got != 2 {
		t.Fatalf("ops len = %d, want 2", got)
	}
	if got := len(payload["edges"].([]any)); got != 1 {
		t.Fatalf("edges len = %d, want 1", got)
	}
}
