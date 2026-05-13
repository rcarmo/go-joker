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
