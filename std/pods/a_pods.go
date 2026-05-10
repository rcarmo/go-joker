package pods

import . "github.com/candid82/joker/core"

var podsNamespace = GLOBAL_ENV.EnsureSymbolIsLib(MakeSymbol("pods"))
var babashkaPodsNamespace = GLOBAL_ENV.EnsureSymbolIsLib(MakeSymbol("babashka.pods"))

func init() {
	podsNamespace.Lazy = Init
	babashkaPodsNamespace.Lazy = Init
}

func Init() {
	installPodsNamespace(podsNamespace)
	installPodsNamespace(babashkaPodsNamespace)
}

func installPodsNamespace(ns *Namespace) {
	ns.ResetMeta(MakeMeta(nil, "Babashka pods compatibility namespace.", "1.0"))
	ns.InternVar("load-pod", loadPod_, MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("path-or-name")), NewVectorFrom(MakeSymbol("path-or-name"), MakeSymbol("version-or-args"))), "Starts a Babashka pod process, sends describe, registers it, and returns its pod id.", "1.0"))
	ns.InternVar("bencode-encode", bencodeEncode_, MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"))), "Encodes a Joker value as bencode bytes, returned as a string.", "1.0"))
	ns.InternVar("bencode-decode", bencodeDecode_, MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("s"))), "Decodes a bencode string into Joker data.", "1.0"))
}

var loadPod_ Proc = Proc{Fn: func(args []Object) Object {
	return loadPod(args)
}, Name: "load-pod", Package: "std/pods"}

var bencodeEncode_ Proc = Proc{Fn: func(args []Object) Object {
	CheckArity(args, 1, 1)
	return MakeString(string(bencodeEncodeObject(args[0])))
}, Name: "bencode-encode", Package: "std/pods"}

var bencodeDecode_ Proc = Proc{Fn: func(args []Object) Object {
	CheckArity(args, 1, 1)
	return bencodeDecodeBytes([]byte(EnsureArgIsString(args, 0).S))
}, Name: "bencode-decode", Package: "std/pods"}
