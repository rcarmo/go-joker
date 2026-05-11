package runtime

import (
	"fmt"
	"runtime"
	"time"

	. "github.com/rcarmo/go-joker/core"
)

// --- Disassemble ---

var procDisassemble ProcFn = func(args []Object) Object {
	fn := ensureArgIsFnLocal(args, 0)
	prog := IrCompileFn(fn)
	if prog == nil {
		return MakeString("; function cannot be compiled to IR")
	}
	return MakeString(IrDisassemble(prog))
}

// --- Profile ---

var procProfile ProcFn = func(args []Object) Object {
	callable := EnsureArgIsCallable(args, 0)
	iterations := 1
	if len(args) > 1 {
		iterations = ensureArgIsIntLocal(args, 1)
	}

	// Force GC before profiling
	runtime.GC()

	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	start := time.Now()
	var result Object
	for i := 0; i < iterations; i++ {
		result = callable.Call(nil)
	}
	elapsed := time.Since(start)

	runtime.ReadMemStats(&memAfter)
	allocs := memAfter.Mallocs - memBefore.Mallocs
	bytes := memAfter.TotalAlloc - memBefore.TotalAlloc

	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("time-ns"), Int{I: int(elapsed.Nanoseconds() / int64(iterations))})
	m = assocM(m, MakeKeyword("time-ms"), Double{D: float64(elapsed.Milliseconds()) / float64(iterations)})
	m = assocM(m, MakeKeyword("allocs"), Int{I: int(allocs / uint64(iterations))})
	m = assocM(m, MakeKeyword("bytes"), Int{I: int(bytes / uint64(iterations))})
	m = assocM(m, MakeKeyword("iterations"), Int{I: iterations})
	m = assocM(m, MakeKeyword("result"), result)
	return m
}

// --- WASM Diagnostic ---

var procWasmDiagnostic ProcFn = func(args []Object) Object {
	fn := ensureArgIsFnLocal(args, 0)
	prog := IrCompileFn(fn)
	if prog == nil {
		m := EmptyArrayMap()
		m = assocM(m, MakeKeyword("eligible"), Boolean{B: false})
		m = assocM(m, MakeKeyword("reason"), MakeString("cannot compile to IR"))
		return m
	}
	diag := ExplainWASMEligibility(prog)
	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("eligible"), Boolean{B: diag.Reason == ""})
	if diag.Reason != "" {
		m = assocM(m, MakeKeyword("reason"), MakeString(diag.Reason))
	}
	m = assocM(m, MakeKeyword("uses-float"), Boolean{B: diag.UsesFloat})
	m = assocM(m, MakeKeyword("has-imports"), Boolean{B: diag.HasImports})
	return m
}

// --- IR Analysis ---

var procAnalyze ProcFn = func(args []Object) Object {
	fn := ensureArgIsFnLocal(args, 0)
	prog := IrCompileFn(fn)
	if prog == nil {
		m := EmptyArrayMap()
		m = assocM(m, MakeKeyword("compiled"), Boolean{B: false})
		return m
	}
	a := AnalyzeIRProgramExported(prog)
	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("compiled"), Boolean{B: true})
	m = assocM(m, MakeKeyword("slots"), Int{I: prog.NumSlots()})
	m = assocM(m, MakeKeyword("code-bytes"), Int{I: prog.CodeLen()})
	m = assocM(m, MakeKeyword("captures"), Int{I: len(prog.CaptureSlots())})
	m = assocM(m, MakeKeyword("self-recursive"), Boolean{B: prog.HasSelf()})
	m = assocM(m, MakeKeyword("eligible-typed"), Boolean{B: a.Eligible})
	m = assocM(m, MakeKeyword("has-call-slot"), Boolean{B: a.HasCallSlot})
	m = assocM(m, MakeKeyword("has-self-call"), Boolean{B: a.HasSelfCall})
	m = assocM(m, MakeKeyword("uses-collection"), Boolean{B: a.UsesCollection})
	m = assocM(m, MakeKeyword("uses-string"), Boolean{B: a.UsesString})
	m = assocM(m, MakeKeyword("has-map-ops"), Boolean{B: a.HasMapOps})
	m = assocM(m, MakeKeyword("has-assoc"), Boolean{B: a.HasAssoc})
	m = assocM(m, MakeKeyword("has-generic-nth"), Boolean{B: a.HasGenericNth})
	if prog.GetNativeHelper() != nil {
		m = assocM(m, MakeKeyword("path"), MakeString("native-f64"))
	} else if a.Eligible {
		m = assocM(m, MakeKeyword("path"), MakeString("typed-ir"))
	} else {
		m = assocM(m, MakeKeyword("path"), MakeString("boxed-ir"))
	}
	return m
}

// --- Escape Analysis ---

var procEscapeAnalysis ProcFn = func(args []Object) Object {
	fn := ensureArgIsFnLocal(args, 0)
	prog := IrCompileFn(fn)
	if prog == nil {
		return NIL
	}
	info := AnalyzeEscapesExported(prog)
	slots := make([]Object, len(info))
	for i, safe := range info {
		slots[i] = Boolean{B: safe}
	}
	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("safe-mutable-slots"), NewVectorFrom(slots...))
	m = assocM(m, MakeKeyword("num-slots"), Int{I: prog.NumSlots()})
	return m
}

// --- Memory stats ---

var procMemStats ProcFn = func(args []Object) Object {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("heap-alloc-mb"), Double{D: float64(ms.HeapAlloc) / 1e6})
	m = assocM(m, MakeKeyword("heap-objects"), Int{I: int(ms.HeapObjects)})
	m = assocM(m, MakeKeyword("gc-cycles"), Int{I: int(ms.NumGC)})
	m = assocM(m, MakeKeyword("total-alloc-mb"), Double{D: float64(ms.TotalAlloc) / 1e6})
	m = assocM(m, MakeKeyword("goroutines"), Int{I: runtime.NumGoroutine()})
	return m
}

// --- GC ---

var procGC ProcFn = func(args []Object) Object {
	runtime.GC()
	return NIL
}

// --- Benchmark helper ---

var procBenchmark ProcFn = func(args []Object) Object {
	callable := EnsureArgIsCallable(args, 0)
	// Warm up a little to reduce one-off effects.
	for i := 0; i < 3; i++ {
		callable.Call(nil)
	}

	// Calibrate by increasing iteration count until a single measurement window
	// is long enough to be stable.
	runtime.GC()
	n := 1
	var elapsed time.Duration
	const target = 250 * time.Millisecond
	const maxIters = 100_000_000
	for {
		start := time.Now()
		for i := 0; i < n; i++ {
			callable.Call(nil)
		}
		elapsed = time.Since(start)
		if elapsed >= target || n >= maxIters {
			break
		}
		if elapsed < 10*time.Millisecond {
			n *= 10
		} else {
			n *= 2
		}
		if n > maxIters {
			n = maxIters
		}
	}
	if n <= 0 {
		n = 1
	}
	nsPerOp := elapsed.Nanoseconds() / int64(n)
	if nsPerOp < 0 {
		nsPerOp = 0
	}

	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("ns-per-op"), Int{I: int(nsPerOp)})
	m = assocM(m, MakeKeyword("ms-per-op"), Double{D: float64(nsPerOp) / 1e6})
	m = assocM(m, MakeKeyword("iterations"), Int{I: n})
	m = assocM(m, MakeKeyword("total-ms"), Double{D: float64(elapsed.Milliseconds())})
	return m
}

// --- Helpers ---

func ensureArgIsFnLocal(args []Object, idx int) *Fn {
	fn, ok := args[idx].(*Fn)
	if !ok {
		panic(RT.NewError(fmt.Sprintf("Expected function, got %s", args[idx].GetType().ToString(false))))
	}
	return fn
}

func ensureArgIsIntLocal(args []Object, idx int) int {
	switch v := args[idx].(type) {
	case Int:
		return v.I
	default:
		panic(RT.NewError("Expected integer"))
	}
}

func assocM(m *ArrayMap, k, v Object) *ArrayMap {
	result := m.Assoc(k, v)
	if am, ok := result.(*ArrayMap); ok {
		return am
	}
	// Shouldn't happen for small maps but handle gracefully
	return m
}
