package runtime

import "testing"

func TestGoIDIsPositive(t *testing.T) {
	if id := GoID(); id <= 0 {
		t.Fatalf("GoID() = %d, want > 0", id)
	}
}
