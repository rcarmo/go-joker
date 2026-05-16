package core_test

import (
	. "github.com/rcarmo/go-joker/core"
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
			return Char{Ch: r}
		}},
		{"cursor-next", func(args []Object) Object { return args[0].(*StringCursor).Next() }},
		{"cursor-done?", func(args []Object) Object { return Boolean{B: args[0].(*StringCursor).Done()} }},
	} {
		sym := MakeSymbol(p.name)
		vr := ns.Intern(sym)
		vr.Value = Proc{Name: "bench-" + p.name, Fn: p.fn}
		GLOBAL_ENV.CurrentNamespace().Intern(sym).Value = vr.Value
	}
}

// Benchmark comparing string iteration with nth vs cursor
func BenchmarkStringIterNth(b *testing.B) {
	// Count chars in string using (loop [i 0 c 0] (if (= i len) c (recur (+ i 1) (+ c 1))))
	// with nth to verify each char exists
	script := `(let [s "The quick brown fox jumps over the lazy dog and does many other things"
                  len (count s)]
              (loop [i 0 c 0]
                (if (= i len)
                  c
                  (let [ch (nth s i)]
                    (recur (+ i 1) (if (= ch \space) (+ c 1) c))))))`
	expr := compileBenchExpr(b, script)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Eval(expr, nil)
	}
}

func BenchmarkStringIterCursor(b *testing.B) {
	initBenchStringCursorProcs()
	// Same but with cursor
	script := `(let [s "The quick brown fox jumps over the lazy dog and does many other things"
                  cur (string-cursor s)]
              (loop [c cur spaces 0]
                (if (cursor-done? c)
                  spaces
                  (let [ch (cursor-char c)]
                    (recur (cursor-next c) (if (= ch \space) (+ spaces 1) spaces))))))`
	expr := compileBenchExpr(b, script)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Eval(expr, nil)
	}
}
