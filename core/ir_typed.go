package core

import (
	"math"
	"os"
	"strconv"
	"unicode/utf8"
	"unsafe"
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

// irValue is the tagged value for the typed IR executor.
// Layout: 32 bytes for the compact numeric path.
// String/collection data is stored behind an unsafe.Pointer to avoid
// bloating the struct for the common numeric case.
type irValue struct {
	tag irValueTag
	i   int            // int value, bool (0/1), rune, rune count for strings
	f   float64        // double value, or ASCII flag (nonzero = ASCII) for strings
	p   unsafe.Pointer // -> string | []byte | map[string]int | []int | Object
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
			return irMakeString(s, utf8.RuneCountInString(s), false)
		}
	}
	return irMakeString(s, len(s), ascii)
}

func objectToIRValue(obj Object) irValue {
	switch v := obj.(type) {
	case Int:
		return irValue{tag: irValInt, i: v.I}
	case Double:
		return irValue{tag: irValDouble, f: v.D}
	case Boolean:
		return irMakeBool(v.B)
	case Char:
		return irMakeChar(v.Ch)
	case String:
		return stringToIRValue(v.S)
	case *ArrayVector:
		if irTypedVecEnabled() {
			iv := make([]int, len(v.arr))
			for i, obj := range v.arr {
				x, ok := obj.(Int)
				if !ok {
					return irMakeObject(obj)
				}
				iv[i] = x.I
			}
			return irMakeIntVector(iv)
		}
	case *ArrayMap:
		if v.Count() == 0 {
			return irMakeStringIntMap(make(map[string]int))
		}
	case *HashMap:
		if v.Count() == 0 {
			return irMakeStringIntMap(make(map[string]int))
		}
	case Nil:
		return irValue{tag: irValNil}
	default:
		return irMakeObject(obj)
	}
	return irMakeObject(obj)
}

func (v irValue) object() Object {
	switch v.tag {
	case irValInt:
		return Int{I: v.i}
	case irValDouble:
		return Double{D: v.f}
	case irValBool:
		return Boolean{B: v.boolean()}
	case irValChar:
		return Char{Ch: v.char()}
	case irValString:
		return String{S: v.str()}
	case irValStringBuilder:
		return String{S: string(v.bytes())}
	case irValStringIntMap:
		res := EmptyArrayMap()
		for k, v := range v.stringIntMap() {
			res.Add(String{S: k}, Int{I: v})
		}
		return res
	case irValIntVector:
		arr := make([]Object, len(v.intVec()))
		for i, x := range v.intVec() {
			arr[i] = Int{I: x}
		}
		return &ArrayVector{arr: arr}
	case irValNil:
		return NIL
	default:
		if v.obj() == nil {
			return NIL
		}
		return v.obj()
	}
}

func (v irValue) truthy() bool {
	switch v.tag {
	case irValBool:
		return v.boolean()
	case irValNil:
		return false
	default:
		return true
	}
}

func irValueToString(v irValue) string {
	switch v.tag {
	case irValString:
		return v.str()
	case irValStringBuilder:
		return string(v.bytes())
	case irValChar:
		return charToStringFast(v.char())
	case irValNil:
		return ""
	case irValInt:
		return strconv.Itoa(v.i)
	case irValDouble:
		return strconv.FormatFloat(v.f, 'g', -1, 64)
	case irValBool:
		if v.boolean() {
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
		return v.str(), true
	case irValStringBuilder:
		return string(v.bytes()), true
	case irValChar:
		return charToStringFast(v.char()), true
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
			return irMakeBool(a.i == b.i), true
		case irValDouble:
			return irMakeBool(a.f == b.f), true
		case irValBool:
			return irMakeBool(a.boolean() == b.boolean()), true
		case irValChar:
			return irMakeBool(a.char() == b.char()), true
		case irValString:
			return irMakeBool(a.str() == b.str()), true
		case irValStringBuilder:
			return irMakeBool(string(a.bytes()) == string(b.bytes())), true
		case irValNil:
			return irMakeBool(true), true
		}
	}
	if a.tag == irValInt && b.tag == irValDouble {
		return irMakeBool(float64(a.i) == b.f), true
	}
	if a.tag == irValDouble && b.tag == irValInt {
		return irMakeBool(a.f == float64(b.i)), true
	}
	return irMakeBool(a.object().Equals(b.object())), true
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
			buf := make([]byte, len(v.str()), len(v.str())+16)
			copy(buf, v.str())
			v = irMakeStringBuilder(buf, v.i, v.boolean())
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
				stack = append(stack, irMakeBool(a.i < b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f < b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f < float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) < b.f))
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
			if v, ok := coll.stringIntMap()[k]; ok {
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
			if v, ok := coll.stringIntMap()[k]; ok {
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
				if coll.stringIntMap() == nil {
					coll.setStringIntMap(make(map[string]int))
				}
				coll.stringIntMap()[k] = val.i
				stack = append(stack, coll)
			} else if coll.tag == irValIntVector && key.tag == irValInt && val.tag == irValInt {
				if key.i < 0 || key.i > len(coll.intVec()) {
					return nil
				}
				iv := coll.intVec()
				if key.i == len(iv) {
					iv = append(iv, val.i)
				} else {
					iv[key.i] = val.i
				}
				coll.setIntVec(iv)
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
				if coll.boolean() {
					if idx.i >= len(coll.str()) {
						return nil
					}
					stack = append(stack, irMakeChar(rune(coll.str()[idx.i])))
				} else {
					n := 0
					found := false
					for _, r := range coll.str() {
						if n == idx.i {
							stack = append(stack, irMakeChar(r))
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
				if idx.i >= len(coll.intVec()) {
					return nil
				}
				stack = append(stack, irValue{tag: irValInt, i: coll.intVec()[idx.i]})
			} else if coll.tag == irValObject {
				switch v := coll.obj().(type) {
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
			stack = append(stack, irMakeChar(rune(s[idx.i])))
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
				abuf := append(a.bytes(), bs...)
				ascii := a.isASCII()
				if ascii {
					for i := 0; i < len(bs); i++ {
						if bs[i] >= utf8.RuneSelf {
							ascii = false
							break
						}
					}
				}
				rc := a.i
				if ascii {
					rc += len(bs)
				} else {
					rc = irStringRuneCount(string(abuf))
				}
				stack = append(stack, irMakeStringBuilder(abuf, rc, ascii))
			} else if b.tag == irValStringBuilder {
				prefix := irValueToString(a)
				if prefix != "" {
					bbuf := b.bytes()
					newBuf := make([]byte, len(prefix)+len(bbuf))
					copy(newBuf, prefix)
					copy(newBuf[len(prefix):], bbuf)
					ascii := b.isASCII()
					if ascii {
						for i := 0; i < len(prefix); i++ {
							if prefix[i] >= utf8.RuneSelf {
								ascii = false
								break
							}
						}
					}
					rc := b.i
					if ascii {
						rc += len(prefix)
					} else {
						rc = irStringRuneCount(string(newBuf))
					}
					b = irMakeStringBuilder(newBuf, rc, ascii)
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
				stack = append(stack, irValue{tag: irValInt, i: len(a.stringIntMap())})
			} else if a.tag == irValIntVector {
				stack = append(stack, irValue{tag: irValInt, i: len(a.intVec())})
			} else if a.tag == irValObject {
				if c, ok := a.obj().(Counted); ok {
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
					// Call native helper with stack-allocated args
					f64buf := [4]float64{}
					for i := nargs - 1; i >= 0; i-- {
						v := stack[len(stack)-1]
						stack = stack[:len(stack)-1]
						if v.tag == irValDouble {
							f64buf[i] = v.f
						} else if v.tag == irValInt {
							f64buf[i] = float64(v.i)
						}
					}
					r := fnProg.nativeHelper(noescape64(f64buf[:nargs]))
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

		case irConj:
			val := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if coll.tag == irValObject {
				if c, ok := coll.obj().(Conjable); ok {
					result := c.Conj(val.object())
					stack = append(stack, objectToIRValue(result))
				} else {
					return nil
				}
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
