package core

// ir_exported.go — exported functions for the joker.runtime namespace.
// These bridge internal IR/WASM/escape analysis to the public API.

import (
	coreir "github.com/rcarmo/go-joker/core/ir"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
)

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
func ExplainWASMEligibility(prog *IRProgram) corewasm.Diagnostic {
	return explainWASMEligibility(prog)
}

// AnalyzeEscapesExported returns the safe-mutable-slots boolean array.
func AnalyzeEscapesExported(prog *IRProgram) []bool {
	info := analyzeEscapes(prog)
	return info.SafeMutableSlots
}

// IRProgram accessor methods for external packages.
func (p *IRProgram) CodeLen() int {
	model := p.neutralModel()
	return len(model.Code)
}

func (p *IRProgram) CodeBytes() []byte {
	model := p.neutralModel()
	return append([]byte(nil), model.Code...)
}

func (p *IRProgram) ConstLen() int       { return len(p.constants) }
func (p *IRProgram) Constants() []Object { return append([]Object(nil), p.constants...) }
func (p *IRProgram) NumSlots() int {
	model := p.neutralModel()
	return model.NumSlots
}
func (p *IRProgram) HasSelf() bool          { return p.hasSelf }
func (p *IRProgram) CaptureSlots() []Object { return p.captureSlots }
func (p *IRProgram) GetNativeHelper() func([]float64) float64 {
	if nativeHelper, ok := runtimeExec.NativeHelper(p); ok {
		return func(args []float64) float64 { return nativeHelper(args) }
	}
	return nil
}

// Exports for std/jit and std/runtime namespaces.
func IrCompileFn(fn *Fn) *IRProgram                  { return irCompileFn(fn) }
func IrExecTyped(prog *IRProgram, s []Object) Object { return irExecTyped(prog, s) }
func IrExec(prog *IRProgram, s []Object) Object      { return irExec(prog, s) }

func IsFloatExported(prog *IRProgram) bool {
	model := prog.neutralModel()
	return model != nil && corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0)
}

func IrToWasmExported(prog *IRProgram) []byte { return irToWasm(prog) }

func WasmCompileBytesExported(prog *IRProgram) []byte {
	wp := wasmCompile(prog)
	if wp == nil {
		return nil
	}
	return append([]byte(nil), wp.bytes...)
}

type IRAnalysisExported struct {
	Eligible       bool
	Path           string
	HasCallSlot    bool
	HasSelfCall    bool
	UsesCollection bool
	UsesString     bool
	HasMapOps      bool
	HasAssoc       bool
	HasGenericNth  bool
}

func AnalyzeIRProgramExported(prog *IRProgram) IRAnalysisExported {
	a := AnalyzeIRProgram(prog)
	return IRAnalysisExported{
		Eligible:       irTypedEligible(a),
		Path:           a.SuggestedPath,
		HasCallSlot:    a.HasCallSlot,
		HasSelfCall:    a.HasSelfCall,
		UsesCollection: a.UsesCollection,
		UsesString:     a.UsesString,
		HasMapOps:      a.HasMapOps,
		HasAssoc:       a.HasAssoc,
		HasGenericNth:  a.HasGenericNth,
	}
}
