package core

import (
	"testing"

	corewasm "github.com/rcarmo/go-joker/core/wasm"
)

func TestWasmArithmeticLoopCorrectness(t *testing.T) {
	expr := compileBenchExpr(t, `(loop [i 0 s 0]
  (if (= i 100) s (recur (inc i) (+ s (rem (* i 7) 11)))))`)
	// Get expected result from IR
	expected := Eval(expr, nil)

	// Try WASM compilation
	loop := expr.(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Skip("IR compilation failed")
	}
	model := prog.neutralModel()
	if model == nil || !corewasm.Eligible(model.Code) {
		t.Skip("IR not WASM-eligible")
	}
	wp := wasmCompile(prog)
	if wp == nil {
		t.Skip("WASM compilation failed")
	}
	result := wasmExec(wp, []Object{Int{I: 0}, Int{I: 0}})
	if result == nil {
		t.Fatal("WASM execution returned nil")
	}
	if !result.Equals(expected) {
		t.Fatalf("WASM result %s != IR result %s", result.ToString(false), expected.ToString(false))
	}
}

func TestWasmSimpleLoop(t *testing.T) {
	expr := compileBenchExpr(t, `(loop [i 0 s 0]
  (if (= i 10) s (recur (+ i 1) (+ s i))))`)
	expected := Eval(expr, nil)

	loop := expr.(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Skip("IR failed")
	}
	wp := wasmCompile(prog)
	if wp == nil {
		t.Skip("WASM failed")
	}
	result := wasmExec(wp, []Object{Int{I: 0}, Int{I: 0}})
	if result == nil {
		t.Fatal("nil result")
	}
	if !result.Equals(expected) {
		t.Fatalf("got %s, want %s", result.ToString(false), expected.ToString(false))
	}
}

func TestWasmFloatLoop(t *testing.T) {
	expr := compileBenchExpr(t, `(loop [x 0.0 i 0]
  (if (= i 100) x (recur (+ x (* 0.5 0.5)) (inc i))))`)
	expected := Eval(expr, nil)
	loop := expr.(*LoopExpr)
	prog := irCompile(loop)
	if prog == nil {
		t.Skip("IR failed")
	}
	model := prog.neutralModel()
	if model == nil || !corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0) {
		t.Fatal("float loop should be detected as using float operations/constants")
	}
	if prog.model == nil {
		t.Fatal("compiled IR program should populate neutral model")
	}
	analysis := AnalyzeIRProgram(prog)
	if prog.model.Analysis == nil || !prog.model.Analysis.UsesFloat || !analysis.UsesFloat {
		t.Fatalf("neutral model analysis should preserve float usage: model=%#v analysis=%#v", prog.model.Analysis, analysis)
	}
	t.Logf("float: %v, eligible: %v", corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0), corewasm.Eligible(model.Code))
	wp := wasmCompile(prog)
	if wp == nil {
		t.Skip("WASM failed")
	}
	result := wasmExec(wp, []Object{Double{D: 0.0}, Int{I: 0}})
	if result == nil {
		t.Fatal("nil")
	}
	t.Logf("WASM=%s IR=%s", result.ToString(false), expected.ToString(false))
	// Allow small FP difference
	if result.ToString(false) != expected.ToString(false) {
		t.Fatalf("mismatch: WASM=%s IR=%s", result.ToString(false), expected.ToString(false))
	}
}
