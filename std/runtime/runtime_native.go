package runtime

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math/big"
	"runtime"
	"time"

	. "github.com/rcarmo/go-joker/core"
)

// --- Disassemble ---

var procDisassemble ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	fn := ensureArgIsFnLocal(args, 0)
	prog := IrCompileFn(fn)
	if prog == nil {
		return coretypes.MakeString("; function cannot be compiled to IR")
	}
	return coretypes.MakeString(IrDisassemble(prog))
}

// --- Profile ---

var procProfile ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 2)
	callable := EnsureArgIsCallable(args, 0)
	iterations := 1
	if len(args) > 1 {
		iterations = ensureArgIsIntLocal(args, 1)
	}
	if iterations <= 0 {
		panic(RT.NewError("profile iterations must be positive"))
	}

	// Force GC before profiling
	runtime.GC()

	var memBefore, memAfter runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	start := time.Now()
	var result coretypes.Object
	for i := 0; i < iterations; i++ {
		result = callable.Call(nil)
	}
	elapsed := time.Since(start)

	runtime.ReadMemStats(&memAfter)
	allocs := memAfter.Mallocs - memBefore.Mallocs
	bytes := memAfter.TotalAlloc - memBefore.TotalAlloc

	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("time-ns"), runtimeIntObject(elapsed.Nanoseconds()/int64(iterations)))
	m = assocM(m, MakeKeyword("time-ms"), coretypes.Double{D: float64(elapsed.Milliseconds()) / float64(iterations)})
	m = assocM(m, MakeKeyword("allocs"), runtimeUintObject(allocs/uint64(iterations)))
	m = assocM(m, MakeKeyword("bytes"), runtimeUintObject(bytes/uint64(iterations)))
	m = assocM(m, MakeKeyword("iterations"), coretypes.Int{I: iterations})
	m = assocM(m, MakeKeyword("result"), result)
	return m
}

// --- WASM Diagnostic ---

var procWasmDiagnostic ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	fn := ensureArgIsFnLocal(args, 0)
	prog := IrCompileFn(fn)
	if prog == nil {
		m := EmptyArrayMap()
		m = assocM(m, MakeKeyword("eligible"), coretypes.Boolean{B: false})
		m = assocM(m, MakeKeyword("reason"), coretypes.MakeString("cannot compile to IR"))
		return m
	}
	diag := ExplainWASMEligibility(prog)
	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("eligible"), coretypes.Boolean{B: diag.Reason == ""})
	if diag.Reason != "" {
		m = assocM(m, MakeKeyword("reason"), coretypes.MakeString(diag.Reason))
	}
	m = assocM(m, MakeKeyword("uses-float"), coretypes.Boolean{B: diag.UsesFloat})
	m = assocM(m, MakeKeyword("has-imports"), coretypes.Boolean{B: diag.HasImports})
	return m
}

// --- IR Analysis ---

var procAnalyze ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	fn := ensureArgIsFnLocal(args, 0)
	prog := IrCompileFn(fn)
	if prog == nil {
		m := EmptyArrayMap()
		m = assocM(m, MakeKeyword("compiled"), coretypes.Boolean{B: false})
		return m
	}
	a := AnalyzeIRProgramExported(prog)
	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("compiled"), coretypes.Boolean{B: true})
	m = assocM(m, MakeKeyword("slots"), coretypes.Int{I: prog.NumSlots()})
	m = assocM(m, MakeKeyword("code-bytes"), coretypes.Int{I: prog.CodeLen()})
	m = assocM(m, MakeKeyword("captures"), coretypes.Int{I: len(prog.CaptureSlots())})
	m = assocM(m, MakeKeyword("self-recursive"), coretypes.Boolean{B: prog.HasSelf()})
	m = assocM(m, MakeKeyword("eligible-typed"), coretypes.Boolean{B: a.Eligible})
	m = assocM(m, MakeKeyword("has-call-slot"), coretypes.Boolean{B: a.HasCallSlot})
	m = assocM(m, MakeKeyword("has-self-call"), coretypes.Boolean{B: a.HasSelfCall})
	m = assocM(m, MakeKeyword("uses-collection"), coretypes.Boolean{B: a.UsesCollection})
	m = assocM(m, MakeKeyword("uses-string"), coretypes.Boolean{B: a.UsesString})
	m = assocM(m, MakeKeyword("has-map-ops"), coretypes.Boolean{B: a.HasMapOps})
	m = assocM(m, MakeKeyword("has-assoc"), coretypes.Boolean{B: a.HasAssoc})
	m = assocM(m, MakeKeyword("has-generic-nth"), coretypes.Boolean{B: a.HasGenericNth})
	if prog.GetNativeHelper() != nil {
		m = assocM(m, MakeKeyword("path"), coretypes.MakeString("native-f64"))
	} else if a.Eligible {
		m = assocM(m, MakeKeyword("path"), coretypes.MakeString("typed-ir"))
	} else {
		m = assocM(m, MakeKeyword("path"), coretypes.MakeString("boxed-ir"))
	}
	return m
}

// --- Escape Analysis ---

var procEscapeAnalysis ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	fn := ensureArgIsFnLocal(args, 0)
	prog := IrCompileFn(fn)
	if prog == nil {
		return NIL
	}
	info := AnalyzeEscapesExported(prog)
	slots := make([]coretypes.Object, len(info))
	for i, safe := range info {
		slots[i] = coretypes.Boolean{B: safe}
	}
	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("safe-mutable-slots"), NewVectorFrom(slots...))
	m = assocM(m, MakeKeyword("num-slots"), coretypes.Int{I: prog.NumSlots()})
	return m
}

// --- Memory stats ---

var procMemStats ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 0, 0)
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m := EmptyArrayMap()
	m = assocM(m, MakeKeyword("heap-alloc-mb"), coretypes.Double{D: float64(ms.HeapAlloc) / 1e6})
	m = assocM(m, MakeKeyword("heap-objects"), runtimeUintObject(ms.HeapObjects))
	m = assocM(m, MakeKeyword("gc-cycles"), runtimeUintObject(uint64(ms.NumGC)))
	m = assocM(m, MakeKeyword("total-alloc-mb"), coretypes.Double{D: float64(ms.TotalAlloc) / 1e6})
	m = assocM(m, MakeKeyword("goroutines"), coretypes.Int{I: runtime.NumGoroutine()})
	return m
}

// --- GC ---

var procGC ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 0, 0)
	runtime.GC()
	return NIL
}

// --- Benchmark helper ---

var procBenchmark ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
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
	m = assocM(m, MakeKeyword("ns-per-op"), runtimeIntObject(nsPerOp))
	m = assocM(m, MakeKeyword("ms-per-op"), coretypes.Double{D: float64(nsPerOp) / 1e6})
	m = assocM(m, MakeKeyword("iterations"), coretypes.Int{I: n})
	m = assocM(m, MakeKeyword("total-ms"), coretypes.Double{D: float64(elapsed.Milliseconds())})
	return m
}

// --- Helpers ---

func runtimeIntObject(n int64) coretypes.Object {
	maxNativeInt := int64(int(^uint(0) >> 1))
	minNativeInt := -maxNativeInt - 1
	if n > maxNativeInt || n < minNativeInt {
		return coretypes.MakeBigInt(big.NewInt(n))
	}
	return coretypes.MakeInt(int(n))
}

func runtimeUintObject(n uint64) coretypes.Object {
	maxNativeUint := uint64(int(^uint(0) >> 1))
	if n > maxNativeUint {
		return coretypes.MakeBigInt(new(big.Int).SetUint64(n))
	}
	return coretypes.MakeInt(int(n))
}

func ensureArgIsFnLocal(args []coretypes.Object, idx int) *Fn {
	fn, ok := args[idx].(*Fn)
	if !ok {
		panic(RT.NewError(fmt.Sprintf("Expected function, got %s", args[idx].GetType().ToString(false))))
	}
	return fn
}

func ensureArgIsIntLocal(args []coretypes.Object, idx int) int {
	switch v := args[idx].(type) {
	case coretypes.Int:
		return v.I
	default:
		panic(RT.NewError("Expected integer"))
	}
}

func assocM(m *ArrayMap, k, v coretypes.Object) *ArrayMap {
	result := m.Assoc(k, v)
	if am, ok := result.(*ArrayMap); ok {
		return am
	}
	// Shouldn't happen for small maps but handle gracefully
	return m
}
