package bufferpool

import "testing"

func TestGetPutBuffer(t *testing.T) {
	b := Get()
	b.WriteString("hello")
	Put(b)

	b2 := Get()
	if got := b2.Len(); got != 0 {
		t.Fatalf("Len after reuse = %d, want 0", got)
	}
	Put(b2)
}
