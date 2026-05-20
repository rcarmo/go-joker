package core

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"
	"unicode/utf8"
	"unsafe"

	coreirx "github.com/rcarmo/go-joker/core/ir"
	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// ---- boxed_exec.go ----
// ---------- Interpreter ----------

func irExec(prog *IRProgram, initSlots []coretypes.Object) coretypes.Object {
	defer traceIRProgramCall(prog, len(initSlots))()
	irProfileExecStart()
	defer irProfileMaybeWrite()
	var slots []coretypes.Object
	if runtimeExec.ProgramNumSlots(prog) <= 16 {
		var buf [16]coretypes.Object
		slots = buf[:runtimeExec.ProgramNumSlots(prog)]
	} else {
		slots = make([]coretypes.Object, runtimeExec.ProgramNumSlots(prog))
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

	var stack []coretypes.Object
	var stackBuf [16]coretypes.Object
	stack = stackBuf[:0]
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	// Frame stack for irCallSelf — avoids recursive irExec calls
	var frameStack *coreirx.FrameStack[coretypes.Object]
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
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Int{I: av.I + bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: float64(av.I) + bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Double{D: av.D + float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: av.D + bv.D})
					continue
				}
			}
			return nil

		case irSub:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Int{I: av.I - bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: float64(av.I) - bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Double{D: av.D - float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: av.D - bv.D})
					continue
				}
			}
			return nil

		case irMul:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Int{I: av.I * bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: float64(av.I) * bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Double{D: av.D * float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: av.D * bv.D})
					continue
				}
			}
			return nil

		case irRem:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if ai, aok := a.(coretypes.Int); aok {
				if bi, bok := b.(coretypes.Int); bok {
					if bi.I == 0 {
						return nil
					}
					stack = append(stack, coretypes.Int{I: ai.I % bi.I})
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
			case coretypes.Int:
				av = float64(x.I)
			case coretypes.Double:
				av = x.D
			default:
				return nil
			}
			switch x := b.(type) {
			case coretypes.Int:
				bv = float64(x.I)
			case coretypes.Double:
				bv = x.D
			default:
				return nil
			}
			if bv == 0 {
				return nil
			}
			stack = append(stack, coretypes.Double{D: av / bv})

		case irInc:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Int:
				stack = append(stack, coretypes.Int{I: av.I + 1})
			case coretypes.Double:
				stack = append(stack, coretypes.Double{D: av.D + 1})
			default:
				return nil
			}

		case irDec:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Int:
				stack = append(stack, coretypes.Int{I: av.I - 1})
			case coretypes.Double:
				stack = append(stack, coretypes.Double{D: av.D - 1})
			default:
				return nil
			}

		case irLt:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I < bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) < bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D < float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D < bv.D})
					continue
				}
			}
			return nil

		case irGte:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I >= bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) >= bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D >= float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D >= bv.D})
					continue
				}
			}
			return nil

		case irGt:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I > bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) > bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D > float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D > bv.D})
					continue
				}
			}
			return nil

		case irLte:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I <= bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) <= bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D <= float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D <= bv.D})
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
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I & b.I})
		case irBitOr:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I | b.I})
		case irBitNot:
			a := stack[len(stack)-1].(coretypes.Int)
			stack = stack[:len(stack)-1]
			stack = append(stack, coretypes.Int{I: ^a.I})
		case irBitShiftLeft:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I << uint(b.I)})
		case irBitShiftRight:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I >> uint(b.I)})

		case irCase:
			// Jump table: dispatch by integer value
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nCases := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			var testVal int
			switch v := slots[slotIdx].(type) {
			case coretypes.Int:
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
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I == bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) == bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D == float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D == bv.D})
					continue
				}
			case coretypes.Char:
				if bv, ok := b.(coretypes.Char); ok {
					stack = append(stack, coretypes.Boolean{B: av.Ch == bv.Ch})
					continue
				}
			case coretypes.String:
				if bv, ok := b.(coretypes.String); ok {
					stack = append(stack, coretypes.Boolean{B: av.S == bv.S})
					continue
				}
			}
			stack = append(stack, coretypes.Boolean{B: runtimeExec.Equal(a, b)})

		case irIsZero:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Int:
				stack = append(stack, coretypes.Boolean{B: av.I == 0})
			case coretypes.Double:
				stack = append(stack, coretypes.Boolean{B: av.D == 0})
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
			case coretypes.Boolean:
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
			if _, ok := coll.(coretypes.Gettable); !ok {
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
			idx, iok := idxObj.(coretypes.Int)
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
			case coretypes.Double:
				stack = append(stack, coretypes.Double{D: math.Sqrt(av.D)})
			case coretypes.Int:
				stack = append(stack, coretypes.Double{D: math.Sqrt(float64(av.I))})
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
						case coretypes.Double:
							f64args[i] = v.D
						case coretypes.Int:
							f64args[i] = float64(v.I)
						default:
							f64args[i] = 0
						}
						stack = stack[:len(stack)-1]
					}
					stack = append(stack, coretypes.Double{D: nativeHelper(coreirx.Float64(f64args))})
					continue
				}
			}
			// Slow path
			var args []coretypes.Object
			var argsBuf [4]coretypes.Object
			if nargs > 0 {
				if nargs <= len(argsBuf) {
					args = argsBuf[:nargs]
				} else {
					args = make([]coretypes.Object, nargs)
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
				frameStack = coreirx.NewFrameStack[coretypes.Object](runtimeExec.ProgramNumSlots(prog))
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
				var args []coretypes.Object
				var argsBuf [4]coretypes.Object
				if nargs <= len(argsBuf) {
					args = argsBuf[:nargs]
				} else {
					args = make([]coretypes.Object, nargs)
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
			arr := make([]coretypes.Object, n)
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
			idx, ok := idxObj.(coretypes.Int)
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
			stack = append(stack, coretypes.Int{I: count})

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
			case coretypes.Char:
				stack = append(stack, coretypes.Int{I: int(v.Ch)})
			case coretypes.Int:
				stack = append(stack, v)
			case coretypes.Double:
				stack = append(stack, coretypes.Int{I: int(v.D)})
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
				s := sObj.(coretypes.String).S
				si := start.(coretypes.Int).I
				ei := end.(coretypes.Int).I
				if coretypes.StringIsASCII(s) {
					stack = append(stack, coretypes.String{S: s[si:ei]})
				} else {
					runes := []rune(s)
					stack = append(stack, coretypes.String{S: string(runes[si:ei])})
				}
			} else {
				start := stack[len(stack)-1]
				sObj := stack[len(stack)-2]
				stack = stack[:len(stack)-2]
				s := sObj.(coretypes.String).S
				si := start.(coretypes.Int).I
				if coretypes.StringIsASCII(s) {
					stack = append(stack, coretypes.String{S: s[si:]})
				} else {
					runes := []rune(s)
					stack = append(stack, coretypes.String{S: string(runes[si:])})
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

// ---- typed_exec.go ----
func irExecTyped(prog *IRProgram, initSlots []coretypes.Object) coretypes.Object {
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
			capturedSlots := make([]coretypes.Object, len(slots))
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
				// General assoc path for coretypes.Object types (vector of doubles, etc.)
				result, ok := runtimeExec.Assoc(coll.object(), key.object(), val.object())
				if !ok {
					return nil
				}
				stack = append(stack, objectToIRValue(result))
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
				obj, ok := runtimeExec.Nth(coll.obj(), idx.i)
				if !ok {
					return nil
				}
				stack = append(stack, objectToIRValue(obj))
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
			s := constant.(coretypes.String).S
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
			var fnObj coretypes.Object
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
			var result coretypes.Object
			if baseProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if baseProg != nil && runtimeExec.HasNativeHelper(baseProg) {
					// Already handled above
				} else if fnProg := runtimeExec.DispatchArityProgram(baseProg, nargs); runtimeExec.CanExecuteIR(fnProg) {
					routedToIR := false
					if runtimeExec.CanExecuteTypedIR(fnProg) {
						// FAST PATH: typed sub-call without coretypes.Object boxing
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
					var argsBuf [4]coretypes.Object
					args := runtimeExec.ObjectsFromTypedValues(typedArgs, argsBuf[:])
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
					var args2 [4]coretypes.Object
					a := runtimeExec.ObjectsFromTypedValues(typedArgs, args2[:])
					var ok bool
					result, ok = runtimeExec.CallObject(fnObj, a)
					if !ok {
						return nil
					}
				}
			} else {
				var args3 [4]coretypes.Object
				a := runtimeExec.ObjectsFromTypedValues(typedArgs, args3[:])
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
				args := make([]coretypes.Object, nargs)
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
			arr := make([]coretypes.Object, n)
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
// directly, avoiding the coretypes.Object boxing/unboxing at callSlot boundaries.
// Returns (result, true) on success, (zero, false) on failure.

// ---- typed_exec_inline.go ----
func irExecTypedIV(prog *IRProgram, initSlots []coretypes.Object) (irValue, bool) {
	result := irExecTyped(prog, initSlots)
	if result == nil {
		return irValue{}, false
	}
	return objectToIRValue(result), true
}

// irExecTypedInline executes a typed IR program with pre-filled irValue slots.
// Returns the result as irValue directly (no coretypes.Object boxing).
// Returns zero irValue on failure.
func irExecTypedInline(prog *IRProgram, slots []irValue) irValue {
	var stackBuf [32]irValue
	stack := stackBuf[:0]
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			constant, ok := runtimeExec.ProgramConstant(prog, idx)
			if !ok {
				return irValue{}
			}
			stack = append(stack, objectToIRValue(constant))
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

// ---- typed_exec_nanbox.go ----
// ir_exec_typed_nb.go — NaN-boxed typed IR executor.
//
// Uses []uint64 stack (8 bytes per entry) instead of []irValue (32 bytes).
// Numeric operations are pure bit manipulation — zero allocation, zero copy.
// coretypes.Object operations convert at the boundary via the local nb* helpers.
//
// This is the typed executor's hot path for numeric loops.
// Falls back to nil (letting irExecTyped handle it) for unsupported patterns.

func irExecTypedNB(prog *IRProgram, initSlots []coretypes.Object) coretypes.Object {
	analysis := AnalyzeIRProgram(prog)
	// Only handle numeric-dominant programs without complex collection ops
	if !irTypedEligible(analysis) {
		return nil
	}
	// Only handle pure numeric programs — no collections, no self-calls,
	// no strings, no fn calls (which allocate []coretypes.Object args).
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

	// coretypes.Object side-table for non-numeric values
	var objTable []coretypes.Object

	// Convert init slots
	for i := 0; i < numSlots && i < len(initSlots); i++ {
		slots[i] = coreirx.NBFromObject(initSlots[i], &objTable, IsNil)
	}
	// Pre-fill captures
	captureIdxs, captureSlots := runtimeExec.ProgramCaptureSlots(prog)
	for i, obj := range captureSlots {
		if i >= len(captureIdxs) || captureIdxs[i] < 0 || captureIdxs[i] >= len(slots) {
			return nil
		}
		slots[captureIdxs[i]] = coreirx.NBFromObject(obj, &objTable, IsNil)
	}

	// Pre-convert constants
	constants := runtimeExec.ProgramConstants(prog)
	consts := make([]uint64, len(constants))
	for i, c := range constants {
		consts[i] = coreirx.NBFromObject(c, &objTable, IsNil)
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
				oa := coreirx.NBToObject(a, objTable, NIL)
				ob := coreirx.NBToObject(b, objTable, NIL)
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
			return coreirx.NBToObject(stackBuf[sp], objTable, NIL)

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

		// coretypes.Collection ops: convert at boundary
		case irNth:
			sp -= 2
			coll := coreirx.NBToObject(stackBuf[sp], objTable, NIL)
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
			stackBuf[sp] = coreirx.NBFromObject(obj, &objTable, IsNil)
			sp++

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnObj := coreirx.NBToObject(slots[slotIdx], objTable, NIL)
			// coretypes.Native f64 fast path
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
			// corecollections.Box args and call
			args := make([]coretypes.Object, nargs)
			for i := nargs - 1; i >= 0; i-- {
				sp--
				args[i] = coreirx.NBToObject(stackBuf[sp], objTable, NIL)
			}
			var result coretypes.Object
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
			stackBuf[sp] = coreirx.NBFromObject(result, &objTable, IsNil)
			sp++

		case irConj:
			sp -= 2
			coll := coreirx.NBToObject(stackBuf[sp], objTable, NIL)
			val := coreirx.NBToObject(stackBuf[sp+1], objTable, NIL)
			result, ok := runtimeExec.Conj(coll, val)
			if !ok {
				return nil
			}
			stackBuf[sp] = coreirx.NBFromObject(result, &objTable, IsNil)
			sp++

		case irCount:
			sp--
			v := stackBuf[sp]
			if !coreirx.IsObj(v) {
				return nil
			}
			count, ok := runtimeExec.Count(coreirx.NBToObject(v, objTable, NIL))
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
		return coreirx.NBToObject(stackBuf[sp-1], objTable, NIL)
	}
	return NIL
}

// ---- typed_value_accessors.go ----
// ir_value_accessors.go — typed accessors for irValue's unsafe.Pointer field.
//
// irValue stores extended data (strings, collections, objects) behind an
// unsafe.Pointer to keep the struct at 32 bytes for the numeric hot path.
// These accessors provide type-safe reads/writes.

// --- String ---

func irMakeString(s string, runeCount int, ascii bool) irValue {
	v := irValue{tag: irValString, i: runeCount, p: unsafe.Pointer(&s)}
	if ascii {
		v.f = 1
	}
	return v
}

func (v irValue) str() string {
	if v.p == nil {
		return ""
	}
	return *(*string)(v.p)
}

func (v irValue) isASCII() bool { return v.f != 0 }

// --- StringBuilder ([]byte) ---

func irMakeStringBuilder(buf []byte, runeCount int, ascii bool) irValue {
	v := irValue{tag: irValStringBuilder, i: runeCount, p: unsafe.Pointer(&buf)}
	if ascii {
		v.f = 1
	}
	return v
}

func (v irValue) bytes() []byte {
	if v.p == nil {
		return nil
	}
	return *(*[]byte)(v.p)
}

func (v *irValue) setBytes(buf []byte) {
	v.p = unsafe.Pointer(&buf)
}

func (v *irValue) setASCII(ascii bool) {
	if ascii {
		v.f = 1
	} else {
		v.f = 0
	}
}

// --- Bool ---

func irMakeBool(b bool) irValue {
	v := irValue{tag: irValBool}
	if b {
		v.i = 1
	}
	return v
}

func (v irValue) boolean() bool { return v.i != 0 }

// --- Char ---

func irMakeChar(r rune) irValue {
	return irValue{tag: irValChar, i: int(r)}
}

func (v irValue) char() rune { return rune(v.i) }

// --- StringIntMap ---

func irMakeStringIntMap(m map[string]int) irValue {
	return irValue{tag: irValStringIntMap, p: unsafe.Pointer(&m)}
}

func (v irValue) stringIntMap() map[string]int {
	if v.p == nil {
		return nil
	}
	return *(*map[string]int)(v.p)
}

func (v *irValue) setStringIntMap(m map[string]int) {
	v.p = unsafe.Pointer(&m)
}

// --- IntVector ---

func irMakeIntVector(iv []int) irValue {
	return irValue{tag: irValIntVector, p: unsafe.Pointer(&iv)}
}

func (v irValue) intVec() []int {
	if v.p == nil {
		return nil
	}
	return *(*[]int)(v.p)
}

func (v *irValue) setIntVec(iv []int) {
	v.p = unsafe.Pointer(&iv)
}

// --- coretypes.Object ---

func irMakeObject(obj coretypes.Object) irValue {
	// For common concrete pointer types, store directly to avoid
	// allocating an coretypes.Object interface box. Use i field as sub-tag.
	switch v := obj.(type) {
	case *corecollections.ArrayVector:
		return irValue{tag: irValObject, i: 1, p: unsafe.Pointer(v)}
	case *coretypes.TransientVector:
		return irValue{tag: irValObject, i: 2, p: unsafe.Pointer(v)}
	case *Fn:
		return irValue{tag: irValObject, i: 3, p: unsafe.Pointer(v)}
	default:
		p := new(coretypes.Object)
		*p = obj
		return irValue{tag: irValObject, i: 0, p: unsafe.Pointer(p)}
	}
}

func (v irValue) obj() coretypes.Object {
	if v.p == nil {
		return NIL
	}
	switch v.i {
	case 1:
		return (*corecollections.ArrayVector)(v.p)
	case 2:
		return (*coretypes.TransientVector)(v.p)
	case 3:
		return (*Fn)(v.p)
	default:
		return *(*coretypes.Object)(v.p)
	}
}

// ---- typed_values.go ----
// ir_typed.go — experimental typed IR executor (v2).
//
// This is the first incremental step away from the boxed []coretypes.Object stack used by
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
	irValKeyword
	irValCursor // StringCursor pointer in obj field
)

// irValue is the tagged value for the typed IR executor.
// Layout: 32 bytes for the compact numeric path.
// String/collection data is stored behind an unsafe.Pointer to avoid
// bloating the struct for the common numeric case.
type irValue struct {
	tag irValueTag
	i   int            // int value, bool (0/1), rune, rune count for strings
	f   float64        // double value, or ASCII flag (nonzero = ASCII) for strings
	p   unsafe.Pointer // -> string | []byte | map[string]int | []int | coretypes.Object
}

func irTypedEligible(a coreirx.Analysis) bool {
	if a.NumOps == 0 || a.UsesTransient {
		return false
	}
	// Call-slot loops: allow if numeric-only or numeric+generic-nth
	if a.HasCallSlot {
		return !a.UsesString && !a.HasMapOps && (!a.UsesCollection || a.HasGenericNth)
	}
	// coretypes.Collection programs with nth but NO assoc (read-only vector access)
	if a.UsesCollection && a.HasGenericNth && !a.HasMapOps && !a.UsesString && !a.HasAssoc {
		return true
	}
	// coretypes.Collection programs with assoc: prefer boxed executor (has transient support)
	if a.UsesCollection && a.HasGenericNth && a.HasAssoc && !a.HasMapOps && !a.UsesString {
		return false
	}
	if a.UsesCollection && (a.HasMapOps || !a.HasGenericNth) {
		if corert.IRTypedMapEnabled() && a.HasMapOps && a.UsesString {
			return true
		}
		// Self-recursive tree builders/walkers (binary-trees pattern)
		if a.HasSelfCall && !a.HasMapOps && !a.UsesString {
			return true
		}
		return corert.IRTypedVecEnabled() && a.UsesCollection && !a.UsesString && !a.HasMapOps
	}
	// Accept: pure numeric loops (no strings, no collections, no call-slots)
	if !a.UsesString && !a.UsesCollection && !a.HasCallSlot {
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

func objectToIRValue(obj coretypes.Object) irValue {
	switch v := obj.(type) {
	case coretypes.Int:
		return irValue{tag: irValInt, i: v.I}
	case coretypes.Double:
		return irValue{tag: irValDouble, f: v.D}
	case coretypes.Boolean:
		return irMakeBool(v.B)
	case coretypes.Char:
		return irMakeChar(v.Ch)
	case coretypes.String:
		return stringToIRValue(v.S)
	case *corecollections.ArrayVector:
		if corert.IRTypedVecEnabled() {
			iv := make([]int, len(v.Arr))
			for i, obj := range v.Arr {
				x, ok := obj.(coretypes.Int)
				if !ok {
					return irMakeObject(obj)
				}
				iv[i] = x.I
			}
			return irMakeIntVector(iv)
		}
	case *corecollections.ArrayMap:
		if v.Count() == 0 {
			return irMakeStringIntMap(make(map[string]int))
		}
	case *corecollections.HashMap:
		if v.Count() == 0 {
			return irMakeStringIntMap(make(map[string]int))
		}
	case Nil:
		return irValue{tag: irValNil}
	case coretypes.Keyword:
		return irValue{tag: irValKeyword, p: unsafe.Pointer(v.NameKey())}
	case *StringCursor:
		return irValue{tag: irValCursor, p: unsafe.Pointer(v)}
	default:
		return irMakeObject(obj)
	}
	return irMakeObject(obj)
}

func (v irValue) object() coretypes.Object {
	switch v.tag {
	case irValInt:
		return coretypes.Int{I: v.i}
	case irValDouble:
		return coretypes.Double{D: v.f}
	case irValBool:
		return coretypes.Boolean{B: v.boolean()}
	case irValChar:
		return coretypes.Char{Ch: v.char()}
	case irValString:
		return coretypes.String{S: v.str()}
	case irValStringBuilder:
		return coretypes.String{S: string(v.bytes())}
	case irValStringIntMap:
		res := corecollections.EmptyArrayMap()
		for k, v := range v.stringIntMap() {
			res.Add(coretypes.String{S: k}, coretypes.Int{I: v})
		}
		return res
	case irValIntVector:
		arr := make([]coretypes.Object, len(v.intVec()))
		for i, x := range v.intVec() {
			arr[i] = coretypes.Int{I: x}
		}
		return runtimeExec.BuildVector(arr)
	case irValNil:
		return NIL
	case irValKeyword:
		return keywordObjectFromName((*string)(v.p))
	case irValCursor:
		return (*StringCursor)(v.p)
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
		case irValKeyword:
			// Keywords are interned — pointer equality on name
			return irMakeBool(a.p == b.p), true
		}
	}
	if a.tag == irValInt && b.tag == irValDouble {
		return irMakeBool(float64(a.i) == b.f), true
	}
	if a.tag == irValDouble && b.tag == irValInt {
		return irMakeBool(a.f == float64(b.i)), true
	}
	return irMakeBool(runtimeExec.Equal(a.object(), b.object())), true
}

// keywordObjectCache caches Keyword Objects by name pointer to avoid
// repeated heap allocation when converting irValKeyword → coretypes.Object.
var keywordObjectCache sync.Map // *string → coretypes.Object (Keyword)

func keywordObjectFromName(name *string) coretypes.Object {
	if v, ok := keywordObjectCache.Load(name); ok {
		return v.(coretypes.Object)
	}
	kw := coretypes.MakeKeywordFromKeys(nil, name)
	// Store as coretypes.Object interface to avoid re-boxing
	var obj coretypes.Object = kw
	keywordObjectCache.Store(name, obj)
	return obj
}

// ---- wasm_exec_runtime.go ----
// wasm_runtime.go — wazero-based WASM execution engine.
// Compiles WASM modules and caches them. Handles coretypes.Object ↔ WASM i64 conversion.

// WasmProgram is a compiled, ready-to-execute WASM module.
type WasmProgram struct {
	mod        api.Module
	execFn     api.Function
	useFloat   bool
	hasImports bool
	constants  []coretypes.Object // pre-stored constants for handle references
	bytes      []byte             // raw wasm module for export/debugging
}

var (
	wasmRT     wazero.Runtime
	wasmRTOnce sync.Once
	wasmCache  sync.Map // map[*IRProgram]*WasmProgram
	wasmFail   = &WasmProgram{}
)

func getWasmRT() wazero.Runtime {
	wasmRTOnce.Do(func() {
		cache := wazero.NewCompilationCache()
		wasmRT = wazero.NewRuntimeWithConfig(context.Background(),
			wazero.NewRuntimeConfig().WithCompilationCache(cache))
		// Register host functions for collection operations
		registerWasmHost(wasmRT)
	})
	return wasmRT
}

// wasmGetCached retrieves or compiles a WASM program for an IR program.
func wasmGetCached(prog *IRProgram) *WasmProgram {
	if v, ok := wasmCache.Load(prog); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFail {
			return nil
		}
		return wp
	}
	wp := wasmCompile(prog)
	if wp == nil {
		wasmCache.Store(prog, wasmFail)
		return nil
	}
	wasmCache.Store(prog, wp)
	return wp
}

// wasmCompile translates IR → WASM binary → wazero compiled module.
func closeWasmModule(ctx context.Context, mod api.Module) {
	if err := mod.Close(ctx); err != nil {
		fmt.Fprintln(Stderr, "wasm module close error:", err)
	}
}

func wasmCompile(prog *IRProgram) *WasmProgram {
	// Try pure-numeric path first (faster, no imports needed)
	bin := irToWasm(prog)
	// TODO: enable imports path once collection handle ABI/control-flow is fully validated.
	// if bin == nil {
	// 	bin = irToWasmWithImports(prog)
	// }
	if bin == nil {
		return nil
	}

	rt := getWasmRT()
	ctx := context.Background()

	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return nil
	}

	cfg := wazero.NewModuleConfig().WithName(corewasm.NextWasmModuleName())
	mod, err := rt.InstantiateModule(ctx, compiled, cfg)
	if err != nil {
		return nil
	}

	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		closeWasmModule(ctx, mod)
		return nil
	}
	model := runtimeExec.ProgramModel(prog)
	if model == nil {
		closeWasmModule(ctx, mod)
		return nil
	}

	wp := &WasmProgram{
		mod:        mod,
		execFn:     execFn,
		useFloat:   corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0),
		hasImports: !corewasm.Eligible(model.Code),
		constants:  runtimeExec.ProgramConstants(prog),
		bytes:      append([]byte(nil), bin...),
	}
	return wp
}

func wasmExec(wp *WasmProgram, slots []coretypes.Object) coretypes.Object {
	// Create object table for this execution
	table := corewasm.NewObjectTable(NIL)

	// Pre-populate with IR program constants (for handle references)
	if wp.hasImports && len(wp.constants) > 0 {
		for _, c := range wp.constants {
			table.Store(c)
		}
	}

	var stackBuf [16]uint64
	var stack []uint64
	if len(slots) <= len(stackBuf) {
		stack = stackBuf[:len(slots)]
	} else {
		stack = make([]uint64, len(slots))
	}
	for i, s := range slots {
		switch v := s.(type) {
		case coretypes.Int:
			if wp.useFloat {
				stack[i] = math.Float64bits(float64(v.I))
			} else {
				stack[i] = uint64(v.I)
			}
		case coretypes.Double:
			if wp.useFloat {
				stack[i] = math.Float64bits(v.D)
			} else {
				return nil
			}
		default:
			stack[i] = table.Store(s)
		}
	}

	ctx := corewasm.WithObjectTable(context.Background(), table)
	if err := wp.execFn.CallWithStack(ctx, stack); err != nil {
		return nil
	}

	r := stack[0]
	if wp.useFloat {
		return coretypes.Double{D: math.Float64frombits(r)}
	}
	// Check if result is a handle
	if corewasm.IsHandle(r) {
		return table.Load(r)
	}
	return corewasm.RawIntObject(r)
}

// Ensure math import is used
var _ = math.Float64bits
