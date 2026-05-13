package wasm

import coreir "github.com/rcarmo/go-joker/core/ir"

type Diagnostic struct {
	Eligible   bool
	Reason     string
	PC         int
	Op         byte
	OpName     string
	UsesFloat  bool
	HasImports bool
}

// ExplainEligibility returns a stable reason why bytecode is or is not suited
// to the pure WASM backend.
func ExplainEligibility(code []byte, hasFloatConsts bool) Diagnostic {
	pc := 0
	usesFloat := UsesFloat(code, hasFloatConsts)
	for pc < len(code) {
		opPC := pc
		op := code[pc]
		pc++
		switch op {
		case coreir.Literal, coreir.LoadSlot, coreir.StoreSlot:
			pc += 2
		case coreir.Add, coreir.Sub, coreir.Mul, coreir.Rem, coreir.Inc, coreir.Dec,
			coreir.Lt, coreir.Gte, coreir.Gt, coreir.Lte, coreir.Eq, coreir.IsZero, coreir.Return:
		case coreir.Div, coreir.Sqrt:
		case coreir.CallSelf:
			pc += 2
		case coreir.JumpIfNot, coreir.Jump:
			pc += 2
		case coreir.Recur:
			pc += 4
			tgt := int(code[pc-2])<<8 | int(code[pc-1])
			if tgt != 0 {
				return Diagnostic{Reason: "nested loop recur not supported by pure WASM backend", PC: opPC, Op: op, OpName: coreir.OpcodeName(op), UsesFloat: usesFloat}
			}
		case coreir.Get, coreir.Get3, coreir.Assoc, coreir.Nth, coreir.Conj, coreir.First, coreir.Count:
			return Diagnostic{Reason: "requires WASM host imports for collection op", PC: opPC, Op: op, OpName: coreir.OpcodeName(op), UsesFloat: usesFloat, HasImports: true}
		case coreir.Str1, coreir.Str2, coreir.NthStringASCII:
			return Diagnostic{Reason: "string operation not supported by WASM backend", PC: opPC, Op: op, OpName: coreir.OpcodeName(op), UsesFloat: usesFloat}
		case coreir.CallSlot:
			return Diagnostic{Reason: "local/helper function call needs multi-function WASM module", PC: opPC, Op: op, OpName: coreir.OpcodeName(op), UsesFloat: usesFloat}
		case coreir.BuildVec, coreir.ToTransient, coreir.AssocBang, coreir.ToPersistent:
			return Diagnostic{Reason: "transient/vector object operation not supported by WASM backend", PC: opPC, Op: op, OpName: coreir.OpcodeName(op), UsesFloat: usesFloat}
		default:
			return Diagnostic{Reason: "unsupported opcode for WASM backend", PC: opPC, Op: op, OpName: coreir.OpcodeName(op), UsesFloat: usesFloat}
		}
	}
	return Diagnostic{Eligible: true, Reason: "eligible for pure WASM backend", UsesFloat: usesFloat}
}
