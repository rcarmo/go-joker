package jit

import (
	core "github.com/rcarmo/go-joker/v42/core"
	types "github.com/rcarmo/go-joker/v42/core/types"
)

func init() {
	previous := jitNamespace.Lazy
	jitNamespace.Lazy = func() {
		previous()
		jitNamespace.InternVar("wasm-engine", core.Proc{Name: "wasm-engine", Package: "std/jit", Fn: func(args []types.Object) types.Object {
			core.CheckArity(args, 0, 0)
			return types.MakeString(core.WasmEngineExported())
		}}, core.MakeMeta(nil, "Returns the actual WASM engine: interpreter or compiler. Set JOKER_WASM_ENGINE before first WASM use.", "42.12.2"))
	}
}
