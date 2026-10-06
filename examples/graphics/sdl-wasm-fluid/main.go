//go:build (linux || darwin || windows) && (amd64 || arm64)

// Hybrid host: dense simulation runs in WASM; Joker controls SDL through FFI.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"

	core "github.com/rcarmo/go-joker/v42/core"
	types "github.com/rcarmo/go-joker/v42/core/types"
	"github.com/rcarmo/go-joker/v42/std/ffi"
	_ "github.com/rcarmo/go-joker/v42/std/jit"
	_ "github.com/rcarmo/go-joker/v42/std/math"
)

func init() { runtime.LockOSThread() }
func main() {
	library := flag.String("library", "/usr/lib/x86_64-linux-gnu/libSDL2-2.0.so.0", "absolute SDL2 library path")
	script := flag.String("script", "examples/graphics/sdl-wasm-fluid/fluid.joke", "Joker script")
	frames := flag.Int("frames", 360, "simulation frames; 0 for interactive")
	shot := flag.String("screenshot", "docs/images/sdl-wasm-fluid.png", "PNG captured from SDL render readback")
	cpu := flag.String("cpuprofile", "", "CPU profile")
	heap := flag.String("memprofile", "", "heap profile")
	engine := flag.String("wasm-engine", "compiler", "WASM compiler or interpreter")
	flag.Parse()
	must(os.Setenv("JOKER_WASM_ENGINE", *engine))
	if *cpu != "" {
		f, e := os.Create(*cpu)
		must(e)
		must(pprof.StartCPUProfile(f))
		defer f.Close()
		defer pprof.StopCPUProfile()
	}
	if *heap != "" {
		defer func() {
			runtime.GC()
			f, e := os.Create(*heap)
			must(e)
			defer f.Close()
			must(pprof.WriteHeapProfile(f))
		}()
	}
	core.GLOBAL_ENV.InitEnv(os.Stdin, os.Stdout, os.Stderr, nil)
	pixels := ffi.NewBuffer(96 * 96 * 4)
	fields := core.NewWasmBuffer(11 * 96 * 96)
	capture := ffi.NewBuffer(768 * 768 * 4)
	ns := core.GLOBAL_ENV.EnsureSymbolIsLib(types.MakeSymbol(core.STRINGS.Intern, "fluid.hybrid"))
	add := func(name string, value types.Object) {
		ns.InternVar(name, value, core.MakeMeta(nil, "SDL fluid example host", ""))
	}
	add("library", types.MakeString(*library))
	add("frames", types.MakeInt(*frames))
	add("engine", types.MakeString(core.WasmEngineExported()))
	add("fields", fields)
	add("pixels", pixels)
	add("capture", capture)
	add("upload!", core.Proc{Name: "upload!", Fn: func(a []types.Object) types.Object {
		core.CheckArity(a, 0, 0)
		values := fields.Values()[10*96*96:]
		dst := pixels.Bytes()
		for i, v := range values {
			u := uint32(v)
			dst[i*4] = byte(u)
			dst[i*4+1] = byte(u >> 8)
			dst[i*4+2] = byte(u >> 16)
			dst[i*4+3] = byte(u >> 24)
		}
		return pixels
	}})
	fmt.Println("Fluid calculations: Joker compile-wasm", core.WasmEngineExported())
	must(runScript("examples/graphics/sdl-wasm-fluid/kernels.joke"))
	add("save!", core.Proc{Name: "save!", Fn: func(a []types.Object) types.Object {
		core.CheckArity(a, 0, 0)
		must(os.MkdirAll(filepath.Dir(*shot), 0755))
		f, e := os.Create(*shot)
		must(e)
		defer f.Close()
		must(png.Encode(f, &image.RGBA{Pix: capture.Bytes(), Stride: 768 * 4, Rect: image.Rect(0, 0, 768, 768)}))
		return types.MakeString(*shot)
	}})
	must(runScript(*script))
}
func runScript(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	return runReader(bufio.NewReader(f), path)
}
func runReader(reader *bufio.Reader, path string) error {
	r := core.NewReader(reader, path)
	for {
		obj, e := core.TryRead(r)
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return e
		}
		expr, e := core.TryParse(obj, &core.ParseContext{GlobalEnv: core.GLOBAL_ENV})
		if e != nil {
			return e
		}
		if _, e = core.TryEval(expr); e != nil {
			return e
		}
	}
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
