package core

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
