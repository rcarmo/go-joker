package jit

import (
	"math"

	core "github.com/rcarmo/go-joker/v42/core"
	types "github.com/rcarmo/go-joker/v42/core/types"
	_ "github.com/rcarmo/go-joker/v42/std/math"
)

func numeric(o types.Object) float64 {
	switch x := o.(type) {
	case types.Int:
		if int64(x.I) > 1<<53 || int64(x.I) < -(1<<53) {
			panic(core.RT.NewError("numeric buffer integer outside exact f64 range"))
		}
		return float64(x.I)
	case types.Double:
		return x.D
	}
	panic(core.RT.NewError("expected numeric argument"))
}
func index(o types.Object) int {
	n := numeric(o)
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 1048576 || math.Floor(n) != n {
		panic(core.RT.NewError("numeric buffer index must be integral and in range"))
	}
	return int(n)
}
func init() {
	previous := jitNamespace.Lazy
	jitNamespace.Lazy = func() {
		previous()
		mathns := core.GLOBAL_ENV.EnsureSymbolIsLib(types.MakeSymbol(core.STRINGS.Intern, "joker.math"))
		if mathns.Lazy != nil {
			lazy := mathns.Lazy
			mathns.Lazy = nil
			lazy()
		}
		for _, name := range []string{"buffer-get", "buffer-set!"} {
			core.RegisterNumericPrimitive(jitNamespace.Resolve(name))
		}
	}
}
func numericBuffer(size types.Object) types.Object {
	n, ok := size.(types.Int)
	if !ok {
		panic(core.RT.NewError("buffer size must be Int"))
	}
	return core.NewWasmBuffer(n.I)
}
func bufferGet(buffer, indexValue types.Object) types.Object {
	b, ok := buffer.(*core.WasmBuffer)
	if !ok {
		panic(core.RT.NewError("expected numeric buffer"))
	}
	return types.MakeDouble(core.WasmBufferGet(b, index(indexValue)))
}
func bufferSet(buffer, indexValue, value types.Object) types.Object {
	b, ok := buffer.(*core.WasmBuffer)
	if !ok {
		panic(core.RT.NewError("expected numeric buffer"))
	}
	return types.MakeDouble(core.WasmBufferSet(b, index(indexValue), numeric(value)))
}
func compileWASMOptions(fn *core.Fn, options types.Object) types.Object {
	m, ok := options.(types.Map)
	if !ok {
		panic(core.RT.NewError("expected options map"))
	}
	found, o := m.Get(types.MakeKeyword(core.STRINGS.Intern, "buffers"))
	if !found {
		panic(core.RT.NewError("numeric options require :buffers"))
	}
	vec, ok := o.(types.Vec)
	if !ok {
		panic(core.RT.NewError(":buffers requires vector"))
	}
	indices := make([]int, vec.Count())
	for i := range indices {
		x, ok := vec.Nth(i).(types.Int)
		if !ok {
			panic(core.RT.NewError("buffer indices must be Int"))
		}
		indices[i] = x.I
	}
	compiled, err := core.CompileNumericWasmExported(fn, indices)
	if err != nil {
		panic(core.RT.NewError("numeric compile-wasm: " + err.Error()))
	}
	return compiled
}

func ensureJitFn(o types.Object, context string) *core.Fn {
	fn, ok := o.(*core.Fn)
	if !ok {
		panic(core.RT.NewError(context + ": expected function"))
	}
	return fn
}
