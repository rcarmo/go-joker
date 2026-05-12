package core

// wasm_runtime.go — wazero-based WASM execution engine.
// Compiles WASM modules and caches them. Handles Object ↔ WASM i64 conversion.

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// WasmProgram is a compiled, ready-to-execute WASM module.
type WasmProgram struct {
	mod        api.Module
	execFn     api.Function
	useFloat   bool
	hasImports bool
	constants  []Object // pre-stored constants for handle references
	bytes      []byte   // raw wasm module for export/debugging
}

var (
	wasmRT     wazero.Runtime
	wasmRTOnce sync.Once
	wasmCache  sync.Map // map[*IRProgram]*WasmProgram
	wasmFail   = &WasmProgram{}
	wasmModSeq uint64 // unique module name counter
	wasmModMu  sync.Mutex
)

func getWasmRT() wazero.Runtime {
	wasmRTOnce.Do(func() {
		cache := wazero.NewCompilationCache()
		wasmRT = wazero.NewRuntimeWithConfig(context.Background(),
			wazero.NewRuntimeConfig().WithCompilationCache(cache))
		// Register host functions for collection operations
		registerWasmHost(wasmRT)
	})
	return wasmRT
}

func nextWasmModName() string {
	wasmModMu.Lock()
	wasmModSeq++
	n := wasmModSeq
	wasmModMu.Unlock()
	return "joker_wasm_" + strconv.FormatUint(n, 10)
}

// wasmGetCached retrieves or compiles a WASM program for an IR program.
func wasmGetCached(prog *IRProgram) *WasmProgram {
	if v, ok := wasmCache.Load(prog); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFail {
			return nil
		}
		return wp
	}
	wp := wasmCompile(prog)
	if wp == nil {
		wasmCache.Store(prog, wasmFail)
		return nil
	}
	wasmCache.Store(prog, wp)
	return wp
}

// wasmCompile translates IR → WASM binary → wazero compiled module.
func closeWasmModule(ctx context.Context, mod api.Module) {
	if err := mod.Close(ctx); err != nil {
		fmt.Fprintln(Stderr, "wasm module close error:", err)
	}
}

func wasmCompile(prog *IRProgram) *WasmProgram {
	// Try pure-numeric path first (faster, no imports needed)
	bin := irToWasm(prog)
	// TODO: enable imports path once collection handle ABI/control-flow is fully validated.
	// if bin == nil {
	// 	bin = irToWasmWithImports(prog)
	// }
	if bin == nil {
		return nil
	}

	rt := getWasmRT()
	ctx := context.Background()

	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return nil
	}

	cfg := wazero.NewModuleConfig().WithName(nextWasmModName())
	mod, err := rt.InstantiateModule(ctx, compiled, cfg)
	if err != nil {
		return nil
	}

	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		closeWasmModule(ctx, mod)
		return nil
	}

	wp := &WasmProgram{
		mod:        mod,
		execFn:     execFn,
		useFloat:   irProgramUsesFloat(prog),
		hasImports: !isWasmEligible(prog),
		constants:  prog.constants,
		bytes:      append([]byte(nil), bin...),
	}
	return wp
}

func wasmExec(wp *WasmProgram, slots []Object) Object {
	// Create object table for this execution
	table := &objectTable{objects: make([]Object, 0, 16)}

	// Pre-populate with IR program constants (for handle references)
	if wp.hasImports && len(wp.constants) > 0 {
		for _, c := range wp.constants {
			table.objects = append(table.objects, c)
		}
	}

	var stackBuf [16]uint64
	var stack []uint64
	if len(slots) <= len(stackBuf) {
		stack = stackBuf[:len(slots)]
	} else {
		stack = make([]uint64, len(slots))
	}
	for i, s := range slots {
		switch v := s.(type) {
		case Int:
			if wp.useFloat {
				stack[i] = math.Float64bits(float64(v.I))
			} else {
				stack[i] = uint64(v.I)
			}
		case Double:
			if wp.useFloat {
				stack[i] = math.Float64bits(v.D)
			} else {
				return nil
			}
		default:
			stack[i] = table.store(s)
		}
	}

	ctx := withObjectTable(context.Background(), table)
	if err := wp.execFn.CallWithStack(ctx, stack); err != nil {
		return nil
	}

	r := stack[0]
	if wp.useFloat {
		return Double{D: math.Float64frombits(r)}
	}
	// Check if result is a handle
	if isHandle(r) {
		return table.load(r)
	}
	return Int{I: int(int64(r))}
}

// Ensure math import is used
var _ = math.Float64bits
