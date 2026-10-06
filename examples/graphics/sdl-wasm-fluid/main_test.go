//go:build (linux || darwin || windows) && (amd64 || arm64)

package main

import (
	"bufio"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	core "github.com/rcarmo/go-joker/v42/core"
	types "github.com/rcarmo/go-joker/v42/core/types"
)

func TestJokerFluidKernels(t *testing.T) {
	core.GLOBAL_ENV.InitEnv(os.Stdin, os.Stdout, os.Stderr, nil)
	path, err := filepath.Abs("kernels.joke")
	if err != nil {
		t.Fatal(err)
	}
	if err := runScript(path); err != nil {
		t.Fatal(err)
	}
	ns := core.GLOBAL_ENV.EnsureSymbolIsLib(types.MakeSymbol(core.STRINGS.Intern, "user"))
	advance := ns.Resolve("advance!").Value.(types.Callable)
	fields := core.NewWasmBuffer(11 * 96 * 96)
	for i := 0; i < 36; i++ {
		advance.Call([]types.Object{fields})
	}
	var dye, energy float64
	for i, n := range fields.Values() {
		if math.IsNaN(n) || math.IsInf(n, 0) {
			t.Fatalf("nonfinite field %d", i)
		}
		if i < 2*9216 {
			energy += n * n
		}
		if i >= 4*9216 && i < 7*9216 {
			if n < 0 || n > 100 {
				t.Fatalf("dye out of range %d: %g", i, n)
			}
			dye += n
		}
	}
	if dye < 10 || dye > 1e5 || energy <= 0 || energy > 1e6 {
		t.Fatalf("empty/unbounded simulation: dye=%g energy=%g", dye, energy)
	}
	pixels := fields.Values()[10*9216:]
	var nonzero int
	for _, n := range pixels {
		u := uint32(n)
		if byte(u>>24) != 255 {
			t.Fatal("alpha")
		}
		if u&0xffffff != 0 {
			nonzero++
		}
	}
	if nonzero < 100 {
		t.Fatal("blank rendering")
	}
	// Compare a second identical state using numerical tolerances, never hashes.
	second := core.NewWasmBuffer(11 * 96 * 96)
	for i := 0; i < 36; i++ {
		advance.Call([]types.Object{second})
	}
	var errorSum float64
	for i, n := range fields.Values() {
		d := math.Abs(n - second.Values()[i])
		if d > 1e-8*math.Max(1, math.Abs(n)) {
			t.Fatalf("state drift %d", i)
		}
		errorSum += d * d
	}
	if errorSum > 1e-10 {
		t.Fatal("aggregate state drift", errorSum)
	}
	t.Logf("actual engine=%s dye=%g energy=%g visible=%d", core.WasmEngineExported(), dye, energy, nonzero)
}

func TestFluidAgainstTreeWalker(t *testing.T) {
	path, err := filepath.Abs("kernels.joke")
	if err != nil {
		t.Fatal(err)
	}
	if err := runScript(path); err != nil {
		t.Fatal(err)
	}
	ns := core.GLOBAL_ENV.EnsureSymbolIsLib(types.MakeSymbol(core.STRINGS.Intern, "user"))
	compiled := core.NewWasmBuffer(11 * 9216)
	advance := ns.Resolve("advance!").Value.(types.Callable)
	for i := 0; i < 2; i++ {
		advance.Call([]types.Object{compiled})
	}
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Test-only oracle: use the same authored kernels with compile-wasm removed.
	referenceSource := strings.ReplaceAll(string(source), "(jit/compile-wasm", "((fn [f opts] f)")
	if err := runReader(bufio.NewReader(strings.NewReader(referenceSource)), path+"-tree-reference"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := runScript(path); err != nil {
			t.Error(err)
		}
	}()
	reference := core.NewWasmBuffer(11 * 9216)
	advance = ns.Resolve("advance!").Value.(types.Callable)
	for i := 0; i < 2; i++ {
		advance.Call([]types.Object{reference})
	}
	var maxAbs, maxRel, sumSq float64
	var colourError int
	for i, n := range compiled.Values() {
		v := reference.Values()[i]
		d := math.Abs(n - v)
		if i >= 10*9216 {
			a, b := uint32(n), uint32(v)
			for ch := 0; ch < 3; ch++ {
				x, y := int(byte(a>>uint(8*ch))), int(byte(b>>uint(8*ch)))
				if x-y > 1 || y-x > 1 {
					t.Fatalf("colour error at %d channel %d: %d %d", i, ch, x, y)
				}
				if x != y {
					colourError++
				}
			}
			continue
		}
		rel := d / math.Max(1, math.Abs(v))
		maxAbs = math.Max(maxAbs, d)
		maxRel = math.Max(maxRel, rel)
		sumSq += d * d
		if d > 1e-10+1e-9*math.Abs(v) {
			t.Fatalf("tree reference mismatch %d: %.17g %.17g (%g)", i, n, v, d)
		}
	}
	t.Logf("engine=%s reference=tree-walker max_abs=%g max_scaled=%g rms=%g colour_differences=%d", core.WasmEngineExported(), maxAbs, maxRel, math.Sqrt(sumSq/(10*9216)), colourError)
}
