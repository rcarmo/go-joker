package core

import "testing"

func TestIRExecutionEnvelopeKeepsRuntimeMetadata(t *testing.T) {
	expr := compileBenchExpr(t, `(loop [i 0 s 0]
  (if (= i 5) s (recur (inc i) (+ s i))))`)
	prog := irCompile(expr.(*LoopExpr))
	if prog == nil {
		t.Fatal("expected loop to compile to IR")
	}
	model := prog.neutralModel()
	if model == nil {
		t.Fatal("expected neutral IR model")
	}
	if model.ConstantsLen != len(prog.constants) {
		t.Fatalf("model constants len = %d, envelope constants len = %d", model.ConstantsLen, len(prog.constants))
	}
	if model.NumSlots != prog.numSlots {
		t.Fatalf("model slots = %d, envelope slots = %d", model.NumSlots, prog.numSlots)
	}
	analysis := AnalyzeIRProgram(prog)
	if prog.escapeInfo == nil {
		t.Fatal("AnalyzeIRProgram should populate root execution escape metadata")
	}
	if model.Analysis == nil || model.Analysis.NumOps != analysis.NumOps {
		t.Fatalf("neutral model analysis not populated from execution analysis: model=%#v analysis=%#v", model.Analysis, analysis)
	}
	got := irExec(prog, []Object{MakeInt(0), MakeInt(0)})
	if !got.Equals(MakeInt(10)) {
		t.Fatalf("irExec result = %s, want 10", got.ToString(false))
	}
}

func TestIRFunctionEnvelopeKeepsCaptureMetadata(t *testing.T) {
	expr := compileBenchExpr(t, `(let [x 10] (fn [y] (+ x y)))`)
	fn := Eval(expr, nil).(*Fn)
	prog := irCompileFn(fn)
	if prog == nil {
		t.Fatal("expected captured fn to compile to IR")
	}
	if len(prog.captureKeys) == 0 || len(prog.captureSlotIdxs) == 0 {
		t.Fatalf("expected root envelope capture metadata: keys=%d idxs=%d", len(prog.captureKeys), len(prog.captureSlotIdxs))
	}
	model := prog.neutralModel()
	if model == nil {
		t.Fatal("expected neutral IR model")
	}
	if len(model.CaptureSlotIdxs) != len(prog.captureSlotIdxs) {
		t.Fatalf("model capture idxs = %d, envelope capture idxs = %d", len(model.CaptureSlotIdxs), len(prog.captureSlotIdxs))
	}
	got := fn.Call([]Object{MakeInt(5)})
	if !got.Equals(MakeInt(15)) {
		t.Fatalf("captured fn result = %s, want 15", got.ToString(false))
	}
}
