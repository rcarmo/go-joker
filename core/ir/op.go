package ir

import "fmt"

// Opcode values for go-joker's lowered IR bytecode.
const (
	Literal byte = iota
	LoadSlot
	StoreSlot
	Add
	Sub
	Mul
	Rem
	Div
	Inc
	Dec
	Lt
	Eq
	IsZero
	JumpIfNot
	Jump
	Recur
	Return
	Get
	Get3
	Assoc
	Nth
	Conj
	Sqrt
	CallSlot
	CallSelf
	First
	BuildVec
	Str2
	Str1
	NthStringASCII
	Count
	ToTransient
	AssocBang
	ToPersistent
	Fallback
	IntCast
	Subs
	Gte
	Gt
	Lte
	CursorChar
	CursorNext
	CursorDone
	PackRest
	Apply
	Throw
	TryCatch
	Pop
	MakeFn
	Case
	BitAnd
	BitOr
	BitNot
	BitShiftLeft
	BitShiftRight
)

func OpcodeName(op byte) string {
	switch op {
	case Literal:
		return "irLiteral"
	case LoadSlot:
		return "irLoadSlot"
	case StoreSlot:
		return "irStoreSlot"
	case Add:
		return "irAdd"
	case Sub:
		return "irSub"
	case Mul:
		return "irMul"
	case Rem:
		return "irRem"
	case Div:
		return "irDiv"
	case Inc:
		return "irInc"
	case Dec:
		return "irDec"
	case Lt:
		return "irLt"
	case Gte:
		return "irGte"
	case Gt:
		return "irGt"
	case Lte:
		return "irLte"
	case CursorChar:
		return "irCursorChar"
	case CursorNext:
		return "irCursorNext"
	case CursorDone:
		return "irCursorDone"
	case PackRest:
		return "irPackRest"
	case Apply:
		return "irApply"
	case Throw:
		return "irThrow"
	case TryCatch:
		return "irTryCatch"
	case Pop:
		return "irPop"
	case MakeFn:
		return "irMakeFn"
	case Case:
		return "irCase"
	case BitAnd:
		return "irBitAnd"
	case BitOr:
		return "irBitOr"
	case BitNot:
		return "irBitNot"
	case BitShiftLeft:
		return "irBitShiftLeft"
	case BitShiftRight:
		return "irBitShiftRight"
	case Eq:
		return "irEq"
	case IsZero:
		return "irIsZero"
	case JumpIfNot:
		return "irJumpIfNot"
	case Jump:
		return "irJump"
	case Recur:
		return "irRecur"
	case Return:
		return "irReturn"
	case Get:
		return "irGet"
	case Get3:
		return "irGet3"
	case Assoc:
		return "irAssoc"
	case Nth:
		return "irNth"
	case Conj:
		return "irConj"
	case Sqrt:
		return "irSqrt"
	case CallSlot:
		return "irCallSlot"
	case CallSelf:
		return "irCallSelf"
	case First:
		return "irFirst"
	case BuildVec:
		return "irBuildVec"
	case Str2:
		return "irStr2"
	case Str1:
		return "irStr1"
	case NthStringASCII:
		return "irNthStringASCII"
	case Count:
		return "irCount"
	case ToTransient:
		return "irToTransient"
	case AssocBang:
		return "irAssocBang"
	case ToPersistent:
		return "irToPersistent"
	case Fallback:
		return "irFallback"
	case IntCast:
		return "irIntCast"
	case Subs:
		return "irSubs"
	default:
		return fmt.Sprintf("irUnknown(%d)", op)
	}
}

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
