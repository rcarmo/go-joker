package system

import . "github.com/rcarmo/go-joker/core"

var systemNamespace = GLOBAL_ENV.EnsureSymbolIsLib(MakeSymbol("System"))

func init() { systemNamespace.Lazy = Init }

func Init() {
	systemNamespace.ResetMeta(MakeMeta(nil, "JVM-shaped System compatibility namespace.", "1.0"))
	systemNamespace.InternVar("getProperty", getProperty_, MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("key")), NewVectorFrom(MakeSymbol("key"), MakeSymbol("default"))), "Returns a system property by string key, or optional default/nil.", "1.0"))
	systemNamespace.InternVar("getProperties", getProperties_, MakeMeta(NewListFrom(EmptyVector()), "Returns all System compatibility properties as a map.", "1.0"))
	systemNamespace.InternVar("getenv", getenv_, MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("name"))), "Returns an environment variable value, or nil.", "1.0"))
	systemNamespace.InternVar("exit", exit_, MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("code"))), "Exits the process with integer code.", "1.0"))
	systemNamespace.InternVar("lineSeparator", lineSeparator_, MakeMeta(NewListFrom(EmptyVector()), "Returns the platform line separator.", "1.0"))
	systemNamespace.InternVar("currentTimeMillis", currentTimeMillis_, MakeMeta(NewListFrom(EmptyVector()), "Returns current Unix time in milliseconds.", "1.0"))
	systemNamespace.InternVar("nanoTime", nanoTime_, MakeMeta(NewListFrom(EmptyVector()), "Returns monotonic-ish current time in nanoseconds.", "1.0"))
}

var getProperty_ Proc = Proc{Fn: func(args []Object) Object {
	return getProperty(args)
}, Name: "getProperty", Package: "std/system"}

var getProperties_ Proc = Proc{Fn: func(args []Object) Object {
	CheckArity(args, 0, 0)
	return systemProperties()
}, Name: "getProperties", Package: "std/system"}

var getenv_ Proc = Proc{Fn: func(args []Object) Object {
	return systemGetenv(args)
}, Name: "getenv", Package: "std/system"}

var exit_ Proc = Proc{Fn: func(args []Object) Object {
	return systemExit(args)
}, Name: "exit", Package: "std/system"}

var lineSeparator_ Proc = Proc{Fn: func(args []Object) Object {
	CheckArity(args, 0, 0)
	return MakeString(lineSeparator())
}, Name: "lineSeparator", Package: "std/system"}

var currentTimeMillis_ Proc = Proc{Fn: func(args []Object) Object {
	CheckArity(args, 0, 0)
	return currentTimeMillis()
}, Name: "currentTimeMillis", Package: "std/system"}

var nanoTime_ Proc = Proc{Fn: func(args []Object) Object {
	CheckArity(args, 0, 0)
	return nanoTime()
}, Name: "nanoTime", Package: "std/system"}
