package runtime

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"strings"
	"testing"
	"time"

	. "github.com/rcarmo/go-joker/core"
)

func TestDisassemble(t *testing.T) {
	// Create a simple fn via eval
	script := `(fn [x y] (+ (* x x) (* y y)))`
	reader := NewReader(strings.NewReader(script), "<test>")
	obj, _ := TryRead(reader)
	expr, _ := TryParse(obj, &ParseContext{GlobalEnv: GLOBAL_ENV})
	fnObj := Eval(expr, nil).(*Fn)

	result := procDisassemble([]coretypes.Object{fnObj})
	dis := result.(coretypes.String).S
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

	result := procAnalyze([]coretypes.Object{fnObj})
	t.Logf("analyze: %s", result.ToString(false))
}

func TestBenchmarkFn(t *testing.T) {
	initRuntimeNamespace()
	// Benchmark a simple fn; this should complete quickly and never loop forever.
	counter := coretypes.Int{I: 0}
	fn := Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		counter.I++
		return counter
	}, Name: "test-fn"}

	resCh := make(chan coretypes.Object, 1)
	go func() {
		resCh <- procBenchmark([]coretypes.Object{fn})
	}()

	var result coretypes.Object
	select {
	case result = <-resCh:
	case <-time.After(5 * time.Second):
		t.Fatal("procBenchmark timed out")
	}

	am, ok := result.(*ArrayMap)
	if !ok {
		t.Fatalf("unexpected result type: %T", result)
	}
	if ok, v := am.Get(MakeKeyword("iterations")); !ok || v.(coretypes.Int).I <= 0 {
		t.Fatalf("invalid iterations in result: %s", result.ToString(false))
	}
	t.Logf("benchmark: %s", result.ToString(false))
}

func TestMemStats(t *testing.T) {
	initRuntimeNamespace()
	result := procMemStats(nil)
	t.Logf("mem-stats: %s", result.ToString(false))
}
