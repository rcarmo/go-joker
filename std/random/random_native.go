package random

import (
	"crypto/rand"
	"encoding/hex"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math/big"
	mrand "math/rand/v2"

	. "github.com/rcarmo/go-joker/core"
)

// random_native.go — joker.random namespace.

var randomNamespace = GLOBAL_ENV.EnsureSymbolIsLib(MakeSymbol("joker.random"))

func init() {
	randomNamespace.Lazy = initRandomNamespace
}

func initRandomNamespace() {
	randomNamespace.ResetMeta(MakeMeta(nil, `Random number generation (math/rand/v2 + crypto/rand).`, "1.0"))

	// int — returns a non-negative random int
	randomNamespace.InternVar("int", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 0, 0)
		return coretypes.MakeInt(mrand.Int())
	}, Name: "random-int", Package: "std/random"},
		MakeMeta(NewListFrom(NewVectorFrom()), `Returns a non-negative random integer.`, "1.0"))

	// int-n — returns a random int in [0, n)
	randomNamespace.InternVar("int-n", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		n := EnsureArgIsInt(args, 0).I
		if n <= 0 {
			panic(RT.NewError("int-n: n must be positive"))
		}
		return coretypes.MakeInt(mrand.IntN(n))
	}, Name: "random-int-n", Package: "std/random"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("n"))), `Returns a random integer in [0, n).`, "1.0"))

	// int-between — returns a random int in [lo, hi)
	randomNamespace.InternVar("int-between", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 2, 2)
		lo := EnsureArgIsInt(args, 0).I
		hi := EnsureArgIsInt(args, 1).I
		if hi <= lo {
			panic(RT.NewError("int-between: hi must be > lo"))
		}
		delta := hi - lo
		if delta <= 0 {
			panic(RT.NewError("int-between: range is too large"))
		}
		return coretypes.MakeInt(lo + mrand.IntN(delta))
	}, Name: "random-int-between", Package: "std/random"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("lo"), MakeSymbol("hi"))),
			`Returns a random integer in [lo, hi).`, "1.0"))

	// float — returns a random float64 in [0.0, 1.0)
	randomNamespace.InternVar("float", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 0, 0)
		return coretypes.MakeDouble(mrand.Float64())
	}, Name: "random-float", Package: "std/random"},
		MakeMeta(NewListFrom(NewVectorFrom()), `Returns a random float in [0.0, 1.0).`, "1.0"))

	// boolean — returns a random boolean
	randomNamespace.InternVar("boolean", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 0, 0)
		return coretypes.MakeBoolean(mrand.IntN(2) == 1)
	}, Name: "random-boolean", Package: "std/random"},
		MakeMeta(NewListFrom(NewVectorFrom()), `Returns a random boolean.`, "1.0"))

	// choice — picks a random element from a seqable collection
	randomNamespace.InternVar("choice", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		coll := EnsureObjectIsSeqable(args[0], "choice requires a Seqable collection").Seq()
		var elems []Object
		for s := coll; !s.IsEmpty(); s = s.Rest() {
			elems = append(elems, s.First())
		}
		if len(elems) == 0 {
			return NIL
		}
		return elems[mrand.IntN(len(elems))]
	}, Name: "random-choice", Package: "std/random"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("coll"))),
			`Returns a random element from coll.`, "1.0"))

	// shuffle — returns a shuffled vector of elements from a seqable
	randomNamespace.InternVar("shuffle", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		coll := EnsureObjectIsSeqable(args[0], "shuffle requires a Seqable collection").Seq()
		var elems []Object
		for s := coll; !s.IsEmpty(); s = s.Rest() {
			elems = append(elems, s.First())
		}
		mrand.Shuffle(len(elems), func(i, j int) {
			elems[i], elems[j] = elems[j], elems[i]
		})
		return NewVectorFrom(elems...)
	}, Name: "random-shuffle", Package: "std/random"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("coll"))),
			`Returns a vector with elements of coll in random order.`, "1.0"))

	// uuid — returns a random UUID string (v4)
	randomNamespace.InternVar("uuid", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 0, 0)
		var b [16]byte
		_, err := rand.Read(b[:])
		if err != nil {
			panic(RT.NewError("uuid: " + err.Error()))
		}
		b[6] = (b[6] & 0x0f) | 0x40 // version 4
		b[8] = (b[8] & 0x3f) | 0x80 // variant 10
		s := hex.EncodeToString(b[:])
		return coretypes.MakeString(s[:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:])
	}, Name: "random-uuid", Package: "std/random"},
		MakeMeta(NewListFrom(NewVectorFrom()), `Returns a random UUID v4 string.`, "1.0"))

	// secure-bytes — returns n cryptographically random bytes as a hex string
	randomNamespace.InternVar("secure-bytes", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		n := EnsureArgIsInt(args, 0).I
		if n <= 0 {
			panic(RT.NewError("secure-bytes: n must be positive"))
		}
		b := make([]byte, n)
		_, err := rand.Read(b)
		if err != nil {
			panic(RT.NewError("secure-bytes: " + err.Error()))
		}
		return coretypes.MakeString(hex.EncodeToString(b))
	}, Name: "random-secure-bytes", Package: "std/random"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("n"))),
			`Returns n cryptographically random bytes as a hex string.`, "1.0"))

	// secure-int — returns a cryptographically random int in [0, n)
	randomNamespace.InternVar("secure-int", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		n := EnsureArgIsInt(args, 0).I
		if n <= 0 {
			panic(RT.NewError("secure-int: n must be positive"))
		}
		bigN := big.NewInt(int64(n))
		r, err := rand.Int(rand.Reader, bigN)
		if err != nil {
			panic(RT.NewError("secure-int: " + err.Error()))
		}
		return coretypes.MakeInt(int(r.Int64()))
	}, Name: "random-secure-int", Package: "std/random"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("n"))),
			`Returns a cryptographically random integer in [0, n).`, "1.0"))
}
