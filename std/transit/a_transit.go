package transit

import (
	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

var transitNamespace = GLOBAL_ENV.EnsureSymbolIsLib(coretypes.MakeSymbol(STRINGS.Intern, "joker.transit"))

func init() { transitNamespace.Lazy = Init }

func Init() {
	transitNamespace.ResetMeta(MakeMeta(nil, "Transit+JSON reader and writer for Joker values.", "1.0"))
	transitNamespace.InternVar("write", write_, MakeMeta(NewListFrom(NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "x"))), "Writes x as a Transit+JSON string.", "1.0"))
	transitNamespace.InternVar("write-str", write_, MakeMeta(NewListFrom(NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "x"))), "Alias for write; writes x as a Transit+JSON string.", "1.0"))
	transitNamespace.InternVar("write-verbose", writeVerbose_, MakeMeta(NewListFrom(NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "x"))), "Writes x as Transit+JSON without rolling cache refs for readable diagnostics.", "1.0"))
	transitNamespace.InternVar("read", read_, MakeMeta(NewListFrom(NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "s"))), "Reads a Transit+JSON string into Joker data.", "1.0"))
	transitNamespace.InternVar("read-str", read_, MakeMeta(NewListFrom(NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "s"))), "Alias for read; reads a Transit+JSON string into Joker data.", "1.0"))
}

var write_ Proc = Proc{Fn: func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	return writeTransit(args[0])
}, Name: "write", Package: "std/transit"}

var writeVerbose_ Proc = Proc{Fn: func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	return writeTransitVerbose(args[0])
}, Name: "write-verbose", Package: "std/transit"}

var read_ Proc = Proc{Fn: func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	return readTransit(coretypes.EnsureArgIsString(args, 0))
}, Name: "read", Package: "std/transit"}
