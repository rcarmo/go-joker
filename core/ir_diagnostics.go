package core

import "fmt"

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

func irOpcodeName(op byte) string {
	switch op {
	case irLiteral:
		return "irLiteral"
	case irLoadSlot:
		return "irLoadSlot"
	case irStoreSlot:
		return "irStoreSlot"
	case irAdd:
		return "irAdd"
	case irSub:
		return "irSub"
	case irMul:
		return "irMul"
	case irRem:
		return "irRem"
	case irDiv:
		return "irDiv"
	case irInc:
		return "irInc"
	case irDec:
		return "irDec"
	case irLt:
		return "irLt"
	case irGte:
		return "irGte"
	case irGt:
		return "irGt"
	case irLte:
		return "irLte"
	case irCursorChar:
		return "irCursorChar"
	case irCursorNext:
		return "irCursorNext"
	case irCursorDone:
		return "irCursorDone"
	case irPackRest:
		return "irPackRest"
	case irApply:
		return "irApply"
	case irThrow:
		return "irThrow"
	case irTryCatch:
		return "irTryCatch"
	case irPop:
		return "irPop"
	case irMakeFn:
		return "irMakeFn"
	case irEq:
		return "irEq"
	case irIsZero:
		return "irIsZero"
	case irJumpIfNot:
		return "irJumpIfNot"
	case irJump:
		return "irJump"
	case irRecur:
		return "irRecur"
	case irReturn:
		return "irReturn"
	case irGet:
		return "irGet"
	case irGet3:
		return "irGet3"
	case irAssoc:
		return "irAssoc"
	case irNth:
		return "irNth"
	case irConj:
		return "irConj"
	case irSqrt:
		return "irSqrt"
	case irCallSlot:
		return "irCallSlot"
	case irCallSelf:
		return "irCallSelf"
	case irFirst:
		return "irFirst"
	case irBuildVec:
		return "irBuildVec"
	case irStr2:
		return "irStr2"
	case irStr1:
		return "irStr1"
	case irNthStringASCII:
		return "irNthStringASCII"
	case irCount:
		return "irCount"
	case irToTransient:
		return "irToTransient"
	case irAssocBang:
		return "irAssocBang"
	case irToPersistent:
		return "irToPersistent"
	case irFallback:
		return "irFallback"
	default:
		return fmt.Sprintf("irUnknown(%d)", op)
	}
}

func irOpCount(code []byte) int {
	pc := 0
	count := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		count++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot, irJumpIfNot, irJump, irCallSelf, irBuildVec, irNthStringASCII:
			pc += 2
		case irCallSlot:
			pc += 4
		case irRecur:
			pc += 4
			if pc <= len(code) {
				tgt := int(code[pc-2])<<8 | int(code[pc-1])
				if tgt != 0 && pc+2 <= len(code) {
					pc += 2
				}
			}
		}
	}
	return count
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
