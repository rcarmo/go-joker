package jit

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
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
	result := compiled.(coretypes.Callable).Call([]coretypes.Object{coretypes.Double{D: 3}, coretypes.Double{D: 4}})
	if result.(coretypes.Double).D != 7.0 {
		t.Fatalf("got %v, want 7.0", result)
	}
}

func TestJITCompileWASM(t *testing.T) {
	Init()
	compiled := compileWASM(mkFn("(fn [limit] (loop [i 0 acc 0] (if (= i limit) acc (recur (+ i 1) (+ acc (rem (* i 7) 11))))))"))
	result := compiled.(coretypes.Callable).Call([]coretypes.Object{coretypes.Int{I: 100}})
	if result.(coretypes.Int).I != 495 {
		t.Fatalf("got %v, want 495", result)
	}
}

func TestJITCompileWASMCapturedLoopLimit(t *testing.T) {
	Init()
	compiled := compileWASM(mkFn("(let [limit 100] (fn [] (loop [i 0 acc 0] (if (= i limit) acc (recur (+ i 1) (+ acc (rem (* i 7) 11)))))))"))
	result := compiled.(coretypes.Callable).Call(nil)
	if result.(coretypes.Int).I != 495 {
		t.Fatalf("got %v, want 495", result)
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
	compiled := compile(mkFn("(fn [x y] (+ x y))")).(coretypes.Callable)
	args := []coretypes.Object{coretypes.Double{D: 3}, coretypes.Double{D: 4}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		compiled.Call(args)
	}
}

func BenchmarkJITInterpreted(b *testing.B) {
	Init()
	fn := mkFn("(fn [x y] (+ x y))")
	args := []coretypes.Object{coretypes.Double{D: 3}, coretypes.Double{D: 4}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		fn.Call(args)
	}
}
