package core

import (
	"math"

	coreirx "github.com/rcarmo/go-joker/core/ir"
)

// ---------- Interpreter ----------

func irExec(prog *IRProgram, initSlots []Object) Object {
	defer traceIRProgramCall(prog, len(initSlots))()
	irProfileExecStart()
	defer irProfileMaybeWrite()
	var slots []Object
	if runtimeExec.ProgramNumSlots(prog) <= 16 {
		var buf [16]Object
		slots = buf[:runtimeExec.ProgramNumSlots(prog)]
	} else {
		slots = make([]Object, runtimeExec.ProgramNumSlots(prog))
	}
	copy(slots, initSlots)
	// Pre-fill captured closure values into their assigned slots
	if !runtimeExec.ApplyProgramCaptureSlots(prog, slots) {
		return nil
	}

	// Escape analysis: convert safe local values to transient builders.
	// Only run if there are actually mutable candidate slots.
	if runtimeExec.HasMutableSlotCandidate(slots) {
		escapeInfo := runtimeExec.ProgramEscapeInfo(prog)
		if escapeInfo == nil {
			return nil
		}
		for i, s := range slots {
			slots[i] = runtimeExec.MutableSlotObject(s, escapeInfo, i)
		}
	}

	var stack []Object
	var stackBuf [16]Object
	stack = stackBuf[:0]
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	// Frame stack for irCallSelf — avoids recursive irExec calls
	var frameStack *coreirx.FrameStack[Object]
	defer func() { coreirx.ReleaseFrameStack(frameStack) }()
	var selfTraceStack []func()

	var irProfPrev byte
	var irProfHasPrev bool
	irProfStarted := irProfileStart()
	defer func() { irProfileFinish(irProfPrev, irProfHasPrev, irProfStarted) }()
loop:
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
			stack = append(stack, constant)

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
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Int{I: av.I + bv.I})
					continue
				case Double:
					stack = append(stack, Double{D: float64(av.I) + bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Double{D: av.D + float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Double{D: av.D + bv.D})
					continue
				}
			}
			return nil

		case irSub:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Int{I: av.I - bv.I})
					continue
				case Double:
					stack = append(stack, Double{D: float64(av.I) - bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Double{D: av.D - float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Double{D: av.D - bv.D})
					continue
				}
			}
			return nil

		case irMul:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Int{I: av.I * bv.I})
					continue
				case Double:
					stack = append(stack, Double{D: float64(av.I) * bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Double{D: av.D * float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Double{D: av.D * bv.D})
					continue
				}
			}
			return nil

		case irRem:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if ai, aok := a.(Int); aok {
				if bi, bok := b.(Int); bok {
					if bi.I == 0 {
						return nil
					}
					stack = append(stack, Int{I: ai.I % bi.I})
					continue
				}
			}
			return nil

		case irDiv:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			var av, bv float64
			switch x := a.(type) {
			case Int:
				av = float64(x.I)
			case Double:
				av = x.D
			default:
				return nil
			}
			switch x := b.(type) {
			case Int:
				bv = float64(x.I)
			case Double:
				bv = x.D
			default:
				return nil
			}
			if bv == 0 {
				return nil
			}
			stack = append(stack, Double{D: av / bv})

		case irInc:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case Int:
				stack = append(stack, Int{I: av.I + 1})
			case Double:
				stack = append(stack, Double{D: av.D + 1})
			default:
				return nil
			}

		case irDec:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case Int:
				stack = append(stack, Int{I: av.I - 1})
			case Double:
				stack = append(stack, Double{D: av.D - 1})
			default:
				return nil
			}

		case irLt:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.I < bv.I})
					continue
				case Double:
					stack = append(stack, Boolean{B: float64(av.I) < bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.D < float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Boolean{B: av.D < bv.D})
					continue
				}
			}
			return nil

		case irGte:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.I >= bv.I})
					continue
				case Double:
					stack = append(stack, Boolean{B: float64(av.I) >= bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.D >= float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Boolean{B: av.D >= bv.D})
					continue
				}
			}
			return nil

		case irGt:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.I > bv.I})
					continue
				case Double:
					stack = append(stack, Boolean{B: float64(av.I) > bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.D > float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Boolean{B: av.D > bv.D})
					continue
				}
			}
			return nil

		case irLte:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.I <= bv.I})
					continue
				case Double:
					stack = append(stack, Boolean{B: float64(av.I) <= bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.D <= float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Boolean{B: av.D <= bv.D})
					continue
				}
			}
			return nil

		case irCursorChar:
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.CursorChar(cur)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irCursorNext:
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.CursorNext(cur)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irCursorDone:
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.CursorDone(cur)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irApply:
			argsSeq := stack[len(stack)-1]
			fnObj := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			args, ok := runtimeExec.CallArgs(argsSeq)
			if !ok {
				return nil
			}
			result, ok := runtimeExec.CallObject(fnObj, args)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irThrow:
			v := stack[len(stack)-1]
			runtimeExec.Throw(v)

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
			fn := runtimeExec.MakeFn(fnExpr, slots)
			stack = append(stack, fn)

		case irBitAnd:
			b, a := stack[len(stack)-1].(Int), stack[len(stack)-2].(Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, Int{I: a.I & b.I})
		case irBitOr:
			b, a := stack[len(stack)-1].(Int), stack[len(stack)-2].(Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, Int{I: a.I | b.I})
		case irBitNot:
			a := stack[len(stack)-1].(Int)
			stack = stack[:len(stack)-1]
			stack = append(stack, Int{I: ^a.I})
		case irBitShiftLeft:
			b, a := stack[len(stack)-1].(Int), stack[len(stack)-2].(Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, Int{I: a.I << uint(b.I)})
		case irBitShiftRight:
			b, a := stack[len(stack)-1].(Int), stack[len(stack)-2].(Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, Int{I: a.I >> uint(b.I)})

		case irCase:
			// Jump table: dispatch by integer value
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nCases := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			var testVal int
			switch v := slots[slotIdx].(type) {
			case Int:
				testVal = v.I
			default:
				// Skip table, jump to default
				pc += nCases * 4
				pc = int(code[pc])<<8 | int(code[pc+1])
				continue
			}
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
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case Int:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.I == bv.I})
					continue
				case Double:
					stack = append(stack, Boolean{B: float64(av.I) == bv.D})
					continue
				}
			case Double:
				switch bv := b.(type) {
				case Int:
					stack = append(stack, Boolean{B: av.D == float64(bv.I)})
					continue
				case Double:
					stack = append(stack, Boolean{B: av.D == bv.D})
					continue
				}
			case Char:
				if bv, ok := b.(Char); ok {
					stack = append(stack, Boolean{B: av.Ch == bv.Ch})
					continue
				}
			case String:
				if bv, ok := b.(String); ok {
					stack = append(stack, Boolean{B: av.S == bv.S})
					continue
				}
			}
			stack = append(stack, Boolean{B: runtimeExec.Equal(a, b)})

		case irIsZero:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case Int:
				stack = append(stack, Boolean{B: av.I == 0})
			case Double:
				stack = append(stack, Boolean{B: av.D == 0})
			default:
				return nil
			}

		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			val := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch v := val.(type) {
			case Nil:
				pc = target
			case Boolean:
				if !v.B {
					pc = target
				}
			}

		case irJump:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc = target

		case irRecur:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			targetPC := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// For nested loops, the recur target baseSlot might not be 0
			// We need to figure out which slots to write to.
			// Convention: recur writes to slots starting from the baseSlot
			// of the loop that recur targets. For the top-level loop, baseSlot=0.
			// For nested loops, we determine baseSlot from the target PC.
			// Simple approach: if targetPC==0, write to slots 0..n-1 (backward compat).
			// Otherwise, we need the baseSlot encoded somewhere.
			// For now, recur always writes to the slots at the end of stack in order.
			if targetPC == 0 {
				// Top-level loop recur
				for i := n - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			} else {
				// Nested loop recur — find the base slot from the compile info
				// We store base slot in the bytecode too: nargs(2) + targetPC(2) + baseSlot(2)
				// ... but we didn't emit baseSlot yet. Let's add it.
				// For now, infer: the slots for this loop start at (numSlots - n) or
				// we need to extend the encoding.
				// Quick fix: also encode baseSlot
				baseSlot := int(code[pc])<<8 | int(code[pc+1])
				pc += 2
				for i := n - 1; i >= 0; i-- {
					slots[baseSlot+i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			}
			pc = targetPC
			stack = stack[:0]
			goto loop

		case irReturn:
			if len(stack) == 0 {
				if frameStack != nil && frameStack.Depth() > 0 {
					if len(selfTraceStack) > 0 {
						exit := selfTraceStack[len(selfTraceStack)-1]
						selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
						exit()
					}
					var sl int
					pc, sl = frameStack.Pop(slots)
					stack = stack[:sl]
					stack = append(stack, NIL)
					continue
				}
				return NIL
			}
			result := stack[len(stack)-1]
			if frameStack != nil && frameStack.Depth() > 0 {
				result = runtimeExec.PersistentResult(result)
				if len(selfTraceStack) > 0 {
					exit := selfTraceStack[len(selfTraceStack)-1]
					selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
					exit()
				}
				var sl int
				pc, sl = frameStack.Pop(slots)
				stack = stack[:sl]
				stack = append(stack, result)
				continue
			}
			return runtimeExec.PersistentResult(result)
		case irGet:
			key := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if _, ok := coll.(Gettable); !ok {
				return nil
			}
			stack = append(stack, runtimeExec.Get(coll, key, NIL))

		case irGet3:
			def := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			stack = append(stack, runtimeExec.Get(coll, key, def))

		case irAssoc:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			result, ok := runtimeExec.Assoc(coll, key, val)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irNth:
			idxObj := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			idx, iok := idxObj.(Int)
			if !iok {
				return nil
			}
			result, ok := runtimeExec.Nth(coll, idx.I)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irConj:
			val := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			result, ok := runtimeExec.Conj(coll, val)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irSqrt:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case Double:
				stack = append(stack, Double{D: math.Sqrt(av.D)})
			case Int:
				stack = append(stack, Double{D: math.Sqrt(float64(av.I))})
			default:
				return nil
			}

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnObj := slots[slotIdx]
			// Fast path: native f64 closure (fn-level or loop-level)
			if fnProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if nativeHelper, ok := runtimeExec.NativeHelper(fnProg); ok {
					var f64buf [4]float64
					var f64args []float64
					if nargs <= len(f64buf) {
						f64args = f64buf[:nargs]
					} else {
						f64args = make([]float64, nargs)
					}
					for i := nargs - 1; i >= 0; i-- {
						switch v := stack[len(stack)-1].(type) {
						case Double:
							f64args[i] = v.D
						case Int:
							f64args[i] = float64(v.I)
						default:
							f64args[i] = 0
						}
						stack = stack[:len(stack)-1]
					}
					stack = append(stack, Double{D: nativeHelper(coreirx.Float64(f64args))})
					continue
				}
			}
			// Slow path
			var args []Object
			var argsBuf [4]Object
			if nargs > 0 {
				if nargs <= len(argsBuf) {
					args = argsBuf[:nargs]
				} else {
					args = make([]Object, nargs)
				}
				for i := nargs - 1; i >= 0; i-- {
					args[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			}
			// Try WASM fn dispatch first, then IR, then tree-walker
			if result, ok := runtimeExec.FnWasmExec(fnObj, args); ok {
				stack = append(stack, result)
				continue
			}
			if baseProg, ok := runtimeExec.FnProgram(fnObj); ok {
				// Try IR — typed executor first, skip if previously failed
				if fnProg := runtimeExec.DispatchArityProgram(baseProg, nargs); runtimeExec.CanExecuteIR(fnProg) {
					callArgs, ok := runtimeExec.FnCallSlots(fnObj, fnProg, args)
					if !ok {
						return nil
					}
					if runtimeExec.CanExecuteTypedIR(fnProg) {
						if result := irExecTyped(fnProg, callArgs); result != nil {
							stack = append(stack, result)
							continue
						}
						runtimeExec.MarkTypedExecutionFailed(fnProg)
					}
					if result := irExec(fnProg, callArgs); result != nil {
						stack = append(stack, result)
						continue
					}
				}
			}
			// Fallback to normal Fn.Call
			result, ok := runtimeExec.CallObjectWithSyntheticCallExpr(fnObj, args)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Use frame stack for bounded recursion, fall back to
			// recursive irExec for deep/exponential recursion.
			if frameStack == nil {
				frameStack = coreirx.NewFrameStack[Object](runtimeExec.ProgramNumSlots(prog))
			}
			if frameStack.Depth() < 512 {
				selfTraceStack = append(selfTraceStack, traceIRProgramCall(prog, nargs))
				frameStack.Push(pc, slots, len(stack)-nargs)
				for i := nargs - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
				// Only clear slots beyond nargs if there are captures
				if runtimeExec.ProgramHasCaptureSlots(prog) {
					for i := nargs; i < len(slots); i++ {
						slots[i] = nil
					}
					if !runtimeExec.ApplyProgramCaptureSlots(prog, slots) {
						return nil
					}
				}
				pc = 0
			} else {
				// Deep recursion: fall back to recursive call
				var args []Object
				var argsBuf [4]Object
				if nargs <= len(argsBuf) {
					args = argsBuf[:nargs]
				} else {
					args = make([]Object, nargs)
				}
				for i := nargs - 1; i >= 0; i-- {
					args[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
				result := irExec(prog, args)
				if result == nil {
					return nil
				}
				stack = append(stack, result)
			}

		case irFirst:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.First(a)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			arr := make([]Object, n)
			for i := n - 1; i >= 0; i-- {
				arr[i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, runtimeExec.BuildVector(arr))

		case irStr1:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			stack = append(stack, runtimeExec.Str1(a))

		case irNthStringASCII:
			idxConst := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			idxObj := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			idx, ok := idxObj.(Int)
			if !ok {
				return nil
			}
			result, ok := runtimeExec.NthASCIIStringConst(prog, idxConst, idx.I)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irStr2:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, runtimeExec.Str2(a, b))

		case irCount:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			count, ok := runtimeExec.Count(a)
			if !ok {
				return nil
			}
			stack = append(stack, Int{I: count})

		case irToTransient:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.ToTransient(a)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irAssocBang:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			result, ok := runtimeExec.AssocBang(coll, key, val)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irToPersistent:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.ToPersistent(a)
			if !ok {
				return nil
			}
			stack = append(stack, result)
		case irIntCast:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch v := a.(type) {
			case Char:
				stack = append(stack, Int{I: int(v.Ch)})
			case Int:
				stack = append(stack, v)
			case Double:
				stack = append(stack, Int{I: int(v.D)})
			default:
				return nil
			}

		case irSubs:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if nargs == 3 {
				end := stack[len(stack)-1]
				start := stack[len(stack)-2]
				sObj := stack[len(stack)-3]
				stack = stack[:len(stack)-3]
				s := sObj.(String).S
				si := start.(Int).I
				ei := end.(Int).I
				if stringIsASCII(s) {
					stack = append(stack, String{S: s[si:ei]})
				} else {
					runes := []rune(s)
					stack = append(stack, String{S: string(runes[si:ei])})
				}
			} else {
				start := stack[len(stack)-1]
				sObj := stack[len(stack)-2]
				stack = stack[:len(stack)-2]
				s := sObj.(String).S
				si := start.(Int).I
				if stringIsASCII(s) {
					stack = append(stack, String{S: s[si:]})
				} else {
					runes := []rune(s)
					stack = append(stack, String{S: string(runes[si:])})
				}
			}

		case irFallback:
			return nil

		default:
			return nil
		}
	}
	if len(stack) > 0 {
		return stack[len(stack)-1]
	}
	return NIL
}
