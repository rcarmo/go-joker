package collections

import "testing"

func TestBitmapHelpers(t *testing.T) {
	if got := BitCount(0); got != 0 {
		t.Fatalf("BitCount(0) = %d, want 0", got)
	}
	if got := BitCount(0b101101); got != 4 {
		t.Fatalf("BitCount(0b101101) = %d, want 4", got)
	}
	var hash uint32 = 0b101011_00010
	if got := HashMask(hash, 0); got != 2 {
		t.Fatalf("HashMask low = %d, want 2", got)
	}
	if got := HashMask(hash, 5); got != 0b101011&0x1f {
		t.Fatalf("HashMask shifted = %d, want %d", got, 0b101011&0x1f)
	}
	if got := Bitpos(2, 0); got != 4 {
		t.Fatalf("Bitpos(2,0) = %d, want 4", got)
	}
}
