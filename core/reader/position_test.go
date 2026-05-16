package reader

import "testing"

func TestPositionStackPushPop(t *testing.T) {
	stack := NewPositionStack(1)
	if stack.Len() != 0 {
		t.Fatalf("new stack length = %d, want 0", stack.Len())
	}
	stack.Push(Position{Line: 1, Column: 2})
	stack.Push(Position{Line: 3, Column: 4})
	if stack.Len() != 2 {
		t.Fatalf("stack length = %d, want 2", stack.Len())
	}
	p, ok := stack.Pop()
	if !ok || p != (Position{Line: 3, Column: 4}) {
		t.Fatalf("first pop = %#v, %v; want line 3 column 4", p, ok)
	}
	p, ok = stack.Pop()
	if !ok || p != (Position{Line: 1, Column: 2}) {
		t.Fatalf("second pop = %#v, %v; want line 1 column 2", p, ok)
	}
	if _, ok := stack.Pop(); ok {
		t.Fatal("empty pop succeeded")
	}
}

func TestNewPositionStackNegativeCapacity(t *testing.T) {
	stack := NewPositionStack(-1)
	stack.Push(Position{Line: 5, Column: 6})
	p, ok := stack.Pop()
	if !ok || p.Line != 5 || p.Column != 6 {
		t.Fatalf("pop after negative-capacity constructor = %#v, %v", p, ok)
	}
}
