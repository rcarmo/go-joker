package core

// ir_native_helper.go — compile pure arithmetic helpers to Go closures.
//
// When a loop calls a pure arithmetic helper via irCallSlot, this path
// compiles the helper's IR to a native Go function that operates on
// float64 values directly, eliminating WASM/IR dispatch and Object boxing.

import "math"

// nativeF64Fn is a compiled Go closure for a pure arithmetic helper.
type nativeF64Fn func(args []float64) float64

// irCompileNativeHelper attempts to compile an IR program (helper function)
// to a native Go float64 closure.
func irCompileNativeHelper(prog *IRProgram) nativeF64Fn {
	if prog == nil || prog.hasSelf {
		return nil
	}
	// Only compile pure numeric programs (no collections, strings, calls)
	code := prog.code
	for pc := 0; pc < len(code); {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irDiv, irRem, irInc, irDec,
			irLt, irEq, irIsZero, irReturn, irSqrt:
			// ok
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				return nil
			}
		default:
			return nil
		}
	}

	// Build constants as float64
	consts := make([]float64, len(prog.constants))
	for i, c := range prog.constants {
		switch v := c.(type) {
		case Int:
			consts[i] = float64(v.I)
		case Double:
			consts[i] = v.D
		default:
			return nil
		}
	}

	numSlots := prog.numSlots
	codeSlice := prog.code

	return func(args []float64) float64 {
		var slotBuf [8]float64
		var slots []float64
		if numSlots <= len(slotBuf) {
			slots = slotBuf[:numSlots]
		} else {
			slots = make([]float64, numSlots)
		}
		copy(slots, args)

		var stack [16]float64
		sp := 0
		pc := 0

		for pc < len(codeSlice) {
			op := codeSlice[pc]
			pc++
			switch op {
			case irLiteral:
				idx := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				stack[sp] = consts[idx]
				sp++
			case irLoadSlot:
				idx := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				stack[sp] = slots[idx]
				sp++
			case irStoreSlot:
				idx := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				sp--
				slots[idx] = stack[sp]
			case irAdd:
				sp--
				stack[sp-1] += stack[sp]
			case irSub:
				sp--
				stack[sp-1] -= stack[sp]
			case irMul:
				sp--
				stack[sp-1] *= stack[sp]
			case irDiv:
				sp--
				stack[sp-1] /= stack[sp]
			case irSqrt:
				stack[sp-1] = math.Sqrt(stack[sp-1])
			case irRem:
				sp--
				b := int(stack[sp])
				if b != 0 {
					stack[sp-1] = float64(int(stack[sp-1]) % b)
				}
			case irInc:
				stack[sp-1]++
			case irDec:
				stack[sp-1]--
			case irLt:
				sp--
				if stack[sp-1] < stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irEq:
				sp--
				if stack[sp-1] == stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irIsZero:
				if stack[sp-1] == 0 {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irJumpIfNot:
				target := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				sp--
				if stack[sp] == 0 {
					pc = target
				}
			case irJump:
				pc = int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
			case irRecur:
				nargs := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				target := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				for i := nargs - 1; i >= 0; i-- {
					sp--
					slots[i] = stack[sp]
				}
				pc = target
			case irReturn:
				sp--
				return stack[sp]
			default:
				return 0
			}
		}
		if sp > 0 {
			return stack[sp-1]
		}
		return 0
	}
}
