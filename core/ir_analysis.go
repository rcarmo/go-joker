package core

// ir_analysis.go — centralized IR shape analysis.
//
// This pass is intentionally conservative. It does not change semantics; it
// gives diagnostics and optimization gates a single program-shape summary so
// future typed-IR, string-builder, helper-inlining and WASM decisions do not
// have to rediscover the same facts independently.

type IRAnalysis struct {
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
	HasStringAppend  bool
	HasStringPrepend bool

	StringAppendSlots  []bool
	StringPrependSlots []bool

	SuggestedPath string
}

func AnalyzeIRProgram(prog *IRProgram) IRAnalysis {
	if prog == nil {
		return IRAnalysis{SuggestedPath: "none"}
	}
	info := prog.escapeInfo
	if info == nil {
		info = analyzeEscapes(prog)
		prog.escapeInfo = info
	}
	a := IRAnalysis{
		NumSlots:           prog.numSlots,
		NumCaptures:        len(prog.captureKeys),
		UsesFloat:          irProgramUsesFloat(prog),
		StringAppendSlots:  append([]bool(nil), info.StringBuilderSlots...),
		StringPrependSlots: append([]bool(nil), info.StringPrependSlots...),
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

	code := prog.code
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		a.NumOps++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot, irJumpIfNot, irJump, irBuildVec, irNthStringASCII:
			if op == irBuildVec {
				a.UsesCollection = true
			}
			if op == irNthStringASCII {
				a.UsesString = true
			}
			pc += 2
		case irGet, irGet3, irAssoc, irNth, irConj, irFirst:
			a.UsesCollection = true
			if op == irNth {
				// Could be string or collection at runtime; mark both so gates stay conservative.
				a.UsesString = true
			}
		case irCount:
			// Count is eligible for typed string loops; collection uses remain
			// represented by the producing collection ops.
			a.UsesString = true
		case irStr1, irStr2:
			a.UsesString = true
		case irToTransient, irAssocBang, irToPersistent:
			a.UsesTransient = true
			a.UsesCollection = true
		case irCallSlot:
			a.HasCallSlot = true
			a.CallSlotCount++
			pc += 4
		case irCallSelf:
			a.HasSelfCall = true
			pc += 2
		case irRecur:
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
	a.SuggestedPath = suggestIRPath(a)
	return a
}

func suggestIRPath(a IRAnalysis) string {
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
	if a.UsesCollection && !a.UsesString {
		return "ir-collection-builder-candidate"
	}
	if a.UsesString && a.UsesCollection {
		return "typed-ir-string-collection-candidate"
	}
	return "boxed-ir"
}
