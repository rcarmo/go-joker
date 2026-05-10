package jit

import (
	. "github.com/candid82/joker/core"
)

var jitNamespace = GLOBAL_ENV.EnsureSymbolIsLib(MakeSymbol("joker.jit"))

func init() {
	jitNamespace.Lazy = Init
}

func Init() {
	jitNamespace.ResetMeta(MakeMeta(nil, "JIT compilation for Clojure functions. Compiles pure arithmetic functions to native Go closures for maximum performance.", "1.0"))

	jitNamespace.InternVar("compile", compile_,
		MakeMeta(
			NewListFrom(NewVectorFrom(MakeSymbol("fn"))),
			`Compiles a function to the fastest available execution path.
For pure arithmetic fns, returns a native Go f64 closure.
For other fns, returns an IR-compiled wrapper.
The returned function can be called like any other fn.`, "1.0"))

	jitNamespace.InternVar("info", info_,
		MakeMeta(
			NewListFrom(NewVectorFrom(MakeSymbol("fn"))),
			`Returns a map with compilation information about a function.
Keys: :compiled, :path, :slots, :captures, :self-recursive.
:path is one of "native-f64", "typed-ir", "boxed-ir".`, "1.0"))

	jitNamespace.InternVar("compiled?", compiled_,
		MakeMeta(
			NewListFrom(NewVectorFrom(MakeSymbol("fn"))),
			"Returns true if the function can be compiled to IR.", "1.0"))

	jitNamespace.InternVar("export-ir", export_ir_,
		MakeMeta(
			NewListFrom(NewVectorFrom(MakeSymbol("fn"), MakeSymbol("path"))),
			`Compiles fn to Joker IR and writes a portable .ir JSON file to path.

The file contains format/version metadata, numSlots, base64-encoded IR
bytecode, serialized literal constants, and WASM eligibility metadata. It is
intended for external runners/tooling that implement the go-joker IR ABI.`, "1.0"))

	jitNamespace.InternVar("export-wasm", export_wasm_,
		MakeMeta(
			NewListFrom(NewVectorFrom(MakeSymbol("fn"), MakeSymbol("path"))),
			`Compiles fn to a standalone numeric WASM module and writes raw .wasm bytes.

The exported module exposes an exec function. Parameters and return values are
i64 for integer IR programs or f64 for floating-point IR programs.`, "1.0"))
}

var compile_ Proc = Proc{Fn: func(args []Object) Object {
	fn := EnsureArgIsFn(args, 0)
	return compile(fn)
}, Name: "compile", Package: "std/jit"}

var info_ Proc = Proc{Fn: func(args []Object) Object {
	fn := EnsureArgIsFn(args, 0)
	return info(fn)
}, Name: "info", Package: "std/jit"}

var compiled_ Proc = Proc{Fn: func(args []Object) Object {
	fn := EnsureArgIsFn(args, 0)
	return Boolean{B: isCompiled(fn)}
}, Name: "compiled?", Package: "std/jit"}

var export_ir_ Proc = Proc{Fn: func(args []Object) Object {
	fn := EnsureArgIsFn(args, 0)
	path := EnsureArgIsString(args, 1)
	return exportIR(fn, path)
}, Name: "export-ir", Package: "std/jit"}

var export_wasm_ Proc = Proc{Fn: func(args []Object) Object {
	fn := EnsureArgIsFn(args, 0)
	path := EnsureArgIsString(args, 1)
	return exportWASM(fn, path)
}, Name: "export-wasm", Package: "std/jit"}
