package core_test

import (
	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"testing"
)

func initBenchStringCursorProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	for _, p := range []struct {
		name string
		fn   ProcFn
	}{
		{"string-cursor", func(args []Object) Object { return NewStringCursor(EnsureArgIsString(args, 0).S) }},
		{"cursor-char", func(args []Object) Object {
			c := args[0].(*StringCursor)
			r := c.Char()
			if r < 0 {
				return NIL
			}
			return coretypes.Char{Ch: r}
		}},
		{"cursor-next", func(args []Object) Object { return args[0].(*StringCursor).Next() }},
		{"cursor-done?", func(args []Object) Object { return coretypes.Boolean{B: args[0].(*StringCursor).Done()} }},
	} {
		sym := MakeSymbol(p.name)
		vr := ns.Intern(sym)
		vr.Value = Proc{Name: "bench-" + p.name, Fn: p.fn}
		GLOBAL_ENV.CurrentNamespace().Intern(sym).Value = vr.Value
	}
}

// Benchmark comparing string iteration with nth vs cursor
func BenchmarkStringIterNth(b *testing.B) {
	// Count chars in string using nth to verify each char exists.
	expr := compileBenchExpr(b, stringIterNthScript)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Eval(expr, nil)
	}
}

func BenchmarkStringIterCursor(b *testing.B) {
	initBenchStringCursorProcs()
	expr := compileBenchExpr(b, stringIterCursorScript)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Eval(expr, nil)
	}
}
