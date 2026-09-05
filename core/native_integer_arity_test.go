package core

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"testing"
)

func TestNativeIntegerArityMatchesInterpreter(t *testing.T) {
	for arity := 1; arity <= 3; arity++ {
		t.Run(fmt.Sprint(arity), func(t *testing.T) {
			params := []string{"x", "x y", "x y z"}[arity-1]
			body := []string{"(inc x)", "(+ x y)", "(+ (+ x y) z)"}[arity-1]
			fn := evalTestScript(t, fmt.Sprintf("(do (defn native-arity-fixture-%d [%s] %s) native-arity-fixture-%d)", arity, params, body, arity)).(*Fn)
			entry := tryNativeRecursive(fn)
			if entry == nil || entry.arity != arity {
				t.Fatal("native integer compilation was not selected")
			}
			valid := make([]coretypes.Object, arity)
			for i := range valid {
				valid[i] = coretypes.MakeInt(7)
			}
			expected := []int{8, 14, 21}[arity-1]
			requireInt(t, callNativeRecursive(entry, valid), expected)
			// Without defVar this straight-line body runs through evalLoop, not native/IR recursion.
			oracle := &Fn{fnExpr: fn.fnExpr, env: fn.env}
			oracle.defVar = nil
			requireInt(t, oracle.Call(valid), expected)
			for _, count := range []int{0, arity - 1, arity + 1} {
				args := make([]coretypes.Object, count)
				for i := range args {
					args[i] = coretypes.MakeInt(7)
				}
				capture := func(f *Fn) (failure any) {
					defer func() { failure = recover() }()
					f.Call(args)
					return nil
				}
				want, got := capture(oracle), capture(fn)
				if want == nil {
					t.Fatal("interpreter accepted invalid arity")
				}
				if fmt.Sprintf("%T:%v", got, got) != fmt.Sprintf("%T:%v", want, want) {
					t.Errorf("arity %d with %d arguments: native %T: %v; interpreter %T: %v", arity, count, got, got, want, want)
				}
			}
		})
	}
}
