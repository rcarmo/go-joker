package core

import (
	"os"
	"strconv"
	"unicode/utf8"
)

// ir_typed.go — experimental typed IR executor (v2).
//
// This is the first incremental step away from the boxed []Object stack used by
// irExec. It is intentionally small and gated: primitive/string-only loops can
// be executed with tagged values, while unsupported opcodes return nil and let
// the normal IR/tree path handle them.

type irValueTag byte

const (
	irValObject irValueTag = iota
	irValInt
	irValDouble
	irValBool
	irValChar
	irValString
	irValNil
)

type irValue struct {
	tag irValueTag
	i   int
	f   float64
	b   bool
	r   rune
	s   string
	obj Object
}

func irTypedEnabled() bool {
	mode := os.Getenv("JOKER_IR_TYPED")
	return mode != "0" && mode != "off" && mode != "false"
}

func irTypedEligible(a IRAnalysis) bool {
	if a.NumOps == 0 || a.UsesTransient || a.HasCallSlot || a.HasSelfCall || a.HasNestedRecur {
		return false
	}
	if a.UsesCollection && (a.HasMapOps || !a.HasGenericNth) {
		return false
	}
	return a.UsesString || a.SuggestedPath == "typed-ir-string-candidate" || a.SuggestedPath == "typed-ir-generic-string-nth-candidate"
}

func objectToIRValue(obj Object) irValue {
	switch v := obj.(type) {
	case Int:
		return irValue{tag: irValInt, i: v.I}
	case Double:
		return irValue{tag: irValDouble, f: v.D}
	case Boolean:
		return irValue{tag: irValBool, b: v.B}
	case Char:
		return irValue{tag: irValChar, r: v.Ch}
	case String:
		return irValue{tag: irValString, s: v.S}
	case Nil:
		return irValue{tag: irValNil}
	default:
		return irValue{tag: irValObject, obj: obj}
	}
}

func (v irValue) object() Object {
	switch v.tag {
	case irValInt:
		return Int{I: v.i}
	case irValDouble:
		return Double{D: v.f}
	case irValBool:
		return Boolean{B: v.b}
	case irValChar:
		return Char{Ch: v.r}
	case irValString:
		return String{S: v.s}
	case irValNil:
		return NIL
	default:
		if v.obj == nil {
			return NIL
		}
		return v.obj
	}
}

func (v irValue) truthy() bool {
	switch v.tag {
	case irValBool:
		return v.b
	case irValNil:
		return false
	default:
		return true
	}
}

func irValueToString(v irValue) string {
	switch v.tag {
	case irValString:
		return v.s
	case irValChar:
		return charToStringFast(v.r)
	case irValNil:
		return ""
	case irValInt:
		return strconv.Itoa(v.i)
	case irValDouble:
		return strconv.FormatFloat(v.f, 'g', -1, 64)
	case irValBool:
		if v.b {
			return "true"
		}
		return "false"
	default:
		return v.object().ToString(false)
	}
}

func irStringRuneCount(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return utf8.RuneCountInString(s)
		}
	}
	return len(s)
}

func irValueEq(a, b irValue) (irValue, bool) {
	if a.tag == b.tag {
		switch a.tag {
		case irValInt:
			return irValue{tag: irValBool, b: a.i == b.i}, true
		case irValDouble:
			return irValue{tag: irValBool, b: a.f == b.f}, true
		case irValBool:
			return irValue{tag: irValBool, b: a.b == b.b}, true
		case irValChar:
			return irValue{tag: irValBool, b: a.r == b.r}, true
		case irValString:
			return irValue{tag: irValBool, b: a.s == b.s}, true
		case irValNil:
			return irValue{tag: irValBool, b: true}, true
		}
	}
	if a.tag == irValInt && b.tag == irValDouble {
		return irValue{tag: irValBool, b: float64(a.i) == b.f}, true
	}
	if a.tag == irValDouble && b.tag == irValInt {
		return irValue{tag: irValBool, b: a.f == float64(b.i)}, true
	}
	return irValue{tag: irValBool, b: a.object().Equals(b.object())}, true
}

func irExecTyped(prog *IRProgram, initSlots []Object) Object {
	analysis := AnalyzeIRProgram(prog)
	if !irTypedEligible(analysis) {
		return nil
	}
	var slotBuf [16]irValue
	var slots []irValue
	if prog.numSlots <= len(slotBuf) {
		slots = slotBuf[:prog.numSlots]
	} else {
		slots = make([]irValue, prog.numSlots)
	}
	for i := 0; i < len(initSlots) && i < len(slots); i++ {
		slots[i] = objectToIRValue(initSlots[i])
	}

	var stackBuf [32]irValue
	stack := stackBuf[:0]
	code := prog.code
	pc := 0

	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stack = append(stack, objectToIRValue(prog.constants[idx]))
		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if idx < 0 || idx >= len(slots) {
				return nil
			}
			stack = append(stack, slots[idx])
		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if idx < 0 || idx >= len(slots) || len(stack) == 0 {
				return nil
			}
			slots[idx] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case irAdd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i + b.i})
			} else {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					return nil
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					return nil
				}
				stack = append(stack, irValue{tag: irValDouble, f: af + bf})
			}
		case irSub:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i - b.i})
			} else {
				return nil
			}
		case irMul:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i * b.i})
			} else if a.tag == irValDouble || b.tag == irValDouble {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					return nil
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					return nil
				}
				stack = append(stack, irValue{tag: irValDouble, f: af * bf})
			} else {
				return nil
			}
		case irRem:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag != irValInt || b.tag != irValInt || b.i == 0 {
				return nil
			}
			stack = append(stack, irValue{tag: irValInt, i: a.i % b.i})
		case irInc:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValInt {
				return nil
			}
			stack = append(stack, irValue{tag: irValInt, i: a.i + 1})
		case irDec:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValInt {
				return nil
			}
			stack = append(stack, irValue{tag: irValInt, i: a.i - 1})
		case irLt:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValBool, b: a.i < b.i})
			} else {
				return nil
			}
		case irEq:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			v, ok := irValueEq(a, b)
			if !ok {
				return nil
			}
			stack = append(stack, v)
		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			cond := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !cond.truthy() {
				pc = target
			}
		case irJump:
			pc = int(code[pc])<<8 | int(code[pc+1])
		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if target != 0 {
				return nil
			}
			for i := nargs - 1; i >= 0; i-- {
				slots[i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			pc = target
			stack = stack[:0]
		case irReturn:
			if len(stack) == 0 {
				return NIL
			}
			return stack[len(stack)-1].object()
		case irNth:
			idx := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if idx.tag != irValInt || coll.tag != irValString {
				return nil
			}
			if idx.i < 0 {
				return nil
			}
			if isASCIIBytes(coll.s) {
				if idx.i >= len(coll.s) {
					return nil
				}
				stack = append(stack, irValue{tag: irValChar, r: rune(coll.s[idx.i])})
			} else {
				n := 0
				found := false
				for _, r := range coll.s {
					if n == idx.i {
						stack = append(stack, irValue{tag: irValChar, r: r})
						found = true
						break
					}
					n++
				}
				if !found {
					return nil
				}
			}
		case irNthStringASCII:
			idxConst := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			idx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if idx.tag != irValInt {
				return nil
			}
			s := prog.constants[idxConst].(String).S
			if idx.i < 0 || idx.i >= len(s) {
				return nil
			}
			stack = append(stack, irValue{tag: irValChar, r: rune(s[idx.i])})
		case irStr1:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValString {
				stack = append(stack, a)
			} else {
				stack = append(stack, irValue{tag: irValString, s: irValueToString(a)})
			}
		case irStr2:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, irValue{tag: irValString, s: irValueToString(a) + irValueToString(b)})
		case irCount:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValString {
				stack = append(stack, irValue{tag: irValInt, i: irStringRuneCount(a.s)})
			} else {
				return nil
			}
		default:
			return nil
		}
	}
	if len(stack) == 0 {
		return NIL
	}
	return stack[len(stack)-1].object()
}
