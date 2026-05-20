package ir

import "testing"

func TestProgramCopiesNeutralModelFields(t *testing.T) {
	code := []byte{LoadSlot, 0, 1, Return}
	p := NewProgram(code, 3, 2).WithCaptures([]int{1}, []bool{false, true, false})
	p.HasSelf = true
	p.FloatConsts = []float64{1.5}

	code[0] = Return
	if p.Code[0] != LoadSlot {
		t.Fatal("Program should copy bytecode")
	}
	if p.NumSlots != 3 || p.ConstantsLen != 2 || !p.HasSelf {
		t.Fatalf("unexpected model header: %#v", p)
	}
	if len(p.CaptureSlotIdxs) != 1 || p.CaptureSlotIdxs[0] != 1 || !p.CaptureSlotSet[1] {
		t.Fatalf("capture metadata mismatch: %#v", p)
	}
}

func TestProgramArityModelCopiesMap(t *testing.T) {
	child := NewProgram([]byte{Return}, 1, 0)
	programs := map[int]*Program{1: child}
	wrapper := NewProgram(nil, 0, 0).WithArityPrograms(programs, child, 2)
	delete(programs, 1)
	if wrapper.ArityPrograms[1] != child {
		t.Fatal("arity program map was not copied")
	}
	if wrapper.VariadicProgram != child || wrapper.VariadicMinArgs != 2 {
		t.Fatal("variadic metadata mismatch")
	}
}

func TestFloat64Identity(t *testing.T) {
	in := []float64{1, 2, 3}
	out := Float64(in)
	if len(out) != 3 || out[0] != 1 || out[2] != 3 {
		t.Fatalf("Float64 identity failed: %#v", out)
	}
}
