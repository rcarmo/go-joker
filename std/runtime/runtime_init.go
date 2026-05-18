package runtime

import (
	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

var runtimeNamespace = GLOBAL_ENV.EnsureSymbolIsLib(coretypes.MakeSymbol(STRINGS.Intern, "joker.runtime"))

func init() {
	runtimeNamespace.Lazy = initRuntimeNamespace
}

func initRuntimeNamespace() {
	runtimeNamespace.ResetMeta(MakeMeta(nil, "Runtime introspection: IR disassembly, profiling, WASM diagnostics, escape analysis, memory stats.", "1.0"))

	procs := []struct {
		name string
		fn   ProcFn
		doc  string
	}{
		{"disassemble", procDisassemble, "Returns IR bytecode disassembly of a function as a string."},
		{"analyze", procAnalyze, "Returns IR analysis map: slots, captures, eligibility, path, opcodes."},
		{"wasm-diagnostic", procWasmDiagnostic, "Returns WASM eligibility diagnostic: reason, uses-float, has-imports."},
		{"escape-analysis", procEscapeAnalysis, "Returns escape analysis: which slots are safe for transient promotion."},
		{"profile", procProfile, "Profiles a zero-arg fn: returns {:time-ms :allocs :bytes :result}. Optional iterations arg."},
		{"benchmark", procBenchmark, "Auto-calibrating benchmark of a zero-arg fn: returns {:ms-per-op :ns-per-op :iterations}."},
		{"mem-stats", procMemStats, "Returns current memory stats: heap, objects, GC cycles."},
		{"gc", procGC, "Forces garbage collection."},
	}

	for _, p := range procs {
		runtimeNamespace.InternVar(p.name, Proc{Fn: p.fn, Name: "runtime/" + p.name},
			MakeMeta(nil, p.doc, "1.0"))
	}
}
