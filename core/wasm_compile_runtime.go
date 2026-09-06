package core

import (
	"context"
	"fmt"

	corewasm "github.com/rcarmo/go-joker/core/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

// closeWasmModule closes an instantiated WASM module and reports close errors
// without changing execution semantics for best-effort cleanup paths.
func closeWasmModule(ctx context.Context, mod api.Module) {
	if err := mod.Close(ctx); err != nil {
		fmt.Fprintln(Stderr, "wasm module close error:", err)
	}
}

// wasmCompile translates IR to a WASM binary, compiles it with wazero, and
// instantiates the exported exec function. Keep this root-owned while IRProgram,
// WasmProgram, and execution slot metadata still live in package core.
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

	cfg := wazero.NewModuleConfig().WithName(corewasm.NextWasmModuleName())
	mod, err := rt.InstantiateModule(ctx, compiled, cfg)
	if err != nil {
		return nil
	}

	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		closeWasmModule(ctx, mod)
		return nil
	}
	model := runtimeExec.ProgramModel(prog)
	if model == nil {
		closeWasmModule(ctx, mod)
		return nil
	}

	wp := &WasmProgram{
		recovery:   prog,
		mod:        mod,
		execFn:     execFn,
		useFloat:   corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0),
		hasImports: !corewasm.Eligible(model.Code),
		constants:  runtimeExec.ProgramConstants(prog),
		bytes:      append([]byte(nil), bin...),
	}
	return wp
}
