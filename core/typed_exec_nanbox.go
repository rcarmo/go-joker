package core

import (
	"math"

	coreirx "github.com/rcarmo/go-joker/core/ir"
)

// ir_exec_typed_nb.go — NaN-boxed typed IR executor.
//
// Uses []uint64 stack (8 bytes per entry) instead of []irValue (32 bytes).
// Numeric operations are pure bit manipulation — zero allocation, zero copy.
// Object operations convert at the boundary via the existing nb* helpers.
//
// This is the typed executor's hot path for numeric loops.
// Falls back to nil (letting irExecTyped handle it) for unsupported patterns.

func irExecTypedNB(prog *IRProgram, initSlots []Object) Object {
	analysis := AnalyzeIRProgram(prog)
	// Only handle numeric-dominant programs without complex collection ops
	if !irTypedEligible(analysis) {
		return nil
	}
	// Only handle pure numeric programs — no collections, no self-calls,
	// no strings, no fn calls (which allocate []Object args).
	if analysis.HasSelfCall || analysis.UsesString || analysis.UsesTransient ||
		analysis.UsesCollection || analysis.HasCallSlot {
		return nil
	}

	numSlots := prog.numSlots
	var slotBuf [16]uint64
	var slots []uint64
	if numSlots <= len(slotBuf) {
		slots = slotBuf[:numSlots]
	} else {
		slots = make([]uint64, numSlots)
	}

	// Object side-table for non-numeric values
	var objTable []Object

	// Convert init slots
	for i := 0; i < numSlots && i < len(initSlots); i++ {
		slots[i] = nbFromObject(initSlots[i], &objTable)
	}
	// Pre-fill captures
	for i, obj := range prog.captureSlots {
		slots[prog.captureSlotIdxs[i]] = nbFromObject(obj, &objTable)
	}

	// Pre-convert constants
	consts := make([]uint64, len(prog.constants))
	for i, c := range prog.constants {
		consts[i] = nbFromObject(c, &objTable)
	}

	var stackBuf [32]uint64
	sp := 0
	code := prog.code
	pc := 0

	for pc < len(code) {
		op := code[pc]
		pc++

		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stackBuf[sp] = consts[idx]
			sp++

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stackBuf[sp] = slots[idx]
			sp++

		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			sp--
			slots[idx] = stackBuf[sp]

		case irAdd:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(a) + coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(a) + coreirx.ToFloat(b))
			}

		case irSub:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(a) - coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(a) - coreirx.ToFloat(b))
			}

		case irMul:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(a) * coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(a) * coreirx.ToFloat(b))
			}

		case irDiv:
			sp--
			stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(stackBuf[sp-1]) / coreirx.ToFloat(stackBuf[sp]))

		case irRem:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				bv := coreirx.ToInt(b)
				if bv == 0 {
					return nil
				}
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(a) % bv)
			} else {
				return nil
			}

		case irSqrt:
			stackBuf[sp-1] = coreirx.BoxDouble(math.Sqrt(coreirx.ToFloat(stackBuf[sp-1])))

		case irInc:
			v := stackBuf[sp-1]
			if coreirx.IsInt(v) {
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(v) + 1)
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(v) + 1)
			}

		case irDec:
			v := stackBuf[sp-1]
			if coreirx.IsInt(v) {
				stackBuf[sp-1] = coreirx.BoxInt(coreirx.ToInt(v) - 1)
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(v) - 1)
			}

		case irLt:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) < coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) < coreirx.ToFloat(b))
			}

		case irGte:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) >= coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) >= coreirx.ToFloat(b))
			}

		case irGt:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) > coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) > coreirx.ToFloat(b))
			}

		case irLte:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) <= coreirx.ToInt(b))
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) <= coreirx.ToFloat(b))
			}

		case irEq:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if a == b {
				stackBuf[sp-1] = coreirx.BoxBool(true)
			} else if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(false)
			} else if coreirx.IsDouble(a) || coreirx.IsDouble(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) == coreirx.ToFloat(b))
			} else {
				oa := nbToObject(a, objTable)
				ob := nbToObject(b, objTable)
				stackBuf[sp-1] = coreirx.BoxBool(oa.Equals(ob))
			}

		case irIsZero:
			v := stackBuf[sp-1]
			if coreirx.IsInt(v) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(v) == 0)
			} else if coreirx.IsDouble(v) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToDouble(v) == 0)
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(false)
			}

		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			sp--
			if !coreirx.Truthy(stackBuf[sp]) {
				pc = target
			}

		case irJump:
			pc = int(code[pc])<<8 | int(code[pc+1])

		case irReturn:
			if sp == 0 {
				return NIL
			}
			sp--
			return nbToObject(stackBuf[sp], objTable)

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if target != 0 {
				baseSlot := int(code[pc])<<8 | int(code[pc+1])
				pc += 2
				for i := nargs - 1; i >= 0; i-- {
					sp--
					slots[baseSlot+i] = stackBuf[sp]
				}
			} else {
				for i := nargs - 1; i >= 0; i-- {
					sp--
					slots[i] = stackBuf[sp]
				}
			}
			sp = 0
			pc = target

		// Collection ops: convert at boundary
		case irNth:
			sp -= 2
			coll := nbToObject(stackBuf[sp], objTable)
			idxV := stackBuf[sp+1]
			var idx int
			if coreirx.IsInt(idxV) {
				idx = coreirx.ToInt(idxV)
			} else {
				idx = int(coreirx.ToFloat(idxV))
			}
			switch v := coll.(type) {
			case *ArrayVector:
				if idx >= 0 && idx < len(v.arr) {
					stackBuf[sp] = nbFromObject(v.arr[idx], &objTable)
					sp++
				} else {
					return nil
				}
			case *TransientVector:
				if idx >= 0 && idx < len(v.arr) {
					stackBuf[sp] = nbFromObject(v.arr[idx], &objTable)
					sp++
				} else {
					return nil
				}
			default:
				return nil
			}

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnObj := nbToObject(slots[slotIdx], objTable)
			// Native f64 fast path
			if fn, ok := fnObj.(*Fn); ok {
				if fnProg := irGetFnProg(fn); fnProg != nil && fnProg.nativeHelper != nil {
					var f64buf [4]float64
					for i := nargs - 1; i >= 0; i-- {
						sp--
						f64buf[i] = coreirx.ToFloat(stackBuf[sp])
					}
					stackBuf[sp] = coreirx.BoxDouble(fnProg.nativeHelper(coreirx.Float64(f64buf[:nargs])))
					sp++
					continue
				}
			}
			// Box args and call
			args := make([]Object, nargs)
			for i := nargs - 1; i >= 0; i-- {
				sp--
				args[i] = nbToObject(stackBuf[sp], objTable)
			}
			var result Object
			if fn, ok := fnObj.(*Fn); ok {
				if fnProg := irCompileFn(fn); fnProg != nil {
					result = irExecTyped(fnProg, args)
					if result == nil {
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
			stackBuf[sp] = nbFromObject(result, &objTable)
			sp++

		case irConj:
			sp -= 2
			coll := nbToObject(stackBuf[sp], objTable)
			val := nbToObject(stackBuf[sp+1], objTable)
			if c, ok := coll.(Conjable); ok {
				stackBuf[sp] = nbFromObject(c.Conj(val), &objTable)
				sp++
			} else {
				return nil
			}

		case irCount:
			sp--
			v := stackBuf[sp]
			if coreirx.IsObj(v) {
				obj := nbToObject(v, objTable)
				if c, ok := obj.(Counted); ok {
					stackBuf[sp] = coreirx.BoxInt(c.Count())
					sp++
				} else {
					return nil
				}
			} else {
				return nil
			}

		default:
			return nil // unsupported — fall back to irExecTyped
		}
	}

	if sp > 0 {
		return nbToObject(stackBuf[sp-1], objTable)
	}
	return NIL
}
