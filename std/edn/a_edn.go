package edn

import (
	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

var ednNamespace = GLOBAL_ENV.EnsureSymbolIsLib(coretypes.MakeSymbol(STRINGS.Intern, "joker.edn"))
var ednAliasNamespace = GLOBAL_ENV.EnsureSymbolIsLib(coretypes.MakeSymbol(STRINGS.Intern, "edn"))

func init() {
	ednNamespace.Lazy = Init
	ednAliasNamespace.Lazy = InitAlias
}

func Init() {
	ednNamespace.ResetMeta(MakeMeta(nil, "EDN reader and writer for Joker values.", "1.0"))
	ednNamespace.InternVar("read-string", readString_, MakeMeta(corecollections.NewListFrom(corecollections.NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "s")), corecollections.NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "opts"), coretypes.MakeSymbol(STRINGS.Intern, "s"))), "Reads one EDN value from s without evaluating it. Options are accepted for Babashka compatibility; custom reader functions are resolved through Joker data readers.", "1.0"))
	ednNamespace.InternVar("write-string", writeString_, MakeMeta(corecollections.NewListFrom(corecollections.NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "x")), corecollections.NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "opts"), coretypes.MakeSymbol(STRINGS.Intern, "x"))), "Writes x as an EDN string. Options are currently accepted for compatibility and ignored.", "1.0"))
}

func InitAlias() {
	Init()
	ednAliasNamespace.ResetMeta(MakeMeta(nil, "Alias namespace for joker.edn.", "1.0"))
	ednAliasNamespace.InternVar("read-string", readString_, MakeMeta(corecollections.NewListFrom(corecollections.NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "s")), corecollections.NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "opts"), coretypes.MakeSymbol(STRINGS.Intern, "s"))), "Reads one EDN value from s without evaluating it.", "1.0"))
	ednAliasNamespace.InternVar("write-string", writeString_, MakeMeta(corecollections.NewListFrom(corecollections.NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "x")), corecollections.NewVectorFrom(coretypes.MakeSymbol(STRINGS.Intern, "opts"), coretypes.MakeSymbol(STRINGS.Intern, "x"))), "Writes x as an EDN string.", "1.0"))
}

var readString_ Proc = Proc{Fn: func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 2)
	return readEDNString(coretypes.EnsureArgIsString(args, len(args)-1).S)
}, Name: "read-string", Package: "std/edn"}

var writeString_ Proc = Proc{Fn: func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 2)
	return writeEDNString(args[len(args)-1])
}, Name: "write-string", Package: "std/edn"}
