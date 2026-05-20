package runtime

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type atomTestCallable func([]coretypes.Object) coretypes.Object

func (f atomTestCallable) Call(args []coretypes.Object) coretypes.Object { return f(args) }

func TestAtomSwapResetAndCompareAndSet(t *testing.T) {
	a := NewAtom(coretypes.Int{I: 1}, nil)

	oldValue, newValue := a.Swap(atomTestCallable(func(args []coretypes.Object) coretypes.Object {
		if got := args[0].(coretypes.Int).I; got != 1 {
			t.Fatalf("swap current value = %d, want 1", got)
		}
		if got := args[1].(coretypes.Int).I; got != 2 {
			t.Fatalf("swap arg = %d, want 2", got)
		}
		return coretypes.Int{I: 3}
	}), []coretypes.Object{coretypes.Int{I: 2}}, nil)
	if oldValue.(coretypes.Int).I != 1 || newValue.(coretypes.Int).I != 3 {
		t.Fatalf("swap returned (%s, %s), want (1, 3)", oldValue.ToString(false), newValue.ToString(false))
	}
	if got := a.Deref().(coretypes.Int).I; got != 3 {
		t.Fatalf("atom after swap = %d, want 3", got)
	}

	oldValue = a.Reset(coretypes.Int{I: 4}, nil)
	if oldValue.(coretypes.Int).I != 3 {
		t.Fatalf("reset old value = %s, want 3", oldValue.ToString(false))
	}
	if got := a.Deref().(coretypes.Int).I; got != 4 {
		t.Fatalf("atom after reset = %d, want 4", got)
	}

	oldValue, ok := a.CompareAndSet(coretypes.Int{I: 4}, coretypes.Int{I: 5}, nil)
	if !ok || oldValue.(coretypes.Int).I != 4 {
		t.Fatalf("compare-and-set success = (%s, %v), want (4, true)", oldValue.ToString(false), ok)
	}
	if got := a.Deref().(coretypes.Int).I; got != 5 {
		t.Fatalf("atom after compare-and-set = %d, want 5", got)
	}
	if oldValue, ok := a.CompareAndSet(coretypes.Int{I: 4}, coretypes.Int{I: 6}, nil); ok || oldValue != nil {
		t.Fatalf("compare-and-set stale = (%v, %v), want (nil, false)", oldValue, ok)
	}
}

func TestAtomValidationPreventsMutation(t *testing.T) {
	a := NewAtom(coretypes.Int{I: 1}, nil)
	validateErr := coretypes.MakeString("invalid")
	defer func() {
		if r := recover(); r != validateErr {
			t.Fatalf("panic = %v, want validator sentinel", r)
		}
		if got := a.Deref().(coretypes.Int).I; got != 1 {
			t.Fatalf("atom mutated after validation failure: %d", got)
		}
	}()
	a.Reset(coretypes.Int{I: 2}, func(coretypes.Object) { panic(validateErr) })
}
