package core

// wasm_runtime.go — wazero-based WASM execution engine.
// Compiles WASM modules and caches them. Handles Object ↔ WASM i64 conversion.

import (
	"context"
	"math"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// WasmProgram is a compiled, ready-to-execute WASM module.
type WasmProgram struct {
	mod      api.Module
	execFn   api.Function
	useFloat bool
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
		// Use a compilation cache for faster startup on repeated runs
		cache := wazero.NewCompilationCache()
		wasmRT = wazero.NewRuntimeWithConfig(context.Background(),
			wazero.NewRuntimeConfig().WithCompilationCache(cache))
	})
	return wasmRT
}

func nextWasmModName() string {
	wasmModMu.Lock()
	wasmModSeq++
	n := wasmModSeq
	wasmModMu.Unlock()
	return "joker_wasm_" + string(rune('0'+n%10)) + string(rune('0'+n/10%10))
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
func wasmCompile(prog *IRProgram) *WasmProgram {
	bin := irToWasm(prog)
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
		return nil
	}

	return &WasmProgram{mod: mod, execFn: execFn, useFloat: irProgramUsesFloat(prog)}
}

// wasmExec runs a WASM program. Supports Int (i64 mode) and Double (f64 mode).
func wasmExec(wp *WasmProgram, slots []Object) Object {
	params := make([]uint64, len(slots))
	for i, s := range slots {
		switch v := s.(type) {
		case Int:
			if wp.useFloat {
				params[i] = math.Float64bits(float64(v.I))
			} else {
				params[i] = uint64(v.I)
			}
		case Double:
			if wp.useFloat {
				params[i] = math.Float64bits(v.D)
			} else {
				return nil
			}
		default:
			return nil
		}
	}

	results, err := wp.execFn.Call(context.Background(), params...)
	if err != nil {
		return nil
	}
	if len(results) == 0 {
		return NIL
	}
	if wp.useFloat {
		return Double{D: math.Float64frombits(results[0])}
	}
	return Int{I: int(int64(results[0]))}
}

// Ensure math import is used
var _ = math.Float64bits
