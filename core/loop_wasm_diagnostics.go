package core

import (
	coreir "github.com/rcarmo/go-joker/core/ir"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
)

// ir_diagnostics.go — lightweight IR/WASM compilation explanations.
//
// These helpers are intentionally internal: they give benchmark and regression
// tests a stable way to answer "which execution path did this hot form take?"
// without changing Joker's public language surface. The goal is to make future
// core-runtime speed work measurable instead of guess-driven.

type IRDiagnostic struct {
	Compiled    bool
	Reason      string
	BodyIndex   int
	NumSlots    int
	NumCaptures int
	NumOps      int
	UsesWASM    bool
	WASM        corewasm.Diagnostic
	Analysis    IRAnalysis
}

func explainIRCompile(loop *LoopExpr) IRDiagnostic {
	if loop == nil {
		return IRDiagnostic{Reason: "nil loop"}
	}
	prog, reason := irCompileExplain(loop)
	if prog == nil {
		if reason == "" {
			reason = "IR compiler rejected loop body (unsupported form or unsafe binding shape)"
		}
		return IRDiagnostic{Reason: reason}
	}
	wasm := explainWASMEligibility(prog)
	analysis := AnalyzeIRProgram(prog)
	model := prog.neutralModel()
	return IRDiagnostic{
		Compiled:    true,
		NumSlots:    model.NumSlots,
		NumCaptures: len(prog.captureKeys),
		NumOps:      coreir.OpCount(model.Code),
		UsesWASM:    wasm.Eligible && !wasm.HasImports,
		WASM:        wasm,
		Analysis:    analysis,
	}
}

func explainWASMEligibility(prog *IRProgram) corewasm.Diagnostic {
	if prog == nil {
		return corewasm.Diagnostic{Reason: "nil IR program"}
	}
	model := prog.neutralModel()
	if model == nil {
		return corewasm.Diagnostic{Reason: "nil IR program model"}
	}
	return corewasm.ExplainEligibility(model.Code, len(model.FloatConsts) > 0)
}

func findFirstLoopExpr(expr Expr) *LoopExpr {
	switch e := expr.(type) {
	case *LoopExpr:
		return e
	case *LetExpr:
		for _, v := range e.values {
			if loop := findFirstLoopExpr(v); loop != nil {
				return loop
			}
		}
		for _, b := range e.body {
			if loop := findFirstLoopExpr(b); loop != nil {
				return loop
			}
		}
	case *IfExpr:
		if loop := findFirstLoopExpr(e.cond); loop != nil {
			return loop
		}
		if loop := findFirstLoopExpr(e.positive); loop != nil {
			return loop
		}
		return findFirstLoopExpr(e.negative)
	case *CallExpr:
		if loop := findFirstLoopExpr(e.callable); loop != nil {
			return loop
		}
		for _, a := range e.args {
			if loop := findFirstLoopExpr(a); loop != nil {
				return loop
			}
		}
	case *RecurExpr:
		for _, a := range e.args {
			if loop := findFirstLoopExpr(a); loop != nil {
				return loop
			}
		}
	}
	return nil
}

func explainFirstLoop(expr Expr) IRDiagnostic {
	loop := findFirstLoopExpr(expr)
	if loop == nil {
		return IRDiagnostic{Reason: "no loop expression found"}
	}
	return explainIRCompile(loop)
}
