package core

import (
	"strings"
	"testing"
)

func TestIRDiagnosticsPureWASM(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 acc 0]
  (if (= i 100)
    acc
    (recur (inc i) (+ acc i))))`)
	d := explainFirstLoop(expr)
	if !d.Compiled {
		t.Fatalf("expected IR compile, got reason %q", d.Reason)
	}
	if !d.WASM.Eligible {
		t.Fatalf("expected pure WASM eligibility, got %+v", d.WASM)
	}
	if !d.UsesWASM {
		t.Fatalf("expected UsesWASM for pure numeric loop: %+v", d)
	}
}

func TestIRDiagnosticsCollectionNeedsImports(t *testing.T) {
	expr := compileTestExpr(t, `(let [ks [:a :b]]
  (loop [i 0 m {}]
    (if (= i 10)
      (get m :a 0)
      (let [k (nth ks (rem i 2))]
        (recur (inc i) (assoc m k (+ 1 (get m k 0))))))))`)
	d := explainFirstLoop(expr)
	if !d.Compiled {
		t.Fatalf("expected IR compile, got reason %q", d.Reason)
	}
	if d.WASM.Eligible {
		t.Fatalf("expected WASM rejection for collection loop")
	}
	if !d.WASM.HasImports || !strings.Contains(d.WASM.Reason, "host imports") {
		t.Fatalf("expected host-import diagnostic, got %+v", d.WASM)
	}
}

func TestIRDiagnosticsStringOpNotWASM(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 s ""]
  (if (= i 3)
    s
    (recur (inc i) (str s "x"))))`)
	d := explainFirstLoop(expr)
	if !d.Compiled {
		t.Fatalf("expected IR compile, got reason %q", d.Reason)
	}
	if d.WASM.Eligible {
		t.Fatalf("expected WASM rejection for string op")
	}
	if d.WASM.OpName != "irStr2" {
		t.Fatalf("expected irStr2 diagnostic, got %+v", d.WASM)
	}
}

func TestIRDiagnosticsNoLoop(t *testing.T) {
	d := explainFirstLoop(compileTestExpr(t, `(+ 1 2)`))
	if d.Compiled || d.Reason != "no loop expression found" {
		t.Fatalf("unexpected diagnostic: %+v", d)
	}
}
