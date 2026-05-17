package jit

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"os"

	. "github.com/rcarmo/go-joker/core"
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
					case coretypes.Double:
						f64buf[i] = v.D
					case coretypes.Int:
						f64buf[i] = float64(v.I)
					default:
						panic(RT.NewError(fmt.Sprintf("jit: argument %d must be a number, got %s", i, a.GetType().ToString(false))))
					}
				}
				return coretypes.Double{D: nh(f64buf)}
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
		m.Add(MakeKeyword("compiled"), coretypes.Boolean{B: false})
		return m
	}
	m.Add(MakeKeyword("compiled"), coretypes.Boolean{B: true})
	m.Add(MakeKeyword("slots"), coretypes.Int{I: prog.NumSlots()})
	m.Add(MakeKeyword("captures"), coretypes.Int{I: len(prog.CaptureSlots())})
	m.Add(MakeKeyword("self-recursive"), coretypes.Boolean{B: prog.HasSelf()})

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

type irExportFile struct {
	Format    string           `json:"format"`
	Version   int              `json:"version"`
	NumSlots  int              `json:"numSlots"`
	Code      string           `json:"code"`
	Constants []irExportConst  `json:"constants"`
	WASM      irExportWASMInfo `json:"wasm"`
}

type irExportConst struct {
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type irExportWASMInfo struct {
	Eligible bool   `json:"eligible"`
	UseFloat bool   `json:"useFloat"`
	Reason   string `json:"reason,omitempty"`
}

func exportConst(o Object) irExportConst {
	switch v := o.(type) {
	case coretypes.Int:
		return irExportConst{Type: "int", Value: v.I}
	case coretypes.Double:
		return irExportConst{Type: "double", Value: v.D}
	case String:
		return irExportConst{Type: "string", Value: v.S}
	case coretypes.Boolean:
		return irExportConst{Type: "boolean", Value: v.B}
	case Keyword:
		return irExportConst{Type: "keyword", Value: v.ToString(false)}
	case Symbol:
		return irExportConst{Type: "symbol", Value: v.ToString(false)}
	case Nil:
		return irExportConst{Type: "nil", Value: nil}
	default:
		return irExportConst{Type: o.GetType().ToString(false), Value: o.ToString(false)}
	}
}

func exportIR(fn *Fn, path String) Object {
	prog := IrCompileFn(fn)
	if prog == nil {
		panic(RT.NewError("jit/export-ir: function cannot be compiled to IR"))
	}
	consts := prog.Constants()
	exportedConsts := make([]irExportConst, len(consts))
	for i, c := range consts {
		exportedConsts[i] = exportConst(c)
	}
	wasmDiag := ExplainWASMEligibility(prog)
	file := irExportFile{
		Format:    "go-joker-ir",
		Version:   1,
		NumSlots:  prog.NumSlots(),
		Code:      base64.StdEncoding.EncodeToString(prog.CodeBytes()),
		Constants: exportedConsts,
		WASM: irExportWASMInfo{
			Eligible: wasmDiag.Eligible,
			UseFloat: IsFloatExported(prog),
			Reason:   wasmDiag.Reason,
		},
	}
	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		panic(RT.NewError("jit/export-ir: " + err.Error()))
	}
	if err := os.WriteFile(path.S, data, 0o644); err != nil {
		panic(RT.NewError("jit/export-ir: " + err.Error()))
	}
	return path
}

func exportWASM(fn *Fn, path String) Object {
	prog := IrCompileFn(fn)
	if prog == nil {
		panic(RT.NewError("jit/export-wasm: function cannot be compiled to IR"))
	}
	bin := WasmCompileBytesExported(prog)
	if bin == nil {
		d := ExplainWASMEligibility(prog)
		reason := d.Reason
		if reason == "" {
			reason = "unsupported IR shape"
		}
		panic(RT.NewError("jit/export-wasm: function cannot be compiled to standalone WASM: " + reason))
	}
	if err := os.WriteFile(path.S, bin, 0o644); err != nil {
		panic(RT.NewError("jit/export-wasm: " + err.Error()))
	}
	return path
}
