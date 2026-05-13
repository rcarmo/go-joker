package core

import "testing"

func TestIRAnalysisNumericWASMPath(t *testing.T) {
	d := explainFirstLoop(compileTestExpr(t, `(loop [i 0 acc 0]
  (if (= i 10) acc (recur (inc i) (+ acc i))))`))
	if !d.Compiled {
		t.Fatalf("expected IR: %s", d.Reason)
	}
	if d.Analysis.SuggestedPath != "wasm" {
		t.Fatalf("expected wasm path, got %+v", d.Analysis)
	}
}

func TestIRAnalysisStringPrependBuilderPath(t *testing.T) {
	d := explainFirstLoop(compileTestExpr(t, `(let [dna "ACGT"]
  (loop [i 0 s ""]
    (if (= i 4) s (recur (inc i) (str (nth dna i) s)))))`))
	if !d.Compiled {
		t.Fatalf("expected IR: %s", d.Reason)
	}
	if !d.Analysis.HasStringPrepend || d.Analysis.SuggestedPath != "ir-string-prepend-builder" {
		t.Fatalf("expected prepend builder analysis, got %+v", d.Analysis)
	}
}

func TestIRAnalysisCollectionPath(t *testing.T) {
	d := explainFirstLoop(compileTestExpr(t, `(loop [i 0 m {}]
  (if (= i 4) (get m :a 0) (recur (inc i) (assoc m :a i))))`))
	if !d.Compiled {
		t.Fatalf("expected IR: %s", d.Reason)
	}
	if !d.Analysis.UsesCollection {
		t.Fatalf("expected collection analysis, got %+v", d.Analysis)
	}
}
