package core_test

import (
	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"
	"strings"
	"testing"
)

// VM microbenchmarks for call overhead, closure recursion, and allocation retention.
// These track regressions in hot calling paths.

// evalBenchForms parses and evaluates multiple top-level forms (e.g. defn + call).
func evalBenchForms(tb testing.TB, code string) coretypes.Object {
	tb.Helper()
	reader := NewReader(strings.NewReader(code), "<bench>")
	var result coretypes.Object
	for {
		obj, err := TryRead(reader)
		if err == io.EOF {
			break
		}
		if err != nil {
			tb.Fatalf("read: %v", err)
		}
		expr, err := TryParse(obj, &ParseContext{GlobalEnv: GLOBAL_ENV})
		if err != nil {
			tb.Fatalf("parse: %v", err)
		}
		result = Eval(expr, nil)
	}
	return result
}

// compileBenchForm compiles the last form; earlier forms are evaluated for side effects (defn).
func compileBenchMulti(tb testing.TB, setup string, bench string) Expr {
	tb.Helper()
	evalBenchForms(tb, setup)
	return compileBenchExpr(tb, bench)
}

// --- Call overhead ---

func BenchmarkCallProc0(b *testing.B) {
	expr := compileBenchMulti(b,
		`(defn noop [] nil)`,
		`(noop)`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCallProc1(b *testing.B) {
	expr := compileBenchMulti(b,
		`(defn ident [x] x)`,
		`(ident 42)`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCallProc2(b *testing.B) {
	expr := compileBenchMulti(b,
		`(defn add2 [a b] (+ a b))`,
		`(add2 1 2)`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

func BenchmarkCallProc3(b *testing.B) {
	expr := compileBenchMulti(b,
		`(defn add3 [a b c] (+ a (+ b c)))`,
		`(add3 1 2 3)`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

// --- Recursive fib (native codegen) ---

func BenchmarkFib10(b *testing.B) {
	expr := compileBenchMulti(b,
		`(defn fib [n] (if (<= n 1) n (+ (fib (- n 1)) (fib (- n 2)))))`,
		`(fib 10)`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

func BenchmarkFib20(b *testing.B) {
	expr := compileBenchMulti(b,
		`(defn fib [n] (if (<= n 1) n (+ (fib (- n 1)) (fib (- n 2)))))`,
		`(fib 20)`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

// --- Tak (triple recursion, native codegen) ---

func BenchmarkTak(b *testing.B) {
	expr := compileBenchMulti(b,
		`(defn tak [x y z] (if (< y x) (tak (tak (dec x) y z) (tak (dec y) z x) (tak (dec z) x y)) z))`,
		`(tak 18 12 6)`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

// --- Loop/recur (tail-call) ---

func BenchmarkLoopRecur1M(b *testing.B) {
	expr := compileBenchExpr(b,
		`(loop [i 0 s 0] (if (= i 1000000) s (recur (inc i) (+ s i))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

// --- Reduce over IntRange ---

func BenchmarkReduceRange10K(b *testing.B) {
	expr := compileBenchExpr(b,
		`(reduce + 0 (range 10000))`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

// --- Closure capture ---

func BenchmarkClosureCapture(b *testing.B) {
	expr := compileBenchMulti(b,
		`(defn make-adder [x] (fn [y] (+ x y))) (def add5 (make-adder 5))`,
		`(add5 10)`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

// --- Map assoc ---

func BenchmarkMapAssoc100(b *testing.B) {
	expr := compileBenchExpr(b,
		`(loop [i 0 m {}] (if (= i 100) m (recur (inc i) (assoc m i (* i i)))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

// --- Vector conj ---

func BenchmarkVectorConj100(b *testing.B) {
	expr := compileBenchExpr(b,
		`(loop [i 0 v []] (if (= i 100) v (recur (inc i) (conj v i))))`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}

// --- Transducer pipeline ---

func BenchmarkTransducePipeline(b *testing.B) {
	expr := compileBenchExpr(b,
		`(transduce (comp (map inc) (filter even?) (take 50)) + 0 (range 200))`)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_ = Eval(expr, nil)
	}
}
