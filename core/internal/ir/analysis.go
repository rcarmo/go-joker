package ir

// Analysis summarizes a program's bytecode shape for diagnostics and execution
// strategy selection. Core supplies object/runtime-specific facts such as
// escape/string-builder slots and float use.
type Analysis struct {
	NumOps           int
	NumSlots         int
	NumCaptures      int
	UsesFloat        bool
	UsesString       bool
	UsesCollection   bool
	UsesTransient    bool
	HasCallSlot      bool
	CallSlotCount    int
	HasSelfCall      bool
	HasNestedRecur   bool
	HasGenericNth    bool
	HasMapOps        bool
	HasAssoc         bool
	HasStringAppend  bool
	HasStringPrepend bool

	StringAppendSlots  []bool
	StringPrependSlots []bool

	SuggestedPath string
}

func Analyze(code []byte, numSlots, numCaptures int, usesFloat bool, stringAppendSlots, stringPrependSlots []bool) Analysis {
	a := Analysis{
		NumSlots:           numSlots,
		NumCaptures:        numCaptures,
		UsesFloat:          usesFloat,
		StringAppendSlots:  append([]bool(nil), stringAppendSlots...),
		StringPrependSlots: append([]bool(nil), stringPrependSlots...),
	}
	for _, ok := range a.StringAppendSlots {
		if ok {
			a.HasStringAppend = true
			break
		}
	}
	for _, ok := range a.StringPrependSlots {
		if ok {
			a.HasStringPrepend = true
			break
		}
	}
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		a.NumOps++
		switch op {
		case Literal, LoadSlot, StoreSlot, JumpIfNot, Jump, BuildVec, NthStringASCII:
			if op == BuildVec {
				a.UsesCollection = true
			}
			if op == NthStringASCII {
				a.UsesString = true
			}
			pc += 2
		case Get, Get3:
			a.UsesCollection = true
			a.HasMapOps = true
		case Assoc:
			a.UsesCollection = true
			a.HasAssoc = true
		case First, Conj:
			a.UsesCollection = true
		case Nth:
			a.UsesCollection = true
			a.HasGenericNth = true
		case Count:
			// Count alone is type-polymorphic.
		case Str1, Str2:
			a.UsesString = true
		case ToTransient, AssocBang, ToPersistent:
			a.UsesTransient = true
			a.UsesCollection = true
		case CallSlot:
			a.HasCallSlot = true
			a.CallSlotCount++
			pc += 4
		case CallSelf:
			a.HasSelfCall = true
			pc += 2
		case Recur:
			pc += 4
			if pc <= len(code) {
				tgt := int(code[pc-2])<<8 | int(code[pc-1])
				if tgt != 0 {
					a.HasNestedRecur = true
					if pc+2 <= len(code) {
						pc += 2
					}
				}
			}
		}
	}
	a.SuggestedPath = SuggestPath(a)
	return a
}

func SuggestPath(a Analysis) string {
	if a.NumOps == 0 {
		return "none"
	}
	if !a.UsesString && !a.UsesCollection && !a.HasCallSlot && !a.HasNestedRecur {
		return "wasm"
	}
	if a.HasCallSlot && !a.UsesCollection && !a.UsesString {
		return "wasm-multifn-candidate"
	}
	if a.HasStringPrepend {
		return "ir-string-prepend-builder"
	}
	if a.HasStringAppend {
		return "ir-string-append-builder-candidate"
	}
	if a.UsesString && !a.UsesCollection {
		return "typed-ir-string-candidate"
	}
	if a.HasGenericNth && a.UsesString && !a.HasMapOps {
		return "typed-ir-generic-string-nth-candidate"
	}
	if a.UsesCollection && !a.UsesString {
		return "ir-collection-builder-candidate"
	}
	if a.UsesString && a.UsesCollection {
		return "typed-ir-string-collection-candidate"
	}
	return "boxed-ir"
}
