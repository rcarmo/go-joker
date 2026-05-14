package core

import (
	"math"
	"unicode/utf8"

	coreirx "github.com/rcarmo/go-joker/core/ir"
)

func irExecTyped(prog *IRProgram, initSlots []Object) Object {
	defer traceIRProgramCall(prog, len(initSlots))()
	irProfileExecStart()
	defer irProfileMaybeWrite()
	analysis := runtimeExec.ProgramAnalysis(prog)
	if !irTypedEligible(analysis) {
		return nil
	}
	var slotBuf [16]irValue
	var slots []irValue
	numSlots := runtimeExec.ProgramNumSlots(prog)
	if numSlots <= len(slotBuf) {
		slots = slotBuf[:numSlots]
	} else {
		slots = make([]irValue, numSlots)
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
	if !runtimeExec.ApplyProgramTypedCaptureSlots(prog, slots) {
		return nil
	}

	var stackBuf [32]irValue
	stack := stackBuf[:0]
	code := runtimeExec.ProgramCode(prog)
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
			constant, ok := runtimeExec.ProgramConstant(prog, idx)
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(constant))
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
			result, ok := runtimeExec.CursorChar(v.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irCursorNext:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			result, ok := runtimeExec.CursorNext(v.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irCursorDone:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			result, ok := runtimeExec.CursorDone(v.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irApply:
			argsVal := stack[len(stack)-1]
			fnVal := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			fnObj := fnVal.object()
			argsObj := argsVal.object()
			args, ok := runtimeExec.CallArgs(argsObj)
			if !ok {
				return nil
			}
			result, ok := runtimeExec.CallObject(fnObj, args)
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irThrow:
			v := stack[len(stack)-1]
			runtimeExec.Throw(v.object())

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
			fnExpr, ok := runtimeExec.ProgramFnExpr(prog, idx)
			if !ok {
				return nil
			}
			capturedSlots := make([]Object, len(slots))
			for i, v := range slots {
				capturedSlots[i] = v.object()
			}
			fn := runtimeExec.MakeFn(fnExpr, capturedSlots)
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
			constant, ok := runtimeExec.ProgramConstant(prog, idxConst)
			if !ok {
				return nil
			}
			s := constant.(String).S
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
			if a.tag == irValString || a.tag == irValStringBuilder {
				stack = append(stack, irValue{tag: irValInt, i: a.i})
			} else if a.tag == irValStringIntMap {
				stack = append(stack, irValue{tag: irValInt, i: len(a.stringIntMap())})
			} else if a.tag == irValIntVector {
				stack = append(stack, irValue{tag: irValInt, i: len(a.intVec())})
			} else if a.tag == irValObject {
				count, ok := runtimeExec.Count(a.obj())
				if !ok {
					return nil
				}
				stack = append(stack, irValue{tag: irValInt, i: count})
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
			if fnProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if nativeHelper, ok := runtimeExec.NativeHelper(fnProg); ok {
					// Call native helper with stack-allocated args for common arities.
					var f64buf [4]float64
					var f64args []float64
					if nargs <= len(f64buf) {
						f64args = f64buf[:nargs]
					} else {
						f64args = make([]float64, nargs)
					}
					for i := nargs - 1; i >= 0; i-- {
						v := stack[len(stack)-1]
						stack = stack[:len(stack)-1]
						if v.tag == irValDouble {
							f64args[i] = v.f
						} else if v.tag == irValInt {
							f64args[i] = float64(v.i)
						}
					}
					r := nativeHelper(coreirx.Float64(f64args))
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
			if baseProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if baseProg != nil && runtimeExec.HasNativeHelper(baseProg) {
					// Already handled above
				} else if fnProg := runtimeExec.DispatchArityProgram(baseProg, nargs); runtimeExec.CanExecuteIR(fnProg) {
					routedToIR := false
					if runtimeExec.CanExecuteTypedIR(fnProg) {
						// FAST PATH: typed sub-call without Object boxing
						// Only for pure numeric programs (no collections/strings)
						subAnalysis := runtimeExec.ProgramAnalysis(fnProg)
						if irTypedEligible(subAnalysis) && !subAnalysis.UsesCollection && !subAnalysis.UsesString && !subAnalysis.HasCallSlot {
							var subBuf [16]irValue
							var subSlots []irValue
							numSlots := runtimeExec.ProgramNumSlots(fnProg)
							if numSlots < nargs {
								return nil
							}
							if numSlots <= 16 {
								subSlots = subBuf[:numSlots]
							} else {
								subSlots = make([]irValue, numSlots)
							}
							copy(subSlots[:nargs], typedArgs)
							// Resolve captures
							if !runtimeExec.InstallFnTypedEnvCaptures(fnObj, fnProg, subSlots) {
								return nil
							}
							// Execute inline with typed slots
							subResult := irExecTypedInline(fnProg, subSlots)
							if subResult.tag != 0 || subResult.i != 0 || subResult.f != 0 {
								stack = append(stack, subResult)
								continue
							}
							runtimeExec.MarkTypedExecutionFailed(fnProg)
						}
					}
					// Fallback: box args
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
					callArgs, ok := runtimeExec.FnCallSlots(fnObj, fnProg, args)
					if !ok {
						return nil
					}
					if r := irExec(fnProg, callArgs); r != nil {
						result = r
						routedToIR = true
					} else {
						runtimeExec.MarkBoxedExecutionFailed(fnProg)
					}
					if !routedToIR && result == nil {
						return nil
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
					var ok bool
					result, ok = runtimeExec.CallObject(fnObj, a)
					if !ok {
						return nil
					}
				}
			} else {
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
				var ok bool
				result, ok = runtimeExec.CallObject(fnObj, a)
				if !ok {
					return nil
				}
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
			if coll.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.Conj(coll.obj(), val.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if typedFrameStack == nil {
				typedFrameStack = coreirx.NewFrameStack[irValue](runtimeExec.ProgramNumSlots(prog))
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
				if !runtimeExec.ClearTypedNonCaptureSlots(prog, slots, nargs) {
					return nil
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
			if a.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.First(a.obj())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			arr := make([]Object, n)
			for i := n - 1; i >= 0; i-- {
				arr[i] = stack[len(stack)-1].object()
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, irMakeObject(runtimeExec.BuildVector(arr)))

		case irToTransient:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.ToTransient(a.obj())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irAssocBang:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			tv := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if tv.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.AssocBang(tv.obj(), key.object(), val.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irToPersistent:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.ToPersistent(a.obj())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

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
