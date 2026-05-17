package math

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"math"

	. "github.com/rcarmo/go-joker/core"
)

// math_extra.go — additional math functions not in the generated a_math.go.

func init() {
	mathNamespace := GLOBAL_ENV.FindNamespace(MakeSymbol("joker.math"))
	if mathNamespace == nil {
		return
	}

	// tan — tangent
	mathNamespace.InternVar("tan", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return coretypes.MakeDouble(math.Tan(EnsureArgIsNumber(args, 0).Double().D))
	}, Name: "tan_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"))),
			`Returns the tangent of the radian argument x.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// asin — arcsine
	mathNamespace.InternVar("asin", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return coretypes.MakeDouble(math.Asin(EnsureArgIsNumber(args, 0).Double().D))
	}, Name: "asin_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"))),
			`Returns the arcsine (in radians) of x.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// acos — arccosine
	mathNamespace.InternVar("acos", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return coretypes.MakeDouble(math.Acos(EnsureArgIsNumber(args, 0).Double().D))
	}, Name: "acos_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"))),
			`Returns the arccosine (in radians) of x.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// atan — arctangent
	mathNamespace.InternVar("atan", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return coretypes.MakeDouble(math.Atan(EnsureArgIsNumber(args, 0).Double().D))
	}, Name: "atan_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"))),
			`Returns the arctangent (in radians) of x.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// atan2 — two-argument arctangent
	mathNamespace.InternVar("atan2", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		y := EnsureArgIsNumber(args, 0).Double().D
		x := EnsureArgIsNumber(args, 1).Double().D
		return coretypes.MakeDouble(math.Atan2(y, x))
	}, Name: "atan2_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("y"), MakeSymbol("x"))),
			`Returns the arc tangent of y/x, using the signs to determine the quadrant.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// sinh — hyperbolic sine
	mathNamespace.InternVar("sinh", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return coretypes.MakeDouble(math.Sinh(EnsureArgIsNumber(args, 0).Double().D))
	}, Name: "sinh_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"))),
			`Returns the hyperbolic sine of x.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// cosh — hyperbolic cosine
	mathNamespace.InternVar("cosh", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return coretypes.MakeDouble(math.Cosh(EnsureArgIsNumber(args, 0).Double().D))
	}, Name: "cosh_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"))),
			`Returns the hyperbolic cosine of x.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// tanh — hyperbolic tangent
	mathNamespace.InternVar("tanh", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return coretypes.MakeDouble(math.Tanh(EnsureArgIsNumber(args, 0).Double().D))
	}, Name: "tanh_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"))),
			`Returns the hyperbolic tangent of x.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// remainder — IEEE 754 floating-point remainder
	mathNamespace.InternVar("remainder", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		x := EnsureArgIsNumber(args, 0).Double().D
		y := EnsureArgIsNumber(args, 1).Double().D
		return coretypes.MakeDouble(math.Remainder(x, y))
	}, Name: "remainder_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"), MakeSymbol("y"))),
			`Returns the IEEE 754 floating-point remainder of x/y.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// fmod — floating-point modulus (same sign as x)
	mathNamespace.InternVar("fmod", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		x := EnsureArgIsNumber(args, 0).Double().D
		y := EnsureArgIsNumber(args, 1).Double().D
		return coretypes.MakeDouble(math.Mod(x, y))
	}, Name: "fmod_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"), MakeSymbol("y"))),
			`Returns the floating-point remainder of x/y (same sign as x).`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// max-val — numeric max of two values
	mathNamespace.InternVar("max-val", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		x := EnsureArgIsNumber(args, 0).Double().D
		y := EnsureArgIsNumber(args, 1).Double().D
		return coretypes.MakeDouble(math.Max(x, y))
	}, Name: "max_val_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"), MakeSymbol("y"))),
			`Returns the larger of x or y.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// min-val — numeric min of two values
	mathNamespace.InternVar("min-val", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		x := EnsureArgIsNumber(args, 0).Double().D
		y := EnsureArgIsNumber(args, 1).Double().D
		return coretypes.MakeDouble(math.Min(x, y))
	}, Name: "min_val_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"), MakeSymbol("y"))),
			`Returns the smaller of x or y.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// degrees — convert radians to degrees
	mathNamespace.InternVar("degrees", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return coretypes.MakeDouble(EnsureArgIsNumber(args, 0).Double().D * 180.0 / math.Pi)
	}, Name: "degrees_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"))),
			`Converts angle x from radians to degrees.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))

	// radians — convert degrees to radians
	mathNamespace.InternVar("radians", Proc{Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return coretypes.MakeDouble(EnsureArgIsNumber(args, 0).Double().D * math.Pi / 180.0)
	}, Name: "radians_", Package: "std/math"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("x"))),
			`Converts angle x from degrees to radians.`, "1.0").Plus(MakeKeyword("tag"), coretypes.String{S: "coretypes.Double"}))
}
