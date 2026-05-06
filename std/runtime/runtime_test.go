package runtime

import (
	"strings"
	"testing"

	. "github.com/candid82/joker/core"
)

func TestDisassemble(t *testing.T) {
	// Create a simple fn via eval
	script := `(fn [x y] (+ (* x x) (* y y)))`
	reader := NewReader(strings.NewReader(script), "<test>")
	obj, _ := TryRead(reader)
	expr, _ := TryParse(obj, &ParseContext{GlobalEnv: GLOBAL_ENV})
	fnObj := Eval(expr, nil).(*Fn)

	result := procDisassemble([]Object{fnObj})
	dis := result.(String).S
	if !strings.Contains(dis, "irMul") {
		t.Fatalf("expected irMul in disassembly, got:\n%s", dis)
	}
	if !strings.Contains(dis, "irAdd") {
		t.Fatalf("expected irAdd in disassembly, got:\n%s", dis)
	}
	t.Logf("disassembly:\n%s", dis)
}

func TestAnalyze(t *testing.T) {
	initRuntimeNamespace()
	script := `(fn [x y] (+ (* x x) (* y y)))`
	reader := NewReader(strings.NewReader(script), "<test>")
	obj, _ := TryRead(reader)
	expr, _ := TryParse(obj, &ParseContext{GlobalEnv: GLOBAL_ENV})
	fnObj := Eval(expr, nil).(*Fn)

	result := procAnalyze([]Object{fnObj})
	t.Logf("analyze: %s", result.ToString(false))
}

func TestBenchmarkFn(t *testing.T) {
	initRuntimeNamespace()
	// Benchmark a simple fn
	counter := Int{I: 0}
	fn := Proc{Fn: func(args []Object) Object {
		counter.I++
		return counter
	}, Name: "test-fn"}
	result := procBenchmark([]Object{fn})
	t.Logf("benchmark: %s", result.ToString(false))
}

func TestMemStats(t *testing.T) {
	initRuntimeNamespace()
	result := procMemStats(nil)
	t.Logf("mem-stats: %s", result.ToString(false))
}
