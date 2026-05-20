package runtime

import (
	"fmt"
	"math/big"
	"runtime"
	"time"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

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
	callable := coretypes.EnsureArgIsCallable(args, 0)
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

	m := corecollections.EmptyArrayMap()
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "time-ns"), runtimeIntObject(elapsed.Nanoseconds()/int64(iterations)))
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "time-ms"), coretypes.Double{D: float64(elapsed.Milliseconds()) / float64(iterations)})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "allocs"), runtimeUintObject(allocs/uint64(iterations)))
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "bytes"), runtimeUintObject(bytes/uint64(iterations)))
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "iterations"), coretypes.Int{I: iterations})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "result"), result)
	return m
}

// --- WASM Diagnostic ---

var procWasmDiagnostic ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	fn := ensureArgIsFnLocal(args, 0)
	prog := IrCompileFn(fn)
	if prog == nil {
		m := corecollections.EmptyArrayMap()
		m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "eligible"), coretypes.Boolean{B: false})
		m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "reason"), coretypes.MakeString("cannot compile to IR"))
		return m
	}
	diag := ExplainWASMEligibility(prog)
	m := corecollections.EmptyArrayMap()
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "eligible"), coretypes.Boolean{B: diag.Reason == ""})
	if diag.Reason != "" {
		m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "reason"), coretypes.MakeString(diag.Reason))
	}
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "uses-float"), coretypes.Boolean{B: diag.UsesFloat})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "has-imports"), coretypes.Boolean{B: diag.HasImports})
	return m
}

// --- IR Analysis ---

var procAnalyze ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	fn := ensureArgIsFnLocal(args, 0)
	prog := IrCompileFn(fn)
	if prog == nil {
		m := corecollections.EmptyArrayMap()
		m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "compiled"), coretypes.Boolean{B: false})
		return m
	}
	a := AnalyzeIRProgramExported(prog)
	m := corecollections.EmptyArrayMap()
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "compiled"), coretypes.Boolean{B: true})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "slots"), coretypes.Int{I: prog.NumSlots()})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "code-bytes"), coretypes.Int{I: prog.CodeLen()})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "captures"), coretypes.Int{I: len(prog.CaptureSlots())})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "self-recursive"), coretypes.Boolean{B: prog.HasSelf()})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "eligible-typed"), coretypes.Boolean{B: a.Eligible})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "has-call-slot"), coretypes.Boolean{B: a.HasCallSlot})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "has-self-call"), coretypes.Boolean{B: a.HasSelfCall})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "uses-collection"), coretypes.Boolean{B: a.UsesCollection})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "uses-string"), coretypes.Boolean{B: a.UsesString})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "has-map-ops"), coretypes.Boolean{B: a.HasMapOps})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "has-assoc"), coretypes.Boolean{B: a.HasAssoc})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "has-generic-nth"), coretypes.Boolean{B: a.HasGenericNth})
	if prog.GetNativeHelper() != nil {
		m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "path"), coretypes.MakeString("native-f64"))
	} else if a.Eligible {
		m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "path"), coretypes.MakeString("typed-ir"))
	} else {
		m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "path"), coretypes.MakeString("boxed-ir"))
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
	m := corecollections.EmptyArrayMap()
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "safe-mutable-slots"), corecollections.NewVectorFrom(slots...))
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "num-slots"), coretypes.Int{I: prog.NumSlots()})
	return m
}

// --- Memory stats ---

var procMemStats ProcFn = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 0, 0)
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	m := corecollections.EmptyArrayMap()
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "heap-alloc-mb"), coretypes.Double{D: float64(ms.HeapAlloc) / 1e6})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "heap-objects"), runtimeUintObject(ms.HeapObjects))
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "gc-cycles"), runtimeUintObject(uint64(ms.NumGC)))
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "total-alloc-mb"), coretypes.Double{D: float64(ms.TotalAlloc) / 1e6})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "goroutines"), coretypes.Int{I: runtime.NumGoroutine()})
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
	callable := coretypes.EnsureArgIsCallable(args, 0)
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

	m := corecollections.EmptyArrayMap()
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "ns-per-op"), runtimeIntObject(nsPerOp))
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "ms-per-op"), coretypes.Double{D: float64(nsPerOp) / 1e6})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "iterations"), coretypes.Int{I: n})
	m = assocM(m, coretypes.MakeKeyword(STRINGS.Intern, "total-ms"), coretypes.Double{D: float64(elapsed.Milliseconds())})
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

func assocM(m *corecollections.ArrayMap, k, v coretypes.Object) *corecollections.ArrayMap {
	result := m.Assoc(k, v)
	if am, ok := result.(*corecollections.ArrayMap); ok {
		return am
	}
	// Shouldn't happen for small maps but handle gracefully
	return m
}
