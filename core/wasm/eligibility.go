package wasm

import coreir "github.com/rcarmo/go-joker/core/ir"

// Eligible reports whether IR bytecode can map to the pure single-function WASM backend.
func Eligible(code []byte) bool {
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case coreir.Literal, coreir.LoadSlot, coreir.StoreSlot, coreir.NthStringASCII:
			pc += 2
		case coreir.Add, coreir.Sub, coreir.Mul, coreir.Rem, coreir.Inc, coreir.Dec,
			coreir.Lt, coreir.Gte, coreir.Gt, coreir.Lte, coreir.Eq, coreir.IsZero, coreir.Return:
			// ok
		case coreir.CallSelf:
			pc += 2
		case coreir.Div, coreir.Sqrt:
			// ok — float ops, need f64 mode
		case coreir.JumpIfNot, coreir.Jump:
			pc += 2
		case coreir.Recur:
			pc += 4
			tgt := int(code[pc-2])<<8 | int(code[pc-1])
			if tgt != 0 {
				pc += 2
				return false
			}
		default:
			return false
		}
	}
	return true
}

// EligibleWithHelper reports whether IR bytecode can target the two-function helper WASM backend.
func EligibleWithHelper(code []byte, helperSlot int) bool {
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case coreir.Literal, coreir.LoadSlot, coreir.StoreSlot, coreir.NthStringASCII:
			pc += 2
		case coreir.Add, coreir.Sub, coreir.Mul, coreir.Rem, coreir.Inc, coreir.Dec,
			coreir.Lt, coreir.Gte, coreir.Gt, coreir.Lte, coreir.Eq, coreir.IsZero, coreir.Return, coreir.Div, coreir.Sqrt:
			// supported
		case coreir.CallSlot:
			slot := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			if slot != helperSlot {
				return false
			}
		case coreir.CallSelf:
			return false
		case coreir.JumpIfNot, coreir.Jump:
			pc += 2
		case coreir.Recur:
			pc += 4
			tgt := int(code[pc-2])<<8 | int(code[pc-1])
			if tgt != 0 {
				pc += 2
				return false
			}
		default:
			return false
		}
	}
	return true
}

// EligibleWithImports reports whether IR bytecode can target the host-import WASM backend.
func EligibleWithImports(code []byte) bool {
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case coreir.Literal, coreir.LoadSlot, coreir.StoreSlot, coreir.NthStringASCII:
			pc += 2
		case coreir.Add, coreir.Sub, coreir.Mul, coreir.Div, coreir.Rem, coreir.Inc, coreir.Dec,
			coreir.Lt, coreir.Gte, coreir.Gt, coreir.Lte, coreir.Eq, coreir.IsZero, coreir.Return, coreir.Sqrt,
			coreir.Get, coreir.Get3, coreir.Assoc, coreir.Nth, coreir.Conj, coreir.First, coreir.Count:
			// supported with imports
		case coreir.CallSelf:
			return false
		case coreir.Str1, coreir.Str2, coreir.BuildVec, coreir.ToTransient, coreir.AssocBang, coreir.ToPersistent, coreir.CallSlot:
			return false
		case coreir.JumpIfNot, coreir.Jump:
			pc += 2
		case coreir.Recur:
			pc += 4
			tgt := int(code[pc-2])<<8 | int(code[pc-1])
			if tgt != 0 {
				pc += 2
				return false
			}
		default:
			return false
		}
	}
	return true
}

// UsesFloat reports whether IR bytecode or constants require f64 mode.
func UsesFloat(code []byte, hasFloatConsts bool) bool {
	if hasFloatConsts {
		return true
	}
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case coreir.Div, coreir.Sqrt:
			return true
		case coreir.Literal, coreir.LoadSlot, coreir.StoreSlot, coreir.NthStringASCII:
			pc += 2
		case coreir.JumpIfNot, coreir.Jump:
			pc += 2
		case coreir.Recur:
			pc += 4
			tgt := int(code[pc-2])<<8 | int(code[pc-1])
			if tgt != 0 {
				pc += 2
			}
		default:
			// single-byte opcodes
		}
	}
	return false
}
