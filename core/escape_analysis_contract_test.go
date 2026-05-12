package core

import "testing"

func TestEscapeAnalysisMarksCallArgumentsUnsafe(t *testing.T) {
	prog := (&IRProgram{
		code: []byte{
			irLoadSlot, 0, 0,
			irCallSlot, 0, 1, 0, 1,
			irReturn,
		},
		numSlots: 2,
	}).refreshModel()
	info := analyzeEscapes(prog)
	if len(info.SafeMutableSlots) != 2 {
		t.Fatalf("SafeMutableSlots len = %d, want 2", len(info.SafeMutableSlots))
	}
	if info.SafeMutableSlots[0] {
		t.Fatal("slot passed as call argument should be unsafe for transient mutation")
	}
	if !info.SafeMutableSlots[1] {
		t.Fatal("unused call function slot should remain safe")
	}
}

func TestEscapeAnalysisTracksStringBuilderSlots(t *testing.T) {
	prog := (&IRProgram{
		code: []byte{
			irLoadSlot, 0, 0,
			irLoadSlot, 0, 1,
			irStr2,
			irReturn,
		},
		numSlots: 2,
	}).refreshModel()
	info := analyzeEscapes(prog)
	if !info.StringBuilderSlots[0] {
		t.Fatal("left str operand slot should be marked append-builder candidate")
	}
	if !info.StringPrependSlots[1] {
		t.Fatal("right str operand slot should be marked prepend-builder candidate")
	}
}
