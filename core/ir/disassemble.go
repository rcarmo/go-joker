package ir

import "fmt"

// OpCount counts bytecode instructions in a program. It intentionally mirrors
// the current variable-width Recur encoding used by the core compiler.
func OpCount(code []byte) int {
	pc := 0
	count := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		count++
		switch op {
		case Literal, LoadSlot, StoreSlot, JumpIfNot, Jump, CallSelf, BuildVec, NthStringASCII:
			pc += 2
		case CallSlot:
			pc += 4
		case Recur:
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

// Disassemble returns a human-readable bytecode listing. constString is called
// for literal operands and should return an empty string when the constant is
// unavailable.
func Disassemble(code []byte, constString func(int) string) string {
	var result string
	pc := 0
	for pc < len(code) {
		op := code[pc]
		name := OpcodeName(op)
		line := fmt.Sprintf("  [%3d] %s", pc, name)
		switch op {
		case Literal:
			idx := int(code[pc+1])<<8 | int(code[pc+2])
			if constString != nil {
				if s := constString(idx); s != "" {
					line += fmt.Sprintf(" const[%d]=%s", idx, s)
				} else {
					line += fmt.Sprintf(" const[%d]", idx)
				}
			} else {
				line += fmt.Sprintf(" const[%d]", idx)
			}
			pc += 3
		case LoadSlot, StoreSlot:
			idx := int(code[pc+1])<<8 | int(code[pc+2])
			line += fmt.Sprintf(" slot[%d]", idx)
			pc += 3
		case JumpIfNot, Jump:
			target := int(code[pc+1])<<8 | int(code[pc+2])
			line += fmt.Sprintf(" -> %d", target)
			pc += 3
		case CallSlot:
			slot := int(code[pc+1])<<8 | int(code[pc+2])
			nargs := int(code[pc+3])<<8 | int(code[pc+4])
			line += fmt.Sprintf(" slot[%d] nargs=%d", slot, nargs)
			pc += 5
		case CallSelf:
			nargs := int(code[pc+1])<<8 | int(code[pc+2])
			line += fmt.Sprintf(" nargs=%d", nargs)
			pc += 3
		case Recur:
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
		case MakeFn:
			idx := int(code[pc+1])<<8 | int(code[pc+2])
			line += fmt.Sprintf(" fnExpr[%d]", idx)
			pc += 3
		case Case:
			slot := int(code[pc+1])<<8 | int(code[pc+2])
			nc := int(code[pc+3])<<8 | int(code[pc+4])
			line += fmt.Sprintf(" slot[%d] cases=%d", slot, nc)
			pc += 5 + nc*4 + 2
		case TryCatch:
			pc += 5
		case Subs:
			pc += 2
		default:
			pc++
		}
		result += line + "\n"
	}
	return result
}
