package core

// ir_exported.go — exported functions for the joker.runtime namespace.
// These bridge internal IR/WASM/escape analysis to the public API.

import "fmt"

// IrDisassemble returns a human-readable disassembly of an IR program.
func IrDisassemble(prog *IRProgram) string {
	if prog == nil {
		return "; nil program"
	}
	var result string
	code := prog.code
	pc := 0
	for pc < len(code) {
		op := code[pc]
		name := irOpcodeName(op)
		line := fmt.Sprintf("  [%3d] %s", pc, name)
		switch op {
		case irLiteral:
			idx := int(code[pc+1])<<8 | int(code[pc+2])
			if idx < len(prog.constants) && prog.constants[idx] != nil {
				line += fmt.Sprintf(" const[%d]=%s", idx, prog.constants[idx].ToString(false))
			} else {
				line += fmt.Sprintf(" const[%d]", idx)
			}
			pc += 3
		case irLoadSlot, irStoreSlot:
			idx := int(code[pc+1])<<8 | int(code[pc+2])
			line += fmt.Sprintf(" slot[%d]", idx)
			pc += 3
		case irJumpIfNot, irJump:
			target := int(code[pc+1])<<8 | int(code[pc+2])
			line += fmt.Sprintf(" -> %d", target)
			pc += 3
		case irCallSlot:
			slot := int(code[pc+1])<<8 | int(code[pc+2])
			nargs := int(code[pc+3])<<8 | int(code[pc+4])
			line += fmt.Sprintf(" slot[%d] nargs=%d", slot, nargs)
			pc += 5
		case irCallSelf:
			nargs := int(code[pc+1])<<8 | int(code[pc+2])
			line += fmt.Sprintf(" nargs=%d", nargs)
			pc += 3
		case irRecur:
			nb := int(code[pc+1])<<8 | int(code[pc+2])
			tp := int(code[pc+3])<<8 | int(code[pc+4])
			line += fmt.Sprintf(" nBinds=%d target=%d", nb, tp)
			if tp != 0 {
				bs := int(code[pc+5])<<8 | int(code[pc+6])
				line += fmt.Sprintf(" base=%d", bs)
				pc += 7
			} else {
				pc += 5
			}
		case irMakeFn:
			idx := int(code[pc+1])<<8 | int(code[pc+2])
			line += fmt.Sprintf(" fnExpr[%d]", idx)
			pc += 3
		case irCase:
			slot := int(code[pc+1])<<8 | int(code[pc+2])
			nc := int(code[pc+3])<<8 | int(code[pc+4])
			line += fmt.Sprintf(" slot[%d] cases=%d", slot, nc)
			pc += 5 + nc*4 + 2
		case irTryCatch:
			pc += 5
		case irSubs:
			pc += 2
		default:
			pc++
		}
		result += line + "\n"
	}
	return result
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
