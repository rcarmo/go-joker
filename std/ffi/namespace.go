// Package ffi registers the joker.ffi namespace. Native calls are available without cgo
// on Linux/macOS/Windows amd64/arm64. Other targets expose unavailable errors.
package ffi

import (
	core "github.com/rcarmo/go-joker/v42/core"
	types "github.com/rcarmo/go-joker/v42/core/types"
)

var namespace = core.GLOBAL_ENV.EnsureSymbolIsLib(types.MakeSymbol(core.STRINGS.Intern, "joker.ffi"))
var initialize = unavailable

func init() {
	namespace.Lazy = func() {
		namespace.ResetMeta(core.MakeMeta(nil, "Fixed-signature C ABI calls for trusted scripts. No cgo required; see docs/FFI.md.", "42.12.0"))
		initialize()
	}
}
func unavailable() {
	for name, doc := range map[string]string{
		"open":   "Load an absolute library path. Libraries stay loaded for process lifetime. Supported on Linux/macOS/Windows amd64/arm64.",
		"bind":   "Bind (library symbol argument-type-vector return-type); fixed scalar/string/pointer signatures. Supported on Linux/macOS/Windows amd64/arm64.",
		"buffer": "Allocate a reusable byte buffer (0..64 MiB). C must not retain its address. Supported on Linux/macOS/Windows amd64/arm64.",
		"u32":    "Read a checked little-endian uint32 from a buffer at a byte offset. Supported on Linux/macOS/Windows amd64/arm64.",
	} {
		namespace.InternVar(name, core.Proc{Name: name, Package: "std/ffi", Fn: func([]types.Object) types.Object {
			panic(core.RT.NewError("ffi unavailable: supported on Linux/macOS/Windows amd64/arm64"))
		}}, core.MakeMeta(nil, doc, "42.12.0"))
	}
}
