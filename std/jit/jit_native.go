package jit

import (
	"fmt"
	. "github.com/candid82/joker/core"
)

// jit_native.go — joker.jit namespace: compile Clojure functions to
// native Go closures for maximum performance.
//
// Usage:
//   (require 'jit)
//   (def fast-add (jit/compile (fn [x y] (+ x y))))
//   (fast-add 3.0 4.0)  ; => 7.0
//
//   (jit/info (fn [x y] (* x y)))
//   ; => {:compiled true :path "native-f64" :slots 2}
//
//   (jit/compiled? (fn [x] (+ x 1)))
//   ; => true

// compile takes a Fn and returns a compiled version.
// For pure arithmetic fns, the compiled version runs as a native
// Go f64 closure — zero interpreter overhead.
// For other fns, returns an IR-compiled wrapper.
func compile(fn *Fn) Object {
	prog := IrCompileFn(fn)
	if prog == nil {
		panic(RT.NewError("jit/compile: function cannot be compiled to IR"))
	}

	// If it has a native f64 helper, wrap it as a Proc
	if nh := prog.GetNativeHelper(); nh != nil {
		nargs := prog.NumSlots()
		if len(prog.CaptureSlots()) > 0 {
			nargs -= len(prog.CaptureSlots())
		}
		return Proc{
			Fn: func(args []Object) Object {
				f64buf := make([]float64, len(args))
				for i, a := range args {
					switch v := a.(type) {
					case Double:
						f64buf[i] = v.D
					case Int:
						f64buf[i] = float64(v.I)
					default:
						panic(RT.NewError(fmt.Sprintf("jit: argument %d must be a number, got %s", i, a.GetType().ToString(false))))
					}
				}
				return Double{D: nh(f64buf)}
			},
			Name: "jit-compiled",
		}
	}

	// Otherwise return an IR-compiled wrapper
	return Proc{
		Fn: func(args []Object) Object {
			result := IrExecTyped(prog, args)
			if result == nil {
				result = IrExec(prog, args)
			}
			if result == nil {
				return fn.Call(args)
			}
			return result
		},
		Name: "jit-compiled",
	}
}

// info returns a map with compilation information about a fn.
func info(fn *Fn) Object {
	m := EmptyArrayMap()
	prog := IrCompileFn(fn)
	if prog == nil {
		m.Add(MakeKeyword("compiled"), Boolean{B: false})
		return m
	}
	m.Add(MakeKeyword("compiled"), Boolean{B: true})
	m.Add(MakeKeyword("slots"), Int{I: prog.NumSlots()})
	m.Add(MakeKeyword("captures"), Int{I: len(prog.CaptureSlots())})
	m.Add(MakeKeyword("self-recursive"), Boolean{B: prog.HasSelf()})

	if prog.GetNativeHelper() != nil {
		m.Add(MakeKeyword("path"), String{S: "native-f64"})
	} else {
		a := AnalyzeIRProgramExported(prog)
		if a.Eligible {
			m.Add(MakeKeyword("path"), String{S: "typed-ir"})
		} else {
			m.Add(MakeKeyword("path"), String{S: "boxed-ir"})
		}
	}
	return m
}

// isCompiled returns true if the fn can be compiled to IR.
func isCompiled(fn *Fn) bool {
	return IrCompileFn(fn) != nil
}
