package core

// ir_exported.go — exported functions for the joker.runtime namespace.
// These bridge internal IR/WASM/escape analysis to the public API.

import coreir "github.com/rcarmo/go-joker/core/internal/ir"

// IrDisassemble returns a human-readable disassembly of an IR program.
func IrDisassemble(prog *IRProgram) string {
	if prog == nil {
		return "; nil program"
	}
	model := prog.neutralModel()
	return coreir.Disassemble(model.Code, func(idx int) string {
		if idx < len(prog.constants) && prog.constants[idx] != nil {
			return prog.constants[idx].ToString(false)
		}
		return ""
	})
}

// ExplainWASMEligibility exposes the WASM diagnostic for a program.
func ExplainWASMEligibility(prog *IRProgram) WASMDiagnostic {
	return explainWASMEligibility(prog)
}

// AnalyzeEscapesExported returns the safe-mutable-slots boolean array.
func AnalyzeEscapesExported(prog *IRProgram) []bool {
	info := analyzeEscapes(prog)
	return info.SafeMutableSlots
}

// IRProgram accessor methods for external packages
