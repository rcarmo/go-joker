package core

import (
	"math"
)

// ---------- Interpreter ----------

func irExec(prog *IRProgram, initSlots []Object) Object {
	var slots []Object
	if prog.numSlots <= 16 {
		var buf [16]Object
		slots = buf[:prog.numSlots]
	} else {
		slots = make([]Object, prog.numSlots)
	}
	copy(slots, initSlots)
	// Pre-fill captured closure values into their assigned slots
	for i, obj := range prog.captureSlots {
		slots[prog.captureSlotIdxs[i]] = obj
	}

	// Escape analysis: convert safe local values to transient builders.
	// Only run if there are actually mutable candidate slots.
	hasMutableCandidate := false
	for _, s := range slots {
		switch s.(type) {
		case *ArrayVector, *ArrayMap, *HashMap, String:
			hasMutableCandidate = true
		}
		if hasMutableCandidate {
			break
		}
	}
	if hasMutableCandidate {
		if prog.escapeInfo == nil {
			prog.escapeInfo = analyzeEscapes(prog)
		}
		for i, s := range slots {
			if !prog.escapeInfo.SafeMutableSlots[i] {
				continue
			}
			switch v := s.(type) {
			case *ArrayVector:
				slots[i] = ToTransient(v)
			case *ArrayMap:
				slots[i] = MapToTransient(v)
			case *HashMap:
				slots[i] = MapToTransient(v)
			case String:
				if !irStringBuilderDisabled() {
					if irStringBuilderForce() && (prog.escapeInfo.StringBuilderSlots[i] || prog.escapeInfo.StringPrependSlots[i]) {
						slots[i] = ToTransientString(v)
					} else if !irStringBuilderForce() && prog.escapeInfo.StringPrependSlots[i] {
						slots[i] = ToTransientString(v)
					}
				}
			}
		}
	}

	var stack []Object
	var stackBuf [16]Object
	stack = stackBuf[:0]
	code := prog.code
	pc := 0

	// Frame stack for irCallSelf — avoids recursive irExec calls
	var frameStack *irFrameStack

loop:
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stack = append(stack, prog.constants[idx])

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
			cur, ok := stack[len(stack)-1].(*StringCursor)
			stack = stack[:len(stack)-1]
			if !ok {
				return nil
			}
			r := cur.Char()
			if r < 0 {
				stack = append(stack, NIL)
			} else {
				stack = append(stack, Char{Ch: r})
			}

		case irCursorNext:
			cur, ok := stack[len(stack)-1].(*StringCursor)
			stack = stack[:len(stack)-1]
			if !ok {
				return nil
			}
			stack = append(stack, cur.Next())

		case irCursorDone:
			cur, ok := stack[len(stack)-1].(*StringCursor)
			stack = stack[:len(stack)-1]
			if !ok {
				return nil
			}
			stack = append(stack, Boolean{B: cur.Done()})

		case irApply:
			argsSeq := stack[len(stack)-1]
			fnObj := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			callable, ok := fnObj.(Callable)
			if !ok {
				return nil
			}
			args := ToSlice(argsSeq.(Seqable).Seq())
			stack = append(stack, callable.Call(args))

		case irThrow:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			panic(RT.NewError(v.ToString(false)))

		case irTryCatch:
			pc += 4
			return nil

		case irPop:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
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
			stack = append(stack, Boolean{B: a.Equals(b)})

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
				if frameStack != nil && frameStack.depth > 0 {
					var sl int
					pc, sl = frameStack.pop(slots)
					stack = stack[:sl]
					stack = append(stack, NIL)
					continue
				}
				return NIL
			}
			result := stack[len(stack)-1]
			if frameStack != nil && frameStack.depth > 0 {
				switch v := result.(type) {
				case *TransientVector:
					result = v.ToPersistent()
				case *TransientMap:
					result = v.ToPersistent()
				case *TransientString:
					result = v.ToPersistent()
				}
				var sl int
				pc, sl = frameStack.pop(slots)
				stack = stack[:sl]
				stack = append(stack, result)
				continue
			}
			switch v := result.(type) {
			case *TransientVector:
				return v.ToPersistent()
			case *TransientMap:
				return v.ToPersistent()
			case *TransientString:
				return v.ToPersistent()
			}
			return result
		case irGet:
			key := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if g, ok := coll.(Gettable); ok {
				ok, v := g.Get(key)
				if ok {
					stack = append(stack, v)
				} else {
					stack = append(stack, NIL)
				}
			} else {
				return nil
			}

		case irGet3:
			def := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if g, ok := coll.(Gettable); ok {
				ok, v := g.Get(key)
				if ok {
					stack = append(stack, v)
				} else {
					stack = append(stack, def)
				}
			} else {
				stack = append(stack, def)
			}

		case irAssoc:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			// Fast path: transient in-place mutation
			if tv, ok := coll.(*TransientVector); ok {
				stack = append(stack, tv.AssocInPlace(key, val))
			} else if tm, ok := coll.(*TransientMap); ok {
				stack = append(stack, tm.AssocInPlace(key, val))
			} else if a, ok := coll.(Associative); ok {
				stack = append(stack, a.Assoc(key, val))
			} else {
				return nil
			}

		case irNth:
			idxObj := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			idx, iok := idxObj.(Int)
			if !iok {
				return nil
			}
			switch c := coll.(type) {
			case *ArrayVector:
				if idx.I >= 0 && idx.I < len(c.arr) {
					stack = append(stack, c.arr[idx.I])
				} else {
					return nil
				}
			case *TransientVector:
				if idx.I >= 0 && idx.I < len(c.arr) {
					stack = append(stack, c.arr[idx.I])
				} else {
					return nil
				}
			case String:
				stack = append(stack, stringNthFast(c.S, idx.I))
			case Indexed:
				stack = append(stack, c.Nth(idx.I))
			default:
				return nil
			}

		case irConj:
			val := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch c := coll.(type) {
			case *TransientVector:
				stack = append(stack, c.ConjInPlace(val))
			case Conjable:
				stack = append(stack, c.Conj(val))
			default:
				return nil
			}

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
			if fn, ok := fnObj.(*Fn); ok {
				if fnProg := irGetFnProg(fn); fnProg != nil && fnProg.nativeHelper != nil {
					var f64buf [4]float64
					f64args := f64buf[:nargs]
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
					stack = append(stack, Double{D: fnProg.nativeHelper(noescape64(f64args))})
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
			if fn, ok := fnObj.(*Fn); ok {
				// Try WASM native
				if wp := wasmGetFn(fn); wp != nil {
					if result := wasmExec(wp, args); result != nil {
						stack = append(stack, result)
						continue
					}
				}
				// Try IR — typed executor first, skip if previously failed
				if fnProg := irCompileFn(fn); fnProg != nil {
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
					if fnProg != nil {
					callArgs := args
					if len(fnProg.captureKeys) > 0 {
						full := make([]Object, fnProg.numSlots)
						copy(full, args)
						for ci, ck := range fnProg.captureKeys {
							e := fn.env
							for e != nil {
								if ck.index < len(e.bindings) {
									full[fnProg.captureSlotIdxs[ci]] = e.bindings[ck.index]
									break
								}
								e = e.parent
							}
						}
						callArgs = full
					}
					if !fnProg.typedFailed {
						if result := irExecTyped(fnProg, callArgs); result != nil {
							stack = append(stack, result)
							continue
						}
						fnProg.typedFailed = true
					}
					if result := irExec(fnProg, callArgs); result != nil {
						stack = append(stack, result)
						continue
					}
					} // end if fnProg != nil
				}
			}
			// Fallback to normal Fn.Call
			prevExpr := RT.currentExpr
			RT.currentExpr = &CallExpr{}
			switch fn := fnObj.(type) {
			case Callable:
				stack = append(stack, fn.Call(args))
			default:
				RT.currentExpr = prevExpr
				return nil
			}
			RT.currentExpr = prevExpr

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Use frame stack for bounded recursion, fall back to
			// recursive irExec for deep/exponential recursion.
			if frameStack == nil {
				frameStack = newIRFrameStack(prog.numSlots)
			}
			if frameStack.depth < 256 {
				frameStack.push(pc, slots, len(stack)-nargs)
				for i := nargs - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
				for i := nargs; i < len(slots); i++ {
					slots[i] = nil
				}
				for i, obj := range prog.captureSlots {
					slots[prog.captureSlotIdxs[i]] = obj
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
			switch v := a.(type) {
			case *ArrayVector:
				if len(v.arr) > 0 {
					stack = append(stack, v.arr[0])
				} else {
					stack = append(stack, NIL)
				}
			case *TransientVector:
				if len(v.arr) > 0 {
					stack = append(stack, v.arr[0])
				} else {
					stack = append(stack, NIL)
				}
			case Seqable:
				s := v.Seq()
				if s.IsEmpty() {
					stack = append(stack, NIL)
				} else {
					stack = append(stack, s.First())
				}
			default:
				return nil
			}

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			arr := make([]Object, n)
			for i := n - 1; i >= 0; i-- {
				arr[i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, &ArrayVector{arr: arr})

		case irStr1:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case Nil:
				stack = append(stack, String{S: ""})
			case String:
				stack = append(stack, av)
			case Char:
				stack = append(stack, charToStringObjectFast(av.Ch))
			default:
				stack = append(stack, String{S: a.ToString(false)})
			}

		case irNthStringASCII:
			idxConst := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			idxObj := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			idx, ok := idxObj.(Int)
			if !ok {
				return nil
			}
			s := prog.constants[idxConst].(String).S
			if idx.I < 0 || idx.I >= len(s) {
				return nil
			}
			stack = append(stack, Char{Ch: rune(s[idx.I])})

		case irStr2:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case *TransientString:
				switch bv := b.(type) {
				case Char:
					stack = append(stack, av.AppendChar(bv.Ch))
				case String:
					stack = append(stack, av.AppendString(bv.S))
				default:
					stack = append(stack, av.AppendString(b.ToString(false)))
				}
			case String:
				switch bv := b.(type) {
				case Char:
					stack = append(stack, String{S: av.S + charToStringFast(bv.Ch)})
				case String:
					stack = append(stack, String{S: av.S + bv.S})
				case *TransientString:
					stack = append(stack, bv.PrependString(av.S))
				default:
					stack = append(stack, String{S: av.S + b.ToString(false)})
				}
			case Char:
				if bv, ok := b.(*TransientString); ok {
					stack = append(stack, bv.PrependChar(av.Ch))
				} else {
					stack = append(stack, String{S: charToStringFast(av.Ch) + b.ToString(false)})
				}
			default:
				stack = append(stack, String{S: a.ToString(false) + b.ToString(false)})
			}

		case irCount:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch v := a.(type) {
			case *TransientString:
				stack = append(stack, Int{I: v.Count()})
			case Counted:
				stack = append(stack, Int{I: v.Count()})
			case String:
				stack = append(stack, Int{I: len(v.S)})
			default:
				return nil
			}

		case irToTransient:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if av, ok := a.(*ArrayVector); ok {
				stack = append(stack, ToTransient(av))
			} else {
				return nil
			}

		case irAssocBang:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if tv, ok := coll.(*TransientVector); ok {
				stack = append(stack, tv.AssocInPlace(key, val))
			} else {
				return nil
			}

		case irToPersistent:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if tv, ok := a.(*TransientVector); ok {
				stack = append(stack, tv.ToPersistent())
			} else {
				return nil
			}
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
