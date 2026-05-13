package numutil

import "testing"

func TestParseInt(t *testing.T) {
	got, err := ParseInt("42", 10, 64)
	if err != nil || got != 42 {
		t.Fatalf("ParseInt() = %d, %v", got, err)
	}
}

func TestParseFloat64(t *testing.T) {
	got, err := ParseFloat64("3.5")
	if err != nil || got != 3.5 {
		t.Fatalf("ParseFloat64() = %v, %v", got, err)
	}
}
