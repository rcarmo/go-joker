package core

import (
	coreir "github.com/rcarmo/go-joker/core/ir"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
)

// ir_analysis.go — centralized IR shape analysis.
//
// This pass is intentionally conservative. It does not change semantics; it
// gives diagnostics and optimization gates a single program-shape summary so
// future typed-IR, string-builder, helper-inlining and WASM decisions do not
// have to rediscover the same facts independently.

type IRAnalysis = coreir.Analysis

func AnalyzeIRProgram(prog *IRProgram) IRAnalysis {
	if prog == nil {
		return IRAnalysis{SuggestedPath: "none"}
	}
	if prog.analysis != nil {
		return *prog.analysis
	}
	info := prog.escapeInfo
	if info == nil {
		info = analyzeEscapes(prog)
		prog.escapeInfo = info
	}
	model := prog.neutralModel()
	a := coreir.Analyze(
		model.Code,
		model.NumSlots,
		len(prog.captureKeys),
		corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0),
		info.StringBuilderSlots,
		info.StringPrependSlots,
	)
	prog.analysis = &a
	model.Analysis = &a
	return a
}

func suggestIRPath(a IRAnalysis) string { return coreir.SuggestPath(a) }
