package core

import (
	"math"

	coreirx "github.com/rcarmo/go-joker/core/ir"
)

// ir_exec_typed_nb.go — NaN-boxed typed IR executor.
//
// Uses []uint64 stack (8 bytes per entry) instead of []irValue (32 bytes).
// Numeric operations are pure bit manipulation — zero allocation, zero copy.
// Object operations convert at the boundary via the local nb* helpers.
//
// This is the typed executor's hot path for numeric loops.
// Falls back to nil (letting irExecTyped handle it) for unsupported patterns.

func nbFromObject(obj Object, table *[]Object) uint64 {
	switch v := obj.(type) {
	case Int:
		return coreirx.BoxInt(v.I)
	case Double:
		return coreirx.BoxDouble(v.D)
	case Boolean:
		return coreirx.BoxBool(v.B)
	case Nil:
		return coreirx.BoxNil()
	default:
		idx := len(*table)
		*table = append(*table, obj)
		return coreirx.BoxObj(idx)
	}
}

func nbToObject(v uint64, table []Object) Object {
	if coreirx.IsDouble(v) {
		return Double{D: coreirx.ToDouble(v)}
	}
	if coreirx.IsInt(v) {
		return Int{I: coreirx.ToInt(v)}
	}
	if coreirx.IsBool(v) {
		return Boolean{B: coreirx.ToBool(v)}
	}
	if coreirx.IsNil(v) {
		return NIL
	}
	if coreirx.IsObj(v) {
		idx := coreirx.ToObjIdx(v)
		if idx < len(table) {
			return table[idx]
		}
	}
	return NIL
}

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

	numSlots := runtimeExec.ProgramNumSlots(prog)
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
	captureIdxs, captureSlots := runtimeExec.ProgramCaptureSlots(prog)
	for i, obj := range captureSlots {
		if i >= len(captureIdxs) || captureIdxs[i] < 0 || captureIdxs[i] >= len(slots) {
			return nil
		}
		slots[captureIdxs[i]] = nbFromObject(obj, &objTable)
	}

	// Pre-convert constants
	constants := runtimeExec.ProgramConstants(prog)
	consts := make([]uint64, len(constants))
	for i, c := range constants {
		consts[i] = nbFromObject(c, &objTable)
	}

	var stackBuf [32]uint64
	sp := 0
	code := runtimeExec.ProgramCode(prog)
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
				stackBuf[sp-1] = coreirx.BoxBool(runtimeExec.Equal(oa, ob))
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
			obj, ok := runtimeExec.Nth(coll, idx)
			if !ok {
				return nil
			}
			stackBuf[sp] = nbFromObject(obj, &objTable)
			sp++

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnObj := nbToObject(slots[slotIdx], objTable)
			// Native f64 fast path
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
						sp--
						f64args[i] = coreirx.ToFloat(stackBuf[sp])
					}
					stackBuf[sp] = coreirx.BoxDouble(nativeHelper(coreirx.Float64(f64args)))
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
			if fnProg, ok := runtimeExec.CompileFnProgram(fnObj); ok {
				result = irExecTyped(fnProg, args)
				if result == nil {
					result = irExec(fnProg, args)
				}
				if result == nil {
					var ok bool
					result, ok = runtimeExec.CallObject(fnObj, args)
					if !ok {
						return nil
					}
				}
			} else {
				var ok bool
				result, ok = runtimeExec.CallObject(fnObj, args)
				if !ok {
					return nil
				}
			}
			stackBuf[sp] = nbFromObject(result, &objTable)
			sp++

		case irConj:
			sp -= 2
			coll := nbToObject(stackBuf[sp], objTable)
			val := nbToObject(stackBuf[sp+1], objTable)
			result, ok := runtimeExec.Conj(coll, val)
			if !ok {
				return nil
			}
			stackBuf[sp] = nbFromObject(result, &objTable)
			sp++

		case irCount:
			sp--
			v := stackBuf[sp]
			if !coreirx.IsObj(v) {
				return nil
			}
			count, ok := runtimeExec.Count(nbToObject(v, objTable))
			if !ok {
				return nil
			}
			stackBuf[sp] = coreirx.BoxInt(count)
			sp++

		default:
			return nil // unsupported — fall back to irExecTyped
		}
	}

	if sp > 0 {
		return nbToObject(stackBuf[sp-1], objTable)
	}
	return NIL
}
