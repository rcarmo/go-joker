package core

// native_recursive.go — Native Go code generation for pure-integer recursive fns.
//
// When a fn body contains only integer arithmetic, comparisons, and self-recursive
// calls (no collections, strings, or other types), we compile to fixed-arity
// native Go functions that run without Object boxing, interface dispatch, or
// slice allocation per call.

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"sync"
)

// nativeIntFn1 through nativeIntFn3 are typed native function signatures.
type nativeIntFn1 func(a int) int
type nativeIntFn2 func(a, b int) int
type nativeIntFn3 func(a, b, c int) int

// nativeRecursiveEntry holds a compiled native fn for a specific arity.
type nativeRecursiveEntry struct {
	arity int
	fn1   nativeIntFn1
	fn2   nativeIntFn2
	fn3   nativeIntFn3
}

var nativeRecursiveCache sync.Map // *Fn → *nativeRecursiveEntry (or nativeRecursiveFailed sentinel)
var nativeRecursiveFailed = &nativeRecursiveEntry{arity: -1}

func tryNativeRecursive(fn *Fn) *nativeRecursiveEntry {
	if cached, ok := nativeRecursiveCache.Load(fn); ok {
		entry := cached.(*nativeRecursiveEntry)
		if entry == nativeRecursiveFailed {
			return nil
		}
		return entry
	}

	entry := compileNativeRecursive(fn)
	if entry == nil {
		nativeRecursiveCache.Store(fn, nativeRecursiveFailed)
	} else {
		nativeRecursiveCache.Store(fn, entry)
	}
	return entry
}

func compileNativeRecursive(fn *Fn) *nativeRecursiveEntry {
	if fn == nil || fn.fnExpr == nil || fn.defVar == nil {
		return nil
	}
	if len(fn.fnExpr.arities) != 1 || fn.fnExpr.variadic != nil {
		return nil
	}
	arity := fn.fnExpr.arities[0]
	nargs := len(arity.args)
	if nargs < 1 || nargs > 3 || len(arity.body) != 1 {
		return nil
	}

	selfVar := fn.defVar
	paramFrame := guessFnParamFrame(arity.body, nargs)
	if paramFrame < 0 {
		paramFrame = 1
	}

	entry := &nativeRecursiveEntry{arity: nargs}

	switch nargs {
	case 1:
		compiled := compileIntExpr1(arity.body[0], selfVar, paramFrame, entry)
		if compiled == nil {
			return nil
		}
		entry.fn1 = compiled
	case 2:
		compiled := compileIntExpr2(arity.body[0], selfVar, paramFrame, entry)
		if compiled == nil {
			return nil
		}
		entry.fn2 = compiled
	case 3:
		compiled := compileIntExpr3(arity.body[0], selfVar, paramFrame, entry)
		if compiled == nil {
			return nil
		}
		entry.fn3 = compiled
	}
	return entry
}

func callNativeRecursive(entry *nativeRecursiveEntry, args []Object) Object {
	switch entry.arity {
	case 1:
		a, ok := args[0].(coretypes.Int)
		if !ok {
			return nil
		}
		return coretypes.Int{I: entry.fn1(a.I)}
	case 2:
		a, aok := args[0].(coretypes.Int)
		b, bok := args[1].(coretypes.Int)
		if !aok || !bok {
			return nil
		}
		return coretypes.Int{I: entry.fn2(a.I, b.I)}
	case 3:
		a, aok := args[0].(coretypes.Int)
		b, bok := args[1].(coretypes.Int)
		c, cok := args[2].(coretypes.Int)
		if !aok || !bok || !cok {
			return nil
		}
		return coretypes.Int{I: entry.fn3(a.I, b.I, c.I)}
	}
	return nil
}

// --- Arity-1 compiler (fib) ---

type intBool1 func(a int) bool

func compileIntExpr1(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn1 {
	switch e := expr.(type) {
	case *LiteralExpr:
		if v, ok := e.obj.(coretypes.Int); ok {
			val := v.I
			return func(a int) int { return val }
		}
	case *BindingExpr:
		if e.binding.frame == pf && e.binding.index == 0 {
			return func(a int) int { return a }
		}
	case *IfExpr:
		cond := compileIntBool1(e.cond, selfVar, pf, entry)
		pos := compileIntExpr1(e.positive, selfVar, pf, entry)
		neg := compileIntExpr1(e.negative, selfVar, pf, entry)
		if cond == nil || pos == nil || neg == nil {
			return nil
		}
		return func(a int) int {
			if cond(a) {
				return pos(a)
			}
			return neg(a)
		}
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok && vref.vr == selfVar && len(e.args) == 1 {
			arg := compileIntExpr1(e.args[0], selfVar, pf, entry)
			if arg == nil {
				return nil
			}
			return func(a int) int { return entry.fn1(arg(a)) }
		}
		if vref, ok := e.callable.(*VarRefExpr); ok {
			return compileArith1(coreVarToProcName(vref.vr), e.args, selfVar, pf, entry)
		}
	}
	return nil
}

func compileIntBool1(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) intBool1 {
	e, ok := expr.(*CallExpr)
	if !ok {
		return nil
	}
	vref, ok := e.callable.(*VarRefExpr)
	if !ok || len(e.args) != 2 {
		return nil
	}
	a := compileIntExpr1(e.args[0], selfVar, pf, entry)
	b := compileIntExpr1(e.args[1], selfVar, pf, entry)
	if a == nil || b == nil {
		return nil
	}
	switch coreVarToProcName(vref.vr) {
	case "procLt":
		return func(x int) bool { return a(x) < b(x) }
	case "procLte":
		return func(x int) bool { return a(x) <= b(x) }
	case "procGt":
		return func(x int) bool { return a(x) > b(x) }
	case "procGte":
		return func(x int) bool { return a(x) >= b(x) }
	case "procEq":
		return func(x int) bool { return a(x) == b(x) }
	}
	return nil
}

func compileArith1(proc string, args []Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn1 {
	switch proc {
	case "procAdd":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		b := compileIntExpr1(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x int) int { return a(x) + b(x) }
	case "procSubtract":
		if len(args) == 1 {
			a := compileIntExpr1(args[0], selfVar, pf, entry)
			if a == nil {
				return nil
			}
			return func(x int) int { return -a(x) }
		}
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		b := compileIntExpr1(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x int) int { return a(x) - b(x) }
	case "procMultiply":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		b := compileIntExpr1(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x int) int { return a(x) * b(x) }
	case "procInc":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x int) int { return a(x) + 1 }
	case "procDec":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x int) int { return a(x) - 1 }
	}
	return nil
}

// --- Arity-3 compiler (tak) ---

type intBool3 func(a, b, c int) bool

func compileIntExpr3(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn3 {
	switch e := expr.(type) {
	case *LiteralExpr:
		if v, ok := e.obj.(coretypes.Int); ok {
			val := v.I
			return func(a, b, c int) int { return val }
		}
	case *BindingExpr:
		if e.binding.frame == pf {
			switch e.binding.index {
			case 0:
				return func(a, b, c int) int { return a }
			case 1:
				return func(a, b, c int) int { return b }
			case 2:
				return func(a, b, c int) int { return c }
			}
		}
	case *IfExpr:
		cond := compileIntBool3(e.cond, selfVar, pf, entry)
		pos := compileIntExpr3(e.positive, selfVar, pf, entry)
		neg := compileIntExpr3(e.negative, selfVar, pf, entry)
		if cond == nil || pos == nil || neg == nil {
			return nil
		}
		return func(a, b, c int) int {
			if cond(a, b, c) {
				return pos(a, b, c)
			}
			return neg(a, b, c)
		}
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok && vref.vr == selfVar && len(e.args) == 3 {
			x := compileIntExpr3(e.args[0], selfVar, pf, entry)
			y := compileIntExpr3(e.args[1], selfVar, pf, entry)
			z := compileIntExpr3(e.args[2], selfVar, pf, entry)
			if x == nil || y == nil || z == nil {
				return nil
			}
			return func(a, b, c int) int { return entry.fn3(x(a, b, c), y(a, b, c), z(a, b, c)) }
		}
		if vref, ok := e.callable.(*VarRefExpr); ok {
			return compileArith3(coreVarToProcName(vref.vr), e.args, selfVar, pf, entry)
		}
	}
	return nil
}

func compileIntBool3(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) intBool3 {
	e, ok := expr.(*CallExpr)
	if !ok {
		return nil
	}
	vref, ok := e.callable.(*VarRefExpr)
	if !ok || len(e.args) != 2 {
		return nil
	}
	a := compileIntExpr3(e.args[0], selfVar, pf, entry)
	b := compileIntExpr3(e.args[1], selfVar, pf, entry)
	if a == nil || b == nil {
		return nil
	}
	switch coreVarToProcName(vref.vr) {
	case "procLt":
		return func(x, y, z int) bool { return a(x, y, z) < b(x, y, z) }
	case "procLte":
		return func(x, y, z int) bool { return a(x, y, z) <= b(x, y, z) }
	case "procGt":
		return func(x, y, z int) bool { return a(x, y, z) > b(x, y, z) }
	case "procGte":
		return func(x, y, z int) bool { return a(x, y, z) >= b(x, y, z) }
	case "procEq":
		return func(x, y, z int) bool { return a(x, y, z) == b(x, y, z) }
	}
	return nil
}

func compileArith3(proc string, args []Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn3 {
	switch proc {
	case "procAdd":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		b := compileIntExpr3(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y, z int) int { return a(x, y, z) + b(x, y, z) }
	case "procSubtract":
		if len(args) == 1 {
			a := compileIntExpr3(args[0], selfVar, pf, entry)
			if a == nil {
				return nil
			}
			return func(x, y, z int) int { return -a(x, y, z) }
		}
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		b := compileIntExpr3(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y, z int) int { return a(x, y, z) - b(x, y, z) }
	case "procMultiply":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		b := compileIntExpr3(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y, z int) int { return a(x, y, z) * b(x, y, z) }
	case "procInc":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y, z int) int { return a(x, y, z) + 1 }
	case "procDec":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y, z int) int { return a(x, y, z) - 1 }
	}
	return nil
}

// --- Arity-2 compiler ---

type intBool2 func(a, b int) bool

func compileIntExpr2(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn2 {
	switch e := expr.(type) {
	case *LiteralExpr:
		if v, ok := e.obj.(coretypes.Int); ok {
			val := v.I
			return func(a, b int) int { return val }
		}
	case *BindingExpr:
		if e.binding.frame == pf {
			switch e.binding.index {
			case 0:
				return func(a, b int) int { return a }
			case 1:
				return func(a, b int) int { return b }
			}
		}
	case *IfExpr:
		cond := compileIntBool2(e.cond, selfVar, pf, entry)
		pos := compileIntExpr2(e.positive, selfVar, pf, entry)
		neg := compileIntExpr2(e.negative, selfVar, pf, entry)
		if cond == nil || pos == nil || neg == nil {
			return nil
		}
		return func(a, b int) int {
			if cond(a, b) {
				return pos(a, b)
			}
			return neg(a, b)
		}
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok && vref.vr == selfVar && len(e.args) == 2 {
			x := compileIntExpr2(e.args[0], selfVar, pf, entry)
			y := compileIntExpr2(e.args[1], selfVar, pf, entry)
			if x == nil || y == nil {
				return nil
			}
			return func(a, b int) int { return entry.fn2(x(a, b), y(a, b)) }
		}
		if vref, ok := e.callable.(*VarRefExpr); ok {
			return compileArith2(coreVarToProcName(vref.vr), e.args, selfVar, pf, entry)
		}
	}
	return nil
}

func compileIntBool2(expr Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) intBool2 {
	e, ok := expr.(*CallExpr)
	if !ok {
		return nil
	}
	vref, ok := e.callable.(*VarRefExpr)
	if !ok || len(e.args) != 2 {
		return nil
	}
	a := compileIntExpr2(e.args[0], selfVar, pf, entry)
	b := compileIntExpr2(e.args[1], selfVar, pf, entry)
	if a == nil || b == nil {
		return nil
	}
	switch coreVarToProcName(vref.vr) {
	case "procLt":
		return func(x, y int) bool { return a(x, y) < b(x, y) }
	case "procLte":
		return func(x, y int) bool { return a(x, y) <= b(x, y) }
	case "procGt":
		return func(x, y int) bool { return a(x, y) > b(x, y) }
	case "procGte":
		return func(x, y int) bool { return a(x, y) >= b(x, y) }
	case "procEq":
		return func(x, y int) bool { return a(x, y) == b(x, y) }
	}
	return nil
}

func compileArith2(proc string, args []Expr, selfVar *Var, pf int, entry *nativeRecursiveEntry) nativeIntFn2 {
	switch proc {
	case "procAdd":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		b := compileIntExpr2(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y int) int { return a(x, y) + b(x, y) }
	case "procSubtract":
		if len(args) == 1 {
			a := compileIntExpr2(args[0], selfVar, pf, entry)
			if a == nil {
				return nil
			}
			return func(x, y int) int { return -a(x, y) }
		}
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		b := compileIntExpr2(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y int) int { return a(x, y) - b(x, y) }
	case "procMultiply":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		b := compileIntExpr2(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y int) int { return a(x, y) * b(x, y) }
	case "procInc":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y int) int { return a(x, y) + 1 }
	case "procDec":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y int) int { return a(x, y) - 1 }
	}
	return nil
}
