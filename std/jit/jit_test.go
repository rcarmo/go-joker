package jit

import (
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func mkFn(code string) *Fn {
	reader := NewReader(strings.NewReader(code), "<test>")
	obj, _ := TryRead(reader)
	parseCtx := &ParseContext{GlobalEnv: GLOBAL_ENV}
	expr, _ := TryParse(obj, parseCtx)
	return Eval(expr, nil).(*Fn)
}

func TestJITCompile(t *testing.T) {
	Init()
	compiled := compile(mkFn("(fn [x y] (+ x y))"))
	result := compiled.(Callable).Call([]Object{Double{D: 3}, Double{D: 4}})
	if result.(Double).D != 7.0 {
		t.Fatalf("got %v, want 7.0", result)
	}
}

func TestJITInfo(t *testing.T) {
	Init()
	t.Logf("info: %s", info(mkFn("(fn [x y] (* x y))")).ToString(false))
}

func TestJITCompiled(t *testing.T) {
	Init()
	if !isCompiled(mkFn("(fn [x y] (+ x y))")) {
		t.Fatal("add should be compilable")
	}
}

func BenchmarkJITCompiled(b *testing.B) {
	Init()
	compiled := compile(mkFn("(fn [x y] (+ x y))")).(Callable)
	args := []Object{Double{D: 3}, Double{D: 4}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compiled.Call(args)
	}
}

func BenchmarkJITInterpreted(b *testing.B) {
	Init()
	fn := mkFn("(fn [x y] (+ x y))")
	args := []Object{Double{D: 3}, Double{D: 4}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn.Call(args)
	}
}
