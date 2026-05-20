package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

// unchecked_arith.go — Unchecked arithmetic operations for Clojure parity.
//
// In Clojure JVM, unchecked-* ops bypass overflow checks and use
// primitive long arithmetic. In go-joker, all ints are Go int (64-bit
// on 64-bit platforms), so unchecked ops are identical to checked ops
// since Go integer arithmetic already wraps on overflow.

func init() {
	registerUncheckedArithProcs()
}

func registerUncheckedArithProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// All unchecked ops delegate to regular arithmetic since Go wraps on overflow.
	ops := []struct {
		name string
		fn   func([]coretypes.Object) coretypes.Object
	}{
		{"unchecked-add", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 2, 2)
			a := coretypes.EnsureArgIsInt(args, 0)
			b := coretypes.EnsureArgIsInt(args, 1)
			return coretypes.Int{I: a.I + b.I}
		}},
		{"unchecked-add-int", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 2, 2)
			a := coretypes.EnsureArgIsInt(args, 0)
			b := coretypes.EnsureArgIsInt(args, 1)
			return coretypes.Int{I: a.I + b.I}
		}},
		{"unchecked-subtract", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 2, 2)
			a := coretypes.EnsureArgIsInt(args, 0)
			b := coretypes.EnsureArgIsInt(args, 1)
			return coretypes.Int{I: a.I - b.I}
		}},
		{"unchecked-subtract-int", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 2, 2)
			a := coretypes.EnsureArgIsInt(args, 0)
			b := coretypes.EnsureArgIsInt(args, 1)
			return coretypes.Int{I: a.I - b.I}
		}},
		{"unchecked-multiply", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 2, 2)
			a := coretypes.EnsureArgIsInt(args, 0)
			b := coretypes.EnsureArgIsInt(args, 1)
			return coretypes.Int{I: a.I * b.I}
		}},
		{"unchecked-multiply-int", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 2, 2)
			a := coretypes.EnsureArgIsInt(args, 0)
			b := coretypes.EnsureArgIsInt(args, 1)
			return coretypes.Int{I: a.I * b.I}
		}},
		{"unchecked-divide-int", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 2, 2)
			a := coretypes.EnsureArgIsInt(args, 0)
			b := coretypes.EnsureArgIsInt(args, 1)
			if b.I == 0 {
				panic(coretypes.RuntimeError("Divide by zero"))
			}
			return coretypes.Int{I: a.I / b.I}
		}},
		{"unchecked-remainder-int", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 2, 2)
			a := coretypes.EnsureArgIsInt(args, 0)
			b := coretypes.EnsureArgIsInt(args, 1)
			if b.I == 0 {
				panic(coretypes.RuntimeError("Divide by zero"))
			}
			return coretypes.Int{I: a.I % b.I}
		}},
		{"unchecked-negate", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			a := coretypes.EnsureArgIsInt(args, 0)
			return coretypes.Int{I: -a.I}
		}},
		{"unchecked-negate-int", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			a := coretypes.EnsureArgIsInt(args, 0)
			return coretypes.Int{I: -a.I}
		}},
		{"unchecked-inc", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			a := coretypes.EnsureArgIsInt(args, 0)
			return coretypes.Int{I: a.I + 1}
		}},
		{"unchecked-inc-int", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			a := coretypes.EnsureArgIsInt(args, 0)
			return coretypes.Int{I: a.I + 1}
		}},
		{"unchecked-dec", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			a := coretypes.EnsureArgIsInt(args, 0)
			return coretypes.Int{I: a.I - 1}
		}},
		{"unchecked-dec-int", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			a := coretypes.EnsureArgIsInt(args, 0)
			return coretypes.Int{I: a.I - 1}
		}},
		// Type conversion (identity in go-joker since all ints are int)
		{"unchecked-int", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			return coretypes.EnsureArgIsNumber(args, 0).Int()
		}},
		{"unchecked-long", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			return coretypes.EnsureArgIsNumber(args, 0).Int()
		}},
		{"unchecked-short", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			return coretypes.EnsureArgIsNumber(args, 0).Int()
		}},
		{"unchecked-byte", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			n := coretypes.EnsureArgIsNumber(args, 0).Int()
			return coretypes.Int{I: n.I & 0xFF}
		}},
		{"unchecked-char", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			n := coretypes.EnsureArgIsNumber(args, 0).Int()
			return coretypes.Char{Ch: rune(n.I)}
		}},
		{"unchecked-float", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			return coretypes.EnsureArgIsNumber(args, 0).Double()
		}},
		{"unchecked-double", func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			return coretypes.EnsureArgIsNumber(args, 0).Double()
		}},
	}

	for _, op := range ops {
		sym := coretypes.MakeSymbol(STRINGS.Intern, op.name)
		vr := ns.Intern(sym)
		vr.Value = Proc{Name: "proc" + op.name, Fn: op.fn}
		referToUser(sym, vr)
	}

	// int-array, long-array, etc. — create vectors (no primitive arrays in go-joker)
	arrayOps := []string{"int-array", "long-array", "short-array", "byte-array",
		"char-array", "float-array", "double-array", "boolean-array", "object-array"}
	for _, name := range arrayOps {
		sym := coretypes.MakeSymbol(STRINGS.Intern, name)
		vr := ns.Intern(sym)
		vr.Value = Proc{Name: "proc" + name, Fn: func(args []coretypes.Object) coretypes.Object {
			switch len(args) {
			case 1:
				switch v := args[0].(type) {
				case coretypes.Int:
					// (int-array n) — create vector of n nils
					result := corecollections.EmptyArrayVector()
					for i := 0; i < v.I; i++ {
						result = result.Conj(NIL).(*corecollections.ArrayVector)
					}
					return result
				default:
					// (int-array coll) — create vector from collection
					s := coretypes.EnsureObjectIsSeqable(args[0], "array constructor requires a number or seqable").Seq()
					result := corecollections.EmptyArrayVector()
					for !s.IsEmpty() {
						result = result.Conj(s.First()).(*corecollections.ArrayVector)
						s = s.Rest()
					}
					return result
				}
			case 2:
				// (int-array n init-val-or-seq)
				n := coretypes.EnsureArgIsInt(args, 0)
				result := corecollections.EmptyArrayVector()
				if s, ok := args[1].(coretypes.Seqable); ok {
					seq := s.Seq()
					for i := 0; i < n.I && !seq.IsEmpty(); i++ {
						result = result.Conj(seq.First()).(*corecollections.ArrayVector)
						seq = seq.Rest()
					}
					for result.Count() < n.I {
						result = result.Conj(NIL).(*corecollections.ArrayVector)
					}
				} else {
					for i := 0; i < n.I; i++ {
						result = result.Conj(args[1]).(*corecollections.ArrayVector)
					}
				}
				return result
			default:
				PanicArityMinMax(len(args), 1, 2)
				return NIL
			}
		}}
		referToUser(sym, vr)
	}

	// make-array — (make-array type size)
	maVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "make-array"))
	maVr.Value = Proc{Name: "procMakeArray", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 1 {
			PanicArityMinMax(len(args), 1, 999)
		}
		// Ignore type argument, just use size
		var size int
		if len(args) >= 2 {
			size = coretypes.EnsureArgIsInt(args, 1).I
		}
		result := corecollections.EmptyArrayVector()
		for i := 0; i < size; i++ {
			result = result.Conj(NIL).(*corecollections.ArrayVector)
		}
		return result
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "make-array"), maVr)

	// aclone — (aclone arr) — clone array (vector in go-joker)
	acVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "aclone"))
	acVr.Value = Proc{Name: "procAclone", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return args[0] // vectors are already persistent/immutable
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "aclone"), acVr)

	// aset — (aset arr idx val) — set array element
	asVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "aset"))
	asVr.Value = Proc{Name: "procAset", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 3, 3)
		v := coretypes.EnsureObjectIsAssociative(args[0], "aset requires an associative collection")
		idx := args[1]
		val := args[2]
		return v.Assoc(idx, val).(coretypes.Object)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "aset"), asVr)

	// aget — (aget arr idx) — get array element
	agVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "aget"))
	agVr.Value = Proc{Name: "procAget", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		g, ok := args[0].(coretypes.Gettable)
		if !ok {
			panic(coretypes.RuntimeError("aget requires an indexed collection"))
		}
		if ok, v := g.Get(args[1]); ok {
			return v
		}
		return NIL
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "aget"), agVr)

	// alength — (alength arr)
	alVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "alength"))
	alVr.Value = Proc{Name: "procAlength", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		c, ok := args[0].(coretypes.Counted)
		if !ok {
			panic(coretypes.RuntimeError("alength requires a counted collection"))
		}
		return coretypes.Int{I: c.Count()}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "alength"), alVr)
}
