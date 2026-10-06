package jit

import (
	core "github.com/rcarmo/go-joker/v42/core"
	types "github.com/rcarmo/go-joker/v42/core/types"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestDenseNumericWASM(t *testing.T) {
	if mode := os.Getenv("JOKER_TEST_DENSE_ENGINE"); mode != "" {
		if mode == "compiler" && runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
			t.Skip("explicit compiler unavailable on this architecture")
		}
		if jitNamespace.Lazy != nil {
			lazy := jitNamespace.Lazy
			jitNamespace.Lazy = nil
			lazy()
		}
		f := mkFn(`(fn [a] (loop [y 0] (if (< y 3) (do (loop [x 0] (if (< x 4) (do (joker.jit/buffer-set! a (+ x (* y 4)) (+ (* y 10) x)) (recur (+ x 1))) 0)) (recur (+ y 1))) (joker.math/sqrt (joker.math/abs (- (joker.jit/buffer-get a 11) 32))))))`)
		kernel, e := core.CompileNumericWasmExported(f, []int{0})
		if e != nil {
			t.Fatal(e)
		}
		b := core.NewWasmBuffer(12)
		r := kernel.(types.Callable).Call([]types.Object{b})
		if r.(types.Double).D != 3 {
			t.Fatal(r)
		}
		for y := 0; y < 3; y++ {
			for x := 0; x < 4; x++ {
				if v := core.WasmBufferGet(b, y*4+x); v != float64(y*10+x) {
					t.Fatal(x, y, v)
				}
			}
		}
		// Fractional/NaN/negative/end indices trap before an out-of-bounds memory operation.
		for _, index := range []float64{-1, 12, 1.5, math.NaN()} {
			fn := mkFn(`(fn [a i] (do (joker.jit/buffer-set! a 0 42) (joker.jit/buffer-get a i)))`)
			k, e := core.CompileNumericWasmExported(fn, []int{0})
			if e != nil {
				t.Fatal(e)
			}
			func() {
				defer func() {
					if recover() == nil {
						t.Error("expected memory trap")
					}
				}()
				k.(types.Callable).Call([]types.Object{b, types.MakeDouble(index)})
			}()
			if core.WasmBufferGet(b, 0) != 42 {
				t.Fatal("effect lost/replayed after trap")
			}
		}
		denseSafety(t)
		// Primitive rebinding must reject a cached kernel before side effects.
		mathns := core.GLOBAL_ENV.EnsureSymbolIsLib(types.MakeSymbol(core.STRINGS.Intern, "joker.math"))
		v := mathns.Resolve("sqrt")
		original := v.Value
		v.Value = core.Proc{Name: "replacement", Fn: func([]types.Object) types.Object { return types.MakeDouble(999) }}
		func() {
			defer func() {
				if recover() == nil {
					t.Error("rebound primitive was compiled")
				}
			}()
			kernel.(types.Callable).Call([]types.Object{b})
		}()
		v.Value = original
		// Aliasing is preserved and two distinct buffers can be passed in either order.
		aliasFn := mkFn(`(fn [a b] (do (joker.jit/buffer-set! a 0 9) (joker.jit/buffer-get b 0)))`)
		alias, e := core.CompileNumericWasmExported(aliasFn, []int{0, 1})
		if e != nil {
			t.Fatal(e)
		}
		if r := alias.(types.Callable).Call([]types.Object{b, b}).(types.Double).D; r != 9 {
			t.Fatal("alias lost", r)
		}
		if core.WasmEngineExported() != mode {
			t.Fatal("silent fallback")
		}
		return
	}
	for _, mode := range []string{"interpreter", "compiler"} {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDenseNumericWASM$")
		cmd.Env = append(os.Environ(), "JOKER_TEST_DENSE_ENGINE="+mode, "JOKER_WASM_ENGINE="+mode)
		if out, e := cmd.CombinedOutput(); e != nil {
			t.Fatalf("%s %v\n%s", mode, e, out)
		}
	}
}

// Run in the engine subprocess so runtime selection cannot leak between cases.
func denseSafety(t *testing.T) {
	cases := []struct {
		source string
		want   float64
	}{
		{`(fn [] (if 0 7 9))`, 7},
		{`(fn [] (if (not 0) 7 9))`, 9},
		{`(fn [] (if nil 7 9))`, 9},
		{`(fn [] (let [x false] (if x 7 9)))`, 9},
		{`(fn [x] (let [x (let [x 9] x) y (+ x 1)] (+ x y)))`, 19},
		{`(fn [x] (loop [a x b 2 n 0] (if (< n 2) (recur b a (+ n 1)) (+ (* a 10) b))))`, 32},
	}
	for _, tc := range cases {
		f := mkFn(tc.source)
		k, e := core.CompileNumericWasmExported(f, nil)
		if e != nil {
			t.Fatal(tc.source, e)
		}
		args := []types.Object{}
		if strings.Contains(tc.source, "[x]") {
			args = []types.Object{types.MakeInt(3)}
		}
		got := k.(types.Callable).Call(args).(types.Double).D
		if got != tc.want {
			t.Fatal(tc.source, got)
		}
		reference := f.Call(args).(types.Number).Double().D
		if got != reference {
			t.Fatal("tree walker discrepancy", tc.source, got, reference)
		}
	}
	reject := []string{`(fn [a] (let [a 2] (joker.jit/buffer-get a 0)))`, `(fn [] true)`, `(fn [] (+ false 2))`, `(fn [] (loop [x 0] (if (< x 1) (recur false) x)))`}
	for _, source := range reject {
		f := mkFn(source)
		if k, e := core.CompileNumericWasmExported(f, func() []int {
			if strings.Contains(source, "[a]") {
				return []int{0}
			}
			return nil
		}()); e == nil {
			t.Fatalf("accepted %s (%v)", source, k)
		}
	}
	b := core.NewWasmBuffer(2)
	// Evaluate all write arguments before bounds validation, matching the host API.
	f := mkFn(`(fn [a] (joker.jit/buffer-set! a 2 (joker.jit/buffer-set! a 0 (+ (joker.jit/buffer-get a 0) 1))))`)
	k, e := core.CompileNumericWasmExported(f, []int{0})
	if e != nil {
		t.Fatal(e)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected trap")
			}
		}()
		k.(types.Callable).Call([]types.Object{b})
	}()
	if core.WasmBufferGet(b, 0) != 1 {
		t.Fatal("value evaluation lost or replayed")
	}
	for _, idx := range []float64{math.Inf(1), math.Inf(-1), math.NaN(), -0.5, 2} {
		fn := mkFn(`(fn [a i] (joker.jit/buffer-get a i))`)
		k, e := core.CompileNumericWasmExported(fn, []int{0})
		if e != nil {
			t.Fatal(e)
		}
		func() {
			defer func() {
				if recover() == nil {
					t.Error("bad index accepted", idx)
				}
			}()
			k.(types.Callable).Call([]types.Object{b, types.MakeDouble(idx)})
		}()
	}
	if _, e := core.CompileNumericWasmExported(mkFn(`(fn [a] 0)`), []int{0, 0}); e == nil {
		t.Error("duplicate index")
	}
	// Independent kernels lock buffers in stable identity order.
	makeKernel := func() types.Callable {
		k, e := core.CompileNumericWasmExported(mkFn(`(fn [a b] (do (joker.jit/buffer-set! a 0 (+ (joker.jit/buffer-get a 0) 1)) (joker.jit/buffer-set! b 0 (+ (joker.jit/buffer-get b 0) 1)) 0))`), []int{0, 1})
		if e != nil {
			t.Fatal(e)
		}
		return k.(types.Callable)
	}
	left, right := makeKernel(), makeKernel()
	if left.(types.Object).Equals(right) {
		t.Fatal("distinct compiled callables compare equal")
	}
	other := core.NewWasmBuffer(2)
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(reverse bool) {
			defer wg.Done()
			a, z, k := b, other, left
			if reverse {
				a, z, k = other, b, right
			}
			for n := 0; n < 100; n++ {
				k.Call([]types.Object{a, z})
			}
		}(i%2 == 0)
	}
	wg.Wait()
	if core.WasmBufferGet(b, 0) != 401 || core.WasmBufferGet(other, 0) != 400 {
		t.Fatal("lost concurrent writes")
	}
	if strconv.IntSize == 64 {
		scalar, e := core.CompileNumericWasmExported(mkFn(`(fn [x] x)`), nil)
		if e != nil {
			t.Fatal(e)
		}
		large := int64(1<<53) + 1
		func() {
			defer func() {
				if recover() == nil {
					t.Error("rounded large integer")
				}
			}()
			scalar.(types.Callable).Call([]types.Object{types.MakeInt(int(large))})
		}()
	}
}

func BenchmarkDenseBufferKernel(b *testing.B) {
	if jitNamespace.Lazy != nil {
		lazy := jitNamespace.Lazy
		jitNamespace.Lazy = nil
		lazy()
	}
	k, e := core.CompileNumericWasmExported(mkFn(`(fn [a] (loop [i 0] (if (< i 9216) (do (joker.jit/buffer-set! a i (+ (joker.jit/buffer-get a i) 0.25)) (recur (+ i 1))) 0)))`), []int{0})
	if e != nil {
		b.Fatal(e)
	}
	buffer := core.NewWasmBuffer(9216)
	args := []types.Object{buffer}
	call := k.(types.Callable)
	call.Call(args)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		call.Call(args)
	}
}
