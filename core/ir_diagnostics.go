package core

import coreir "github.com/rcarmo/go-joker/core/internal/ir"

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
	WASM        WASMDiagnostic
	Analysis    IRAnalysis
}

type WASMDiagnostic struct {
	Eligible   bool
	Reason     string
	PC         int
	Op         byte
	OpName     string
	UsesFloat  bool
	HasImports bool
}

func irOpcodeName(op byte) string { return coreir.OpcodeName(op) }

func irOpCount(code []byte) int { return coreir.OpCount(code) }

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
	return IRDiagnostic{
		Compiled:    true,
		NumSlots:    prog.numSlots,
		NumCaptures: len(prog.captureKeys),
		NumOps:      irOpCount(prog.code),
		UsesWASM:    wasm.Eligible && !wasm.HasImports,
		WASM:        wasm,
		Analysis:    analysis,
	}
}

func explainWASMEligibility(prog *IRProgram) WASMDiagnostic {
	if prog == nil {
		return WASMDiagnostic{Reason: "nil IR program"}
	}
	code := prog.code
	pc := 0
	usesFloat := irProgramUsesFloat(prog)
	for pc < len(code) {
		opPC := pc
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irRem, irInc, irDec,
			irLt, irGte, irGt, irLte, irEq, irIsZero, irReturn:
			// pure WASM supported
		case irDiv, irSqrt:
			// pure WASM supported in f64 mode
		case irCallSelf:
			pc += 2
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			tgt := int(code[pc-2])<<8 | int(code[pc-1])
			if tgt != 0 {
				return WASMDiagnostic{Reason: "nested loop recur not supported by pure WASM backend", PC: opPC, Op: op, OpName: irOpcodeName(op), UsesFloat: usesFloat}
			}
		case irGet, irGet3, irAssoc, irNth, irConj, irFirst, irCount:
			return WASMDiagnostic{Reason: "requires WASM host imports for collection op", PC: opPC, Op: op, OpName: irOpcodeName(op), UsesFloat: usesFloat, HasImports: true}
		case irStr1, irStr2, irNthStringASCII:
			return WASMDiagnostic{Reason: "string operation not supported by WASM backend", PC: opPC, Op: op, OpName: irOpcodeName(op), UsesFloat: usesFloat}
		case irCallSlot:
			return WASMDiagnostic{Reason: "local/helper function call needs multi-function WASM module", PC: opPC, Op: op, OpName: irOpcodeName(op), UsesFloat: usesFloat}
		case irBuildVec, irToTransient, irAssocBang, irToPersistent:
			return WASMDiagnostic{Reason: "transient/vector object operation not supported by WASM backend", PC: opPC, Op: op, OpName: irOpcodeName(op), UsesFloat: usesFloat}
		default:
			return WASMDiagnostic{Reason: "unsupported opcode for WASM backend", PC: opPC, Op: op, OpName: irOpcodeName(op), UsesFloat: usesFloat}
		}
	}
	return WASMDiagnostic{Eligible: true, Reason: "eligible for pure WASM backend", UsesFloat: usesFloat}
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
