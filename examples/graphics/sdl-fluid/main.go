//go:build (linux || darwin || windows) && (amd64 || arm64)

// SDL fluid example host: simulation buffers only; SDL calls live in fluid.joke.
package main

import (
	"bufio"
	"flag"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"

	core "github.com/rcarmo/go-joker/v42/core"
	types "github.com/rcarmo/go-joker/v42/core/types"
	"github.com/rcarmo/go-joker/v42/examples/graphics/sdl-fluid/internal/fluid"
	"github.com/rcarmo/go-joker/v42/std/ffi"
)

func init() { runtime.LockOSThread() }
func main() {
	library := flag.String("library", "/usr/lib/x86_64-linux-gnu/libSDL2-2.0.so.0", "absolute SDL2 library path")
	script := flag.String("script", "examples/graphics/sdl-fluid/fluid.joke", "Joker script")
	frames := flag.Int("frames", 240, "simulation frames; 0 for interactive")
	shot := flag.String("screenshot", ".cache/sdl-fluid.png", "PNG captured from SDL render readback")
	cpu := flag.String("cpuprofile", "", "CPU profile")
	heap := flag.String("memprofile", "", "heap profile")
	flag.Parse()
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
	s := fluid.New(96)
	pixels := ffi.NewBuffer(96 * 96 * 4)
	capture := ffi.NewBuffer(768 * 768 * 4)
	ns := core.GLOBAL_ENV.EnsureSymbolIsLib(types.MakeSymbol(core.STRINGS.Intern, "fluid.example"))
	add := func(name string, value types.Object) {
		ns.InternVar(name, value, core.MakeMeta(nil, "SDL fluid example host", ""))
	}
	add("library", types.MakeString(*library))
	add("frames", types.MakeInt(*frames))
	add("pixels", pixels)
	add("capture", capture)
	add("step!", core.Proc{Name: "step!", Fn: func(a []types.Object) types.Object {
		core.CheckArity(a, 0, 0)
		s.Step(1.0 / 60)
		s.RGBA(pixels.Bytes())
		return pixels
	}})
	add("save!", core.Proc{Name: "save!", Fn: func(a []types.Object) types.Object {
		core.CheckArity(a, 0, 0)
		must(os.MkdirAll(filepath.Dir(*shot), 0755))
		f, e := os.Create(*shot)
		must(e)
		defer f.Close()
		must(png.Encode(f, &image.RGBA{Pix: capture.Bytes(), Stride: 768 * 4, Rect: image.Rect(0, 0, 768, 768)}))
		return types.MakeString(*shot)
	}})
	f, e := os.Open(*script)
	must(e)
	defer f.Close()
	r := core.NewReader(bufio.NewReader(f), *script)
	for {
		obj, e := core.TryRead(r)
		if e == io.EOF {
			break
		}
		must(e)
		expr, e := core.TryParse(obj, &core.ParseContext{GlobalEnv: core.GLOBAL_ENV})
		must(e)
		_, e = core.TryEval(expr)
		must(e)
	}
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
