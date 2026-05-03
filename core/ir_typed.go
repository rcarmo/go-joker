package core

import (
	"math"
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
	irValStringBuilder
	irValStringIntMap
	irValIntVector
	irValNil
)

type irValue struct {
	tag irValueTag
	i   int // int value, or cached rune count for strings/builders
	f   float64
	b   bool // cached ASCII flag for strings/builders
	r   rune
	s   string
	buf []byte
	sm  map[string]int
	iv  []int
	obj Object
}

func irTypedEnabled() bool {
	mode := os.Getenv("JOKER_IR_TYPED")
	return mode != "0" && mode != "off" && mode != "false"
}

func irTypedMapMode() string {
	mode := os.Getenv("JOKER_IR_TYPED_MAP")
	if mode == "" {
		return "auto"
	}
	return mode
}
func irTypedMapEnabled() bool {
	mode := irTypedMapMode()
	return mode != "0" && mode != "off" && mode != "false"
}

func irTypedVecEnabled() bool {
	mode := os.Getenv("JOKER_IR_TYPED_VEC")
	return mode == "1" || mode == "on" || mode == "true" || mode == "force"
}
func irTypedMapForce() bool {
	mode := irTypedMapMode()
	return mode == "1" || mode == "force" || mode == "all"
}

func irTypedEligible(a IRAnalysis) bool {
	if a.NumOps == 0 || a.UsesTransient || a.HasSelfCall {
		return false
	}
	// Call-slot loops: allow if numeric-only or numeric+generic-nth
	if a.HasCallSlot {
		return !a.UsesString && !a.HasMapOps && (!a.UsesCollection || a.HasGenericNth)
	}
	if a.UsesCollection && (a.HasMapOps || !a.HasGenericNth) {
		if irTypedMapEnabled() && a.HasMapOps && a.UsesString {
			return true
		}
		return irTypedVecEnabled() && a.UsesCollection && !a.UsesString && !a.HasMapOps
	}
	// Accept: float/int + generic-nth (e.g. inlined arithmetic helpers with vector nth)
	if a.UsesCollection && a.HasGenericNth && !a.HasMapOps && !a.UsesString {
		return true
	}
	// Accept: pure numeric loops (no strings, no collections, no call-slots)
	if !a.UsesString && !a.UsesCollection && !a.HasCallSlot && !a.HasSelfCall {
		return true
	}
	return a.UsesString || a.SuggestedPath == "typed-ir-string-candidate" || a.SuggestedPath == "typed-ir-generic-string-nth-candidate"
}

func stringToIRValue(s string) irValue {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			return irValue{tag: irValString, s: s, i: utf8.RuneCountInString(s), b: false}
		}
	}
	return irValue{tag: irValString, s: s, i: len(s), b: ascii}
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
		return stringToIRValue(v.S)
	case *ArrayVector:
		if irTypedVecEnabled() {
			iv := make([]int, len(v.arr))
			for i, obj := range v.arr {
				x, ok := obj.(Int)
				if !ok {
					return irValue{tag: irValObject, obj: obj}
				}
				iv[i] = x.I
			}
			return irValue{tag: irValIntVector, iv: iv}
		}
	case *ArrayMap:
		if v.Count() == 0 {
			return irValue{tag: irValStringIntMap, sm: make(map[string]int)}
		}
	case *HashMap:
		if v.Count() == 0 {
			return irValue{tag: irValStringIntMap, sm: make(map[string]int)}
		}
	case Nil:
		return irValue{tag: irValNil}
	default:
		return irValue{tag: irValObject, obj: obj}
	}
	return irValue{tag: irValObject, obj: obj}
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
	case irValStringBuilder:
		return String{S: string(v.buf)}
	case irValStringIntMap:
		res := EmptyArrayMap()
		for k, v := range v.sm {
			res.Add(String{S: k}, Int{I: v})
		}
		return res
	case irValIntVector:
		arr := make([]Object, len(v.iv))
		for i, x := range v.iv {
			arr[i] = Int{I: x}
		}
		return &ArrayVector{arr: arr}
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
	case irValStringBuilder:
		return string(v.buf)
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

func irValueStringKey(v irValue) (string, bool) {
	switch v.tag {
	case irValString:
		return v.s, true
	case irValStringBuilder:
		return string(v.buf), true
	case irValChar:
		return charToStringFast(v.r), true
	default:
		return "", false
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
		case irValStringBuilder:
			return irValue{tag: irValBool, b: string(a.buf) == string(b.buf)}, true
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
		v := objectToIRValue(initSlots[i])
		if v.tag == irValString && i < len(analysis.StringAppendSlots) && (analysis.StringAppendSlots[i] || analysis.StringPrependSlots[i]) {
			buf := make([]byte, len(v.s), len(v.s)+16)
			copy(buf, v.s)
			v = irValue{tag: irValStringBuilder, buf: buf, i: v.i, b: v.b}
		}
		slots[i] = v
	}
	// Pre-fill captured closure values into their assigned slots
	for i, obj := range prog.captureSlots {
		slots[prog.captureSlotIdxs[i]] = objectToIRValue(obj)
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
				stack = append(stack, irValue{tag: irValDouble, f: af - bf})
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
		case irDiv:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
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
			if bf == 0 {
				return nil
			}
			stack = append(stack, irValue{tag: irValDouble, f: af / bf})
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
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValBool, b: a.f < b.f})
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValBool, b: a.f < float64(b.i)})
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValBool, b: float64(a.i) < b.f})
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
			baseSlot := 0
			if target != 0 {
				baseSlot = int(code[pc])<<8 | int(code[pc+1])
				pc += 2
			}
			for i := nargs - 1; i >= 0; i-- {
				slots[baseSlot+i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			pc = target
			stack = stack[:0]
		case irReturn:
			if len(stack) == 0 {
				return NIL
			}
			return stack[len(stack)-1].object()
		case irGet:
			key := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if coll.tag != irValStringIntMap {
				return nil
			}
			k, ok := irValueStringKey(key)
			if !ok {
				return nil
			}
			if v, ok := coll.sm[k]; ok {
				stack = append(stack, irValue{tag: irValInt, i: v})
			} else {
				stack = append(stack, irValue{tag: irValNil})
			}
		case irGet3:
			def := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if coll.tag != irValStringIntMap || def.tag != irValInt {
				return nil
			}
			k, ok := irValueStringKey(key)
			if !ok {
				return nil
			}
			if v, ok := coll.sm[k]; ok {
				stack = append(stack, irValue{tag: irValInt, i: v})
			} else {
				stack = append(stack, def)
			}
		case irAssoc:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if coll.tag == irValStringIntMap && val.tag == irValInt {
				k, ok := irValueStringKey(key)
				if !ok {
					return nil
				}
				if coll.sm == nil {
					coll.sm = make(map[string]int)
				}
				coll.sm[k] = val.i
				stack = append(stack, coll)
			} else if coll.tag == irValIntVector && key.tag == irValInt && val.tag == irValInt {
				if key.i < 0 || key.i > len(coll.iv) {
					return nil
				}
				if key.i == len(coll.iv) {
					coll.iv = append(coll.iv, val.i)
				} else {
					coll.iv[key.i] = val.i
				}
				stack = append(stack, coll)
			} else {
				return nil
			}
		case irNth:
			idx := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if idx.tag != irValInt || idx.i < 0 {
				return nil
			}
			if coll.tag == irValString {
				if coll.b {
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
			} else if coll.tag == irValIntVector {
				if idx.i >= len(coll.iv) {
					return nil
				}
				stack = append(stack, irValue{tag: irValInt, i: coll.iv[idx.i]})
			} else if coll.tag == irValObject {
				switch v := coll.obj.(type) {
				case *ArrayVector:
					if idx.i >= len(v.arr) {
						return nil
					}
					stack = append(stack, objectToIRValue(v.arr[idx.i]))
				case Indexed:
					stack = append(stack, objectToIRValue(v.Nth(idx.i)))
				default:
					return nil
				}
			} else {
				return nil
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
			if a.tag == irValString || a.tag == irValStringBuilder {
				stack = append(stack, a)
			} else {
				stack = append(stack, stringToIRValue(irValueToString(a)))
			}
		case irStr2:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValStringBuilder {
				bs := irValueToString(b)
				a.buf = append(a.buf, bs...)
				if a.b {
					for i := 0; i < len(bs); i++ {
						if bs[i] >= utf8.RuneSelf {
							a.b = false
							break
						}
					}
				}
				if a.b {
					a.i += len(bs)
				} else {
					a.i = irStringRuneCount(string(a.buf))
				}
				stack = append(stack, a)
			} else if b.tag == irValStringBuilder {
				prefix := irValueToString(a)
				if prefix != "" {
					b.buf = append(b.buf, make([]byte, len(prefix))...)
					copy(b.buf[len(prefix):], b.buf[:len(b.buf)-len(prefix)])
					copy(b.buf, prefix)
					if b.b {
						for i := 0; i < len(prefix); i++ {
							if prefix[i] >= utf8.RuneSelf {
								b.b = false
								break
							}
						}
					}
					if b.b {
						b.i += len(prefix)
					} else {
						b.i = irStringRuneCount(string(b.buf))
					}
				}
				stack = append(stack, b)
			} else {
				stack = append(stack, stringToIRValue(irValueToString(a)+irValueToString(b)))
			}
		case irCount:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValString {
				stack = append(stack, irValue{tag: irValInt, i: a.i})
			} else if a.tag == irValStringBuilder {
				stack = append(stack, irValue{tag: irValInt, i: a.i})
			} else if a.tag == irValStringIntMap {
				stack = append(stack, irValue{tag: irValInt, i: len(a.sm)})
			} else if a.tag == irValIntVector {
				stack = append(stack, irValue{tag: irValInt, i: len(a.iv)})
			} else if a.tag == irValObject {
				if c, ok := a.obj.(Counted); ok {
					stack = append(stack, irValue{tag: irValInt, i: c.Count()})
				} else {
					return nil
				}
			} else {
				return nil
			}
		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnObj := initSlots[slotIdx]
			// Fast path: native f64 closure (zero boxing)
			if fn, ok := fnObj.(*Fn); ok {
				if fnProg := irGetFnProg(fn); fnProg != nil && fnProg.nativeHelper != nil {
					var f64buf [4]float64
					f64args := f64buf[:nargs]
					for i := nargs - 1; i >= 0; i-- {
						v := stack[len(stack)-1]
						stack = stack[:len(stack)-1]
						if v.tag == irValDouble {
							f64args[i] = v.f
						} else if v.tag == irValInt {
							f64args[i] = float64(v.i)
						}
					}
					r := fnProg.nativeHelper(f64args)
					stack = append(stack, irValue{tag: irValDouble, f: r})
					continue
				}
			}
			// Slow path: box args and dispatch through WASM/IR/tree-walker
			var argsBuf [4]Object
			var args []Object
			if nargs <= len(argsBuf) {
				args = argsBuf[:nargs]
			} else {
				args = make([]Object, nargs)
			}
			for i := nargs - 1; i >= 0; i-- {
				args[i] = stack[len(stack)-1].object()
				stack = stack[:len(stack)-1]
			}
			var result Object
			if fn, ok := fnObj.(*Fn); ok {
				if wp := wasmGetFn(fn); wp != nil {
					result = wasmExec(wp, args)
				}
				if result == nil {
					if fnProg := irCompileFn(fn); fnProg != nil {
						result = irExec(fnProg, args)
					}
				}
				if result == nil {
					result = fn.Call(args)
				}
			} else if callable, ok := fnObj.(Callable); ok {
				result = callable.Call(args)
			} else {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irSqrt:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: math.Sqrt(a.f)})
			} else if a.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: math.Sqrt(float64(a.i))})
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
