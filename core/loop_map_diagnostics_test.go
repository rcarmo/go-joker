package core

import (
	"strings"
	"testing"
)

func TestIRCompilesLiteralMapInitializer(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0 m {}]
  (if (= i 4)
    (get m :a 0)
    (recur (inc i) (assoc m :a (inc (get m :a 0))))))`)
	d := explainFirstLoop(expr)
	if !d.Compiled {
		t.Fatalf("expected IR compile for empty map initializer, got %q", d.Reason)
	}
	requireInt(t, Eval(expr, nil), 4)
}

func TestIRDiagnosticsUnsupportedDynamicMapLiteral(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0]
  (if (= i 1)
    {:x i}
    (recur (inc i))))`)
	d := explainFirstLoop(expr)
	if d.Compiled {
		t.Fatalf("expected dynamic map literal rejection")
	}
	if !strings.Contains(d.Reason, "dynamic map literal") {
		t.Fatalf("expected dynamic map literal reason, got %q", d.Reason)
	}
}

func TestIRDiagnosticsSpecificUnsupportedCallable(t *testing.T) {
	expr := compileTestExpr(t, `(loop [i 0]
  (if (= i 1)
    i
    ((fn [x] x) (inc i))))`)
	d := explainFirstLoop(expr)
	if d.Compiled {
		t.Fatalf("expected unsupported callable rejection")
	}
	if !strings.Contains(d.Reason, "unsupported callable expression") {
		t.Fatalf("expected unsupported callable reason, got %q", d.Reason)
	}
}
