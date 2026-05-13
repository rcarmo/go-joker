package ir

import "testing"

func TestFrameStackPushPop(t *testing.T) {
	fs := NewFrameStack[int](4)
	defer ReleaseFrameStack(fs)
	slots := []int{1, 2, 3, 4}
	fs.Push(7, slots, 2)
	for i := range slots {
		slots[i] = 0
	}
	pc, stackLen := fs.Pop(slots)
	if pc != 7 || stackLen != 2 {
		t.Fatalf("Pop() = (%d,%d)", pc, stackLen)
	}
	if slots[0] != 1 || slots[3] != 4 {
		t.Fatalf("restored slots = %#v", slots)
	}
}
