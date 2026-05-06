package core

// escape_analysis.go — determines which IR slots can safely use in-place mutation.
//
// A slot is "non-escaping" if:
// 1. It is only read via irLoadSlot
// 2. It is only written via irStoreSlot or irRecur
// 3. It is only consumed by irAssoc/irNth/irGet/irGet3 (collection ops that
//    produce new values without retaining references to the original)
// 4. It is NOT passed to irCallSlot/irCallSelf (which could alias it)
//
// Non-escaping vector slots can use in-place mutation (transient optimization).

// EscapeInfo holds escape analysis results for an IR program.
type EscapeInfo struct {
	// SafeMutableSlots[i] = true means slot i can use transient builders.
	SafeMutableSlots []bool
	// StringBuilderSlots[i] = true means slot i is used as the left operand
	// of irStr2 and can benefit from append-style TransientString building.
	StringBuilderSlots []bool
	// StringPrependSlots[i] = true means slot i is used as the right operand
	// of irStr2 and can benefit from prepend-style TransientString building.
	StringPrependSlots []bool
}

// analyzeEscapes performs escape analysis on an IR program.
func analyzeEscapes(prog *IRProgram) *EscapeInfo {
	info := &EscapeInfo{
		SafeMutableSlots:   make([]bool, prog.numSlots),
		StringBuilderSlots: make([]bool, prog.numSlots),
		StringPrependSlots: make([]bool, prog.numSlots),
	}

	// Start by assuming all slots are safe
	for i := range info.SafeMutableSlots {
		info.SafeMutableSlots[i] = true
	}

	code := prog.code
	pc := 0

	// Track which slots are used as arguments to function calls
	// or other operations that could retain references.
	//
	// Strategy: walk the bytecode and track the stack symbolically.
	// When a slot value reaches a call argument position, mark it unsafe.

	type stackEntry struct {
		fromSlot int // which slot this value came from, or -1
	}

	stack := make([]stackEntry, 0, 16)

	push := func(slot int) {
		stack = append(stack, stackEntry{fromSlot: slot})
	}
	pop := func() stackEntry {
		if len(stack) == 0 {
			return stackEntry{fromSlot: -1}
		}
		e := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return e
	}
	popN := func(n int) []stackEntry {
		entries := make([]stackEntry, n)
		for i := n - 1; i >= 0; i-- {
			entries[i] = pop()
		}
		return entries
	}

	for pc < len(code) {
		op := code[pc]
		pc++

		switch op {
		case irLiteral:
			pc += 2
			push(-1) // literal, not from a slot

		case irNthStringASCII:
			pc += 2
			pop() // index
			push(-1)

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			push(idx)

		case irStoreSlot:
			pc += 2
			pop()

		case irAdd, irSub, irMul, irDiv, irRem, irLt, irGte, irGt, irLte, irEq:
			pop()
			pop()
			push(-1) // result not from a slot

		case irInc, irDec, irIsZero, irSqrt, irFirst, irCount:
			pop()
			push(-1)

		case irGet:
			// get(coll, key) — coll is consumed, doesn't escape
			pop() // key
			pop() // coll — used for lookup only, safe
			push(-1)

		case irGet3:
			pop() // default
			pop() // key
			pop() // coll — safe
			push(-1)

		case irAssoc:
			// assoc(coll, key, val) stores key/val in the resulting collection.
			// The collection slot itself remains safe for transient mutation, but
			// key/value slots escape into the collection and must not be mutable
			// builders (e.g. TransientString).
			val := pop()
			key := pop()
			pop() // coll — safe
			if val.fromSlot >= 0 {
				info.SafeMutableSlots[val.fromSlot] = false
			}
			if key.fromSlot >= 0 {
				info.SafeMutableSlots[key.fromSlot] = false
			}
			push(-1)

		case irNth:
			pop() // idx
			pop() // coll — safe
			push(-1)

		case irConj:
			pop() // val
			pop() // coll — safe
			push(-1)

		case irCallSlot:
			pc += 4 // slot(2) + nargs(2)
			nargs := int(code[pc-2])<<8 | int(code[pc-1])
			// All arguments to a call ESCAPE — the function could retain them
			args := popN(nargs)
			for _, a := range args {
				if a.fromSlot >= 0 {
					info.SafeMutableSlots[a.fromSlot] = false
				}
			}
			push(-1) // result

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Self-call args also escape
			args := popN(nargs)
			for _, a := range args {
				if a.fromSlot >= 0 {
					info.SafeMutableSlots[a.fromSlot] = false
				}
			}
			push(-1)

		case irJumpIfNot:
			pc += 2
			pop() // condition consumed

		case irJump:
			pc += 2

		case irReturn:
			// Return value doesn't affect in-function mutation safety.
			_ = pop()

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			targetPC := int(code[pc-2])<<8 | int(code[pc-1])
			if targetPC != 0 {
				pc += 2
			}
			// Recur rebinds slots — this is safe (same as initial binding)
			popN(nargs)

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			popN(n)
			push(-1)

		case irStr1:
			pop()
			push(-1)

		case irStr2:
			b := pop()
			a := pop()
			if a.fromSlot >= 0 {
				info.StringBuilderSlots[a.fromSlot] = true
			}
			if b.fromSlot >= 0 {
				info.StringPrependSlots[b.fromSlot] = true
			}
			push(-1)

		case irToTransient, irToPersistent, irAssocBang:
			// These shouldn't appear in programs being analyzed
			// (they're generated by the optimization itself)
			pop()
			push(-1)

		default:
			// Unknown op — conservatively mark all slots as unsafe
			for i := range info.SafeMutableSlots {
				info.SafeMutableSlots[i] = false
			}
			return info
		}
	}

	return info
}
