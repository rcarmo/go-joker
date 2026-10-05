// Package ffi registers the optional joker.ffi namespace. Native support needs
// -tags joker_ffi on Linux/macOS/Windows amd64/arm64; default builds stay static.
package ffi

import (
	core "github.com/rcarmo/go-joker/v42/core"
	types "github.com/rcarmo/go-joker/v42/core/types"
)

var namespace = core.GLOBAL_ENV.EnsureSymbolIsLib(types.MakeSymbol(core.STRINGS.Intern, "joker.ffi"))
var initialize = unavailable

func init() {
	namespace.Lazy = func() {
		namespace.ResetMeta(core.MakeMeta(nil, "Optional fixed-signature C ABI calls for trusted scripts. Build with joker_ffi; see docs/FFI.md.", "42.11.4-dev"))
		initialize()
	}
}
func unavailable() {
	for name, doc := range map[string]string{
		"open":   "Load an absolute library path. Libraries stay loaded for process lifetime. Requires joker_ffi.",
		"bind":   "Bind (library symbol argument-type-vector return-type); fixed scalar/string/pointer signatures. Requires joker_ffi.",
		"buffer": "Allocate a reusable byte buffer (0..64 MiB). C must not retain its address. Requires joker_ffi.",
		"u32":    "Read a checked little-endian uint32 from a buffer at a byte offset. Requires joker_ffi.",
	} {
		namespace.InternVar(name, core.Proc{Name: name, Package: "std/ffi", Fn: func([]types.Object) types.Object {
			panic(core.RT.NewError("ffi unavailable: build with -tags joker_ffi on Linux/macOS/Windows amd64/arm64"))
		}}, core.MakeMeta(nil, doc, "42.11.4-dev"))
	}
}
