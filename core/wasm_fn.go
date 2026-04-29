package core

// wasm_fn.go — WASM compilation for standalone functions.
// When irCallSlot targets a fn whose IR is WASM-eligible,
// execute it via native WASM instead of nested irExec.

import "sync"

var wasmFnCache sync.Map // map[*FnArityExpr]*WasmProgram

var wasmFnFail = &WasmProgram{}

// wasmGetFn retrieves or compiles a WASM program for a Fn.
func wasmGetFn(fn *Fn) *WasmProgram {
	if len(fn.fnExpr.arities) != 1 {
		return nil
	}
	arity := &fn.fnExpr.arities[0]

	if v, ok := wasmFnCache.Load(arity); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFnFail {
			return nil
		}
		return wp
	}

	// First compile to IR
	irProg := irCompileFn(fn)
	if irProg == nil {
		wasmFnCache.Store(arity, wasmFnFail)
		return nil
	}

	// Then try WASM
	wp := wasmCompile(irProg)
	if wp == nil {
		wasmFnCache.Store(arity, wasmFnFail)
		return nil
	}

	wasmFnCache.Store(arity, wp)
	return wp
}
