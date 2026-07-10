package ir

import "testing"

func TestAnalyzeEscapesMarksCallArgumentsUnsafe(t *testing.T) {
	info := AnalyzeEscapes([]byte{
		LoadSlot, 0, 0,
		CallSlot, 0, 1, 0, 1,
		Return,
	}, 2)
	if len(info.SafeMutableSlots) != 2 {
		t.Fatalf("SafeMutableSlots len = %d, want 2", len(info.SafeMutableSlots))
	}
	if info.SafeMutableSlots[0] {
		t.Fatal("slot passed as call argument should be unsafe for transient mutation")
	}
	if !info.SafeMutableSlots[1] {
		t.Fatal("unused function slot should remain safe")
	}
}

func TestAnalyzeEscapesTracksStringBuilderSlots(t *testing.T) {
	info := AnalyzeEscapes([]byte{
		LoadSlot, 0, 0,
		LoadSlot, 0, 1,
		Str2,
		Return,
	}, 2)
	if !info.StringBuilderSlots[0] {
		t.Fatal("left string operand should be an append-builder candidate")
	}
	if !info.StringPrependSlots[1] {
		t.Fatal("right string operand should be a prepend-builder candidate")
	}
}

func TestAnalyzeEscapesUnknownOpcodeIsConservative(t *testing.T) {
	info := AnalyzeEscapes([]byte{255}, 3)
	for slot, safe := range info.SafeMutableSlots {
		if safe {
			t.Fatalf("unknown opcode left slot %d marked safe", slot)
		}
	}
}
