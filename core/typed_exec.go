package core

import (
	"math"
	"unicode/utf8"
	"unsafe"

	coreirx "github.com/rcarmo/go-joker/core/ir"
)

func irExecTyped(prog *IRProgram, initSlots []Object) Object {
	defer traceIRProgramCall(prog, len(initSlots))()
	irProfileExecStart()
	defer irProfileMaybeWrite()
	var analysis IRAnalysis
	if prog.analysis != nil {
		analysis = *prog.analysis
	} else {
		analysis = AnalyzeIRProgram(prog)
		prog.analysis = &analysis
	}
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
	if !(RuntimeExecutionAdapter{}).ApplyTypedCaptureSlots(slots, prog.captureSlotIdxs, prog.captureSlots) {
		return nil
	}

	var stackBuf [32]irValue
	stack := stackBuf[:0]
	code := prog.code
	pc := 0

	// Frame stack for irCallSelf — avoids recursive irExecTyped calls
	var typedFrameStack *coreirx.FrameStack[irValue]
	defer func() { coreirx.ReleaseFrameStack(typedFrameStack) }()
	var selfTraceStack []func()
	var irProfPrev byte
	var irProfHasPrev bool
	irProfStarted := irProfileStart()
	defer func() { irProfileFinish(irProfPrev, irProfHasPrev, irProfStarted) }()

	for pc < len(code) {
		op := code[pc]
		irProfStarted = irProfileOp(irProfPrev, op, irProfHasPrev, irProfStarted)
		irProfPrev, irProfHasPrev = op, true
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
		case irGte:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i >= b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f >= b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f >= float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) >= b.f))
			} else {
				return nil
			}
		case irGt:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i > b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f > b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f > float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) > b.f))
			} else {
				return nil
			}
		case irLte:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i <= b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f <= b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f <= float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) <= b.f))
			} else {
				return nil
			}

		case irCursorChar:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			cur := (*StringCursor)(v.p)
			r := cur.Char()
			if r < 0 {
				stack = append(stack, irValue{tag: irValNil})
			} else {
				stack = append(stack, irMakeChar(r))
			}

		case irCursorNext:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			cur := (*StringCursor)(v.p)
			next := cur.Next()
			stack = append(stack, irValue{tag: irValCursor, p: unsafe.Pointer(next)})

		case irCursorDone:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			cur := (*StringCursor)(v.p)
			stack = append(stack, irMakeBool(cur.Done()))

		case irApply:
			argsVal := stack[len(stack)-1]
			fnVal := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			fnObj := fnVal.object()
			argsObj := argsVal.object()
			callable, ok := fnObj.(Callable)
			if !ok {
				return nil
			}
			args := ToSlice(argsObj.(Seqable).Seq())
			result := callable.Call(args)
			stack = append(stack, objectToIRValue(result))

		case irThrow:
			v := stack[len(stack)-1]
			(RuntimeExecutionAdapter{}).Throw(v.object())

		case irTryCatch:
			pc += 4
			return nil

		case irPop:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

		case irMakeFn:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if prog.fnExprs == nil || idx >= len(prog.fnExprs) {
				return nil
			}
			capturedSlots := make([]Object, len(slots))
			for i, v := range slots {
				capturedSlots[i] = v.object()
			}
			fn := (RuntimeExecutionAdapter{}).MakeFn(prog.fnExprs[idx], capturedSlots)
			stack = append(stack, objectToIRValue(fn))

		case irBitAnd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i & b.i})
			} else {
				return nil
			}
		case irBitOr:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i | b.i})
			} else {
				return nil
			}
		case irBitNot:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: ^a.i})
			} else {
				return nil
			}
		case irBitShiftLeft:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i << uint(b.i)})
			} else {
				return nil
			}
		case irBitShiftRight:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i >> uint(b.i)})
			} else {
				return nil
			}

		case irCase:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nCases := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			v := slots[slotIdx]
			if v.tag != irValInt {
				pc += nCases * 4
				pc = int(code[pc])<<8 | int(code[pc+1])
				continue
			}
			testVal := v.i
			matched := false
			for i := 0; i < nCases; i++ {
				caseVal := int(int16(code[pc])<<8 | int16(code[pc+1]))
				target := int(code[pc+2])<<8 | int(code[pc+3])
				pc += 4
				if testVal == caseVal {
					pc = target
					matched = true
					break
				}
			}
			if !matched {
				pc = int(code[pc])<<8 | int(code[pc+1])
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
				if typedFrameStack != nil && typedFrameStack.Depth() > 0 {
					if len(selfTraceStack) > 0 {
						exit := selfTraceStack[len(selfTraceStack)-1]
						selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
						exit()
					}
					var sl int
					pc, sl = typedFrameStack.Pop(slots)
					stack = stack[:sl]
					stack = append(stack, irValue{tag: irValNil})
					continue
				}
				return NIL
			}
			result := stack[len(stack)-1]
			if typedFrameStack != nil && typedFrameStack.Depth() > 0 {
				if len(selfTraceStack) > 0 {
					exit := selfTraceStack[len(selfTraceStack)-1]
					selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
					exit()
				}
				var sl int
				pc, sl = typedFrameStack.Pop(slots)
				stack = stack[:sl]
				stack = append(stack, result)
				continue
			}
			return result.object()
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
				// General assoc path for Object types (vector of doubles, etc.)
				collObj := coll.object()
				keyObj := key.object()
				valObj := val.object()
				if tv, ok := collObj.(*TransientVector); ok {
					result := tv.AssocInPlace(keyObj, valObj)
					stack = append(stack, objectToIRValue(result))
				} else if assocable, ok := collObj.(Associative); ok {
					result := assocable.Assoc(keyObj, valObj)
					stack = append(stack, objectToIRValue(result))
				} else {
					return nil
				}
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
			// Load fn from typed slots (supports captures beyond initSlots)
			var fnObj Object
			if slotIdx < len(initSlots) {
				fnObj = initSlots[slotIdx]
			} else {
				fnObj = slots[slotIdx].object()
			}
			// Fast path: native f64 closure (zero boxing)
			if fn, ok := fnObj.(*Fn); ok {
				if nativeHelper, ok := runtimeExec.NativeHelper(irGetFnProg(fn)); ok {
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
					r := nativeHelper(coreirx.Float64(f64buf[:nargs]))
					stack = append(stack, irValue{tag: irValDouble, f: r})
					continue
				}
			}
			// Pop args as irValues (no boxing)
			var typedArgBuf [4]irValue
			var typedArgs []irValue
			if nargs <= 4 {
				typedArgs = typedArgBuf[:nargs]
			} else {
				typedArgs = make([]irValue, nargs)
			}
			for i := nargs - 1; i >= 0; i-- {
				typedArgs[i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			var result Object
			if fn, ok := fnObj.(*Fn); ok {
				if fnProg := irGetFnProg(fn); fnProg != nil && runtimeExec.HasNativeHelper(fnProg) {
					// Already handled above
				} else if fnProg := irGetFnProg(fn); runtimeExec.CanExecuteIR(fnProg) {
					// Multi-arity dispatch
					if fnProg.arityPrograms != nil {
						if sub, ok := fnProg.arityPrograms[nargs]; ok {
							fnProg = sub
						} else if fnProg.variadicProg != nil && nargs >= fnProg.variadicMinArgs {
							fnProg = fnProg.variadicProg
						} else {
							fnProg = nil
						}
					}
					if runtimeExec.CanExecuteTypedIR(fnProg) {
						// FAST PATH: typed sub-call without Object boxing
						// Only for pure numeric programs (no collections/strings)
						var subAnalysis IRAnalysis
						if fnProg.analysis != nil {
							subAnalysis = *fnProg.analysis
						} else {
							subAnalysis = AnalyzeIRProgram(fnProg)
							fnProg.analysis = &subAnalysis
						}
						if irTypedEligible(subAnalysis) && !subAnalysis.UsesCollection && !subAnalysis.UsesString && !subAnalysis.HasCallSlot {
							var subBuf [16]irValue
							var subSlots []irValue
							if fnProg.numSlots <= 16 {
								subSlots = subBuf[:fnProg.numSlots]
							} else {
								subSlots = make([]irValue, fnProg.numSlots)
							}
							copy(subSlots[:nargs], typedArgs)
							// Resolve captures
							(RuntimeExecutionAdapter{}).InstallTypedEnvCaptures(fnProg, subSlots, fn.env)
							// Execute inline with typed slots
							subResult := irExecTypedInline(fnProg, subSlots)
							if subResult.tag != 0 || subResult.i != 0 || subResult.f != 0 {
								stack = append(stack, subResult)
								continue
							}
							(RuntimeExecutionAdapter{}).MarkTypedExecutionFailed(fnProg)
						}
					}
					// Fallback: box args
					if fnProg != nil {
						var argsBuf [4]Object
						var args []Object
						if nargs <= 4 {
							args = argsBuf[:nargs]
						} else {
							args = make([]Object, nargs)
						}
						for i, v := range typedArgs {
							args[i] = v.object()
						}
						callArgs := (RuntimeExecutionAdapter{}).PrepareCallSlots(fnProg, args, fn.env)
						if r := irExec(fnProg, callArgs); r != nil {
							result = r
						} else {
							(RuntimeExecutionAdapter{}).MarkBoxedExecutionFailed(fnProg)
						}
					}
				}
				if result == nil {
					var args2 [4]Object
					var a []Object
					if nargs <= 4 {
						a = args2[:nargs]
					} else {
						a = make([]Object, nargs)
					}
					for i, v := range typedArgs {
						a[i] = v.object()
					}
					result = fn.Call(a)
				}
			} else if callable, ok := fnObj.(Callable); ok {
				var args3 [4]Object
				var a []Object
				if nargs <= 4 {
					a = args3[:nargs]
				} else {
					a = make([]Object, nargs)
				}
				for i, v := range typedArgs {
					a[i] = v.object()
				}
				result = callable.Call(a)
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

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if typedFrameStack == nil {
				typedFrameStack = coreirx.NewFrameStack[irValue](prog.numSlots)
			}
			if typedFrameStack.Depth() < 256 {
				// Save current state and restart
				selfTraceStack = append(selfTraceStack, traceIRProgramCall(prog, nargs))
				typedFrameStack.Push(pc, slots, len(stack)-nargs)
				// Pop args directly into slots (no intermediate copy)
				for i := nargs - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
				// Clear only non-capture working slots
				if prog.captureSlotSet != nil {
					for i := nargs; i < len(slots); i++ {
						if !prog.captureSlotSet[i] {
							slots[i] = irValue{}
						}
					}
				} else {
					for i := nargs; i < len(slots); i++ {
						slots[i] = irValue{}
					}
					for i, obj := range prog.captureSlots {
						slots[prog.captureSlotIdxs[i]] = objectToIRValue(obj)
					}
				}
				pc = 0
			} else {
				// Deep recursion: box args and fall back
				args := make([]Object, nargs)
				for i := nargs - 1; i >= 0; i-- {
					args[i] = stack[len(stack)-1].object()
					stack = stack[:len(stack)-1]
				}
				result := irExecTyped(prog, args)
				if result == nil {
					result = irExec(prog, args)
				}
				if result == nil {
					return nil
				}
				stack = append(stack, objectToIRValue(result))
			}

		case irFirst:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValObject {
				switch v := a.obj().(type) {
				case *ArrayVector:
					if len(v.arr) > 0 {
						stack = append(stack, objectToIRValue(v.arr[0]))
					} else {
						stack = append(stack, irValue{tag: irValNil})
					}
				case *TransientVector:
					if len(v.arr) > 0 {
						stack = append(stack, objectToIRValue(v.arr[0]))
					} else {
						stack = append(stack, irValue{tag: irValNil})
					}
				case Seqable:
					seq := v.Seq()
					if seq == nil {
						stack = append(stack, irValue{tag: irValNil})
					} else {
						stack = append(stack, objectToIRValue(seq.First()))
					}
				default:
					return nil
				}
			} else {
				return nil
			}

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			arr := make([]Object, n)
			for i := n - 1; i >= 0; i-- {
				arr[i] = stack[len(stack)-1].object()
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, irMakeObject(&ArrayVector{arr: arr}))

		case irToTransient:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValObject {
				if av, ok := a.obj().(*ArrayVector); ok {
					stack = append(stack, objectToIRValue(ToTransient(av)))
				} else {
					return nil
				}
			} else {
				return nil
			}

		case irAssocBang:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			tv := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if tv.tag == irValObject {
				switch t := tv.obj().(type) {
				case *TransientVector:
					t.AssocInPlace(key.object(), val.object())
					stack = append(stack, objectToIRValue(t))
				case *TransientMap:
					t.AssocInPlace(key.object(), val.object())
					stack = append(stack, objectToIRValue(t))
				default:
					return nil
				}
			} else {
				return nil
			}

		case irToPersistent:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValObject {
				switch t := a.obj().(type) {
				case *TransientVector:
					stack = append(stack, objectToIRValue(t.ToPersistent()))
				case *TransientMap:
					stack = append(stack, objectToIRValue(t.ToPersistent()))
				default:
					return nil
				}
			} else {
				return nil
			}

		case irIntCast:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch a.tag {
			case irValChar:
				stack = append(stack, irValue{tag: irValInt, i: int(a.char())})
			case irValInt:
				stack = append(stack, a)
			case irValDouble:
				stack = append(stack, irValue{tag: irValInt, i: int(a.f)})
			default:
				return nil
			}

		case irSubs:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if nargs == 3 {
				ei := stack[len(stack)-1]
				si := stack[len(stack)-2]
				sv := stack[len(stack)-3]
				stack = stack[:len(stack)-3]
				if sv.tag == irValString && si.tag == irValInt {
					s := sv.str()
					start := si.i
					end := ei.i
					if sv.isASCII() {
						stack = append(stack, irMakeString(s[start:end], end-start, true))
					} else {
						runes := []rune(s)
						stack = append(stack, stringToIRValue(string(runes[start:end])))
					}
				} else {
					return nil
				}
			} else {
				si := stack[len(stack)-1]
				sv := stack[len(stack)-2]
				stack = stack[:len(stack)-2]
				if sv.tag == irValString && si.tag == irValInt {
					s := sv.str()
					start := si.i
					if sv.isASCII() {
						stack = append(stack, irMakeString(s[start:], len(s)-start, true))
					} else {
						runes := []rune(s)
						stack = append(stack, stringToIRValue(string(runes[start:])))
					}
				} else {
					return nil
				}
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

// irExecTypedIV runs the typed executor and returns the result as irValue
// directly, avoiding the Object boxing/unboxing at callSlot boundaries.
// Returns (result, true) on success, (zero, false) on failure.
func irExecTypedIV(prog *IRProgram, initSlots []Object) (irValue, bool) {
	result := irExecTyped(prog, initSlots)
	if result == nil {
		return irValue{}, false
	}
	return objectToIRValue(result), true
}

// irExecTypedInline executes a typed IR program with pre-filled irValue slots.
// Returns the result as irValue directly (no Object boxing).
// Returns zero irValue on failure.
func irExecTypedInline(prog *IRProgram, slots []irValue) irValue {
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
			stack = append(stack, slots[idx])
		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			slots[idx] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case irAdd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: a.f + b.f})
			} else if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i + b.i})
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: a.f + float64(b.i)})
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: float64(a.i) + b.f})
			} else {
				return irValue{}
			}
		case irSub:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: a.f - b.f})
			} else if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i - b.i})
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: a.f - float64(b.i)})
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: float64(a.i) - b.f})
			} else {
				return irValue{}
			}
		case irMul:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: a.f * b.f})
			} else if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i * b.i})
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: a.f * float64(b.i)})
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: float64(a.i) * b.f})
			} else {
				return irValue{}
			}
		case irDiv:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: a.f / b.f})
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: a.f / float64(b.i)})
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: float64(a.i) / b.f})
			} else if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: float64(a.i) / float64(b.i)})
			} else {
				return irValue{}
			}
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
				return irValue{}
			}
		case irEq:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			v, ok := irValueEq(a, b)
			if !ok {
				return irValue{}
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
		case irReturn:
			if len(stack) == 0 {
				return irValue{}
			}
			return stack[len(stack)-1]
		case irRecur:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			targetPC := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if targetPC == 0 {
				for i := n - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			} else {
				baseSlot := int(code[pc])<<8 | int(code[pc+1])
				pc += 2
				for i := n - 1; i >= 0; i-- {
					slots[baseSlot+i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			}
			pc = targetPC
			stack = stack[:0]
		case irInc:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: v.i + 1})
			} else {
				return irValue{}
			}
		case irDec:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: v.i - 1})
			} else {
				return irValue{}
			}
		case irIsZero:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag == irValInt {
				stack = append(stack, irMakeBool(v.i == 0))
			} else {
				return irValue{}
			}
		case irBitAnd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i & b.i})
			} else {
				return irValue{}
			}
		case irBitOr:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i | b.i})
			} else {
				return irValue{}
			}
		case irBitNot:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: ^a.i})
			} else {
				return irValue{}
			}
		case irBitShiftLeft:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i << uint(b.i)})
			} else {
				return irValue{}
			}
		case irBitShiftRight:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i >> uint(b.i)})
			} else {
				return irValue{}
			}
		default:
			return irValue{} // unsupported opcode — bail
		}
	}
	return irValue{}
}
