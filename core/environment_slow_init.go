//go:build gen_code
// +build gen_code

package core

import coretypes "github.com/rcarmo/go-joker/core/types"

/*
Called by parse_init.go in an outer var block, this runs before any

	func init() as well as before func main(). InitEnv() and others are
	called at runtime to set some of these Values based on the current
	invocation.
*/
func NewEnv() *Env {
	features := collectionConstruction.NewEmptySet()
	features.Add(coretypes.MakeKeyword(STRINGS.Intern, "default"))
	features.Add(coretypes.MakeKeyword(STRINGS.Intern, "joker"))
	res := &Env{
		Namespaces: make(map[*string]*Namespace),
		Features:   features,
	}
	res.CoreNamespace = res.EnsureSymbolIsNamespace(SYMBOLS.joker_core)
	res.CoreNamespace.Meta = MakeMeta(nil, "Core library of Joker.", "1.0")
	res.NS_VAR = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "ns"))
	res.IN_NS_VAR = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "in-ns"))
	res.ns = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*ns*"))
	res.stdin = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*in*"))
	res.stdout = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*out*"))
	res.stderr = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*err*"))
	res.file = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*file*"))
	res.MainFile = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*main-file*"))
	res.version = res.CoreNamespace.InternVar("*joker-version*", versionMap(),
		MakeMeta(nil, `The version info for Clojure core, as a map containing :major :minor
			:incremental and :qualifier keys. Feature releases may increment
			:minor and/or :major, bugfix releases will increment :incremental.`, "1.0"))
	res.args = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*command-line-args*"))
	res.classPath = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*classpath*"))
	res.classPath.Value = NIL
	res.classPath.isPrivate = true
	res.printReadably = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*print-readably*"))
	res.printReadably.Value = coretypes.Boolean{B: true}
	res.CoreNamespace.InternVar("*repl*", coretypes.Boolean{B: false},
		MakeMeta(nil, "true if Joker is running in repl mode", "1.5"))
	res.CoreNamespace.InternVar("*linter-mode*", coretypes.Boolean{B: LINTER_MODE},
		MakeMeta(nil, "true if Joker is running in linter mode", "1.0"))
	res.CoreNamespace.InternVar("*linter-config*", collectionConstruction.NewEmptyArrayMap(),
		MakeMeta(nil, "Map of configuration key/value pairs for linter mode", "1.0"))
	res.libs = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*loaded-libs*"))
	res.libs.Value = collectionConstruction.NewEmptySet()
	res.libs.isPrivate = true
	return res
}

func (env *Env) ReferCoreToUser() {
	env.FindNamespace(coretypes.MakeSymbol(STRINGS.Intern, "user")).ReferAll(env.CoreNamespace)
}
