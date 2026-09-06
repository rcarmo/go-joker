//go:generate go run gen/gen_types.go assert coretypes.Comparable coretypes.Vec coretypes.Char coretypes.String coretypes.Symbol coretypes.Keyword *coretypes.Regex coretypes.Boolean coretypes.Time coretypes.Number coretypes.Seqable coretypes.Callable *coretypes.Type coretypes.Meta coretypes.Int coretypes.Double coretypes.Stack coretypes.Map coretypes.Set coretypes.Associative coretypes.Reversible coretypes.Named coretypes.Comparator *coretypes.Ratio *coretypes.BigFloat *coretypes.BigInt *Namespace *Var coretypes.Error *Fn coretypes.Deref *corert.Atom coretypes.Ref coretypes.KVReduce coretypes.Reduce coretypes.Pending *corert.File io.Reader io.Writer coretypes.StringReader io.RuneReader *corert.ObjectChannel coretypes.CountedIndexed
//go:generate go run gen/gen_types.go info *corecollections.List *corecollections.ArrayMapSeq *corecollections.ArrayMap *corecollections.HashMap *ExInfo *Fn *Var Nil *corecollections.LazySeq *corecollections.MappingSeq *corecollections.ArraySeq *corecollections.ConsSeq *corecollections.NodeSeq *corecollections.ArrayNodeSeq *corecollections.MapSet *corecollections.Vector *corecollections.ArrayVector *corecollections.VectorSeq *corecollections.VectorRSeq
//go:generate go run -tags gen_code gen/codegen/main.go

package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/rcarmo/go-joker/core/deps"
	coregenerated "github.com/rcarmo/go-joker/core/generated"
	coreir "github.com/rcarmo/go-joker/core/ir"
	coreirx "github.com/rcarmo/go-joker/core/ir"
	"github.com/rcarmo/go-joker/core/osutil"
	corereader "github.com/rcarmo/go-joker/core/reader"
	coretrace "github.com/rcarmo/go-joker/core/trace"
	"github.com/rcarmo/go-joker/core/types/numerical"
	corewasm "github.com/rcarmo/go-joker/core/wasm"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"io"
	"maps"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
	"unsafe"

	coretypes "github.com/rcarmo/go-joker/core/types"

	"github.com/rcarmo/go-joker/core/hashutil"
	corert "github.com/rcarmo/go-joker/core/runtime"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

type (
	Traceable = corert.Traceable
	Runtime   struct{}
)

var RT *Runtime = &Runtime{}

// An unsupported executor may return nil, but a language failure is observable
// and must not be converted into a retry of an already-started computation.
// Preserve the original language error/type while marking callable-origin
// failures as non-speculative across executor recovery boundaries.
type irLanguageError = coretypes.Error
type irCallbackError struct{ irLanguageError }

func rethrowIRLanguageFailure(failure interface{}) {
	if _, ok := failure.(*irCallbackError); ok {
		panic(failure)
	}
	if _, ok := failure.(*ExInfo); ok {
		panic(failure)
	}
	// Numeric primitives currently use this string panic for zero divisors.
	// Other executor failures still need a typed unsupported/error protocol.
	if message, ok := failure.(string); ok && message == "Division by zero" {
		panic(failure)
	}
}

func Eval(expr Expr, env *LocalEnv) coretypes.Object {
	switch expr := expr.(type) {
	case *LiteralExpr:
		return expr.obj
	case *BindingExpr:
		return resolveBinding(env, expr.binding)
	case *VarRefExpr:
		return expr.vr.Resolve()
	}
	grt := currentGRT()
	parentExpr := grt.CurrentExpr
	grt.CurrentExpr = expr
	defer restoreCurrentExpr(parentExpr)
	switch expr := expr.(type) {
	case *IfExpr:
		switch cond := Eval(expr.cond, env).(type) {
		case Nil:
			return Eval(expr.negative, env)
		case coretypes.Boolean:
			if cond.B {
				return Eval(expr.positive, env)
			}
			return Eval(expr.negative, env)
		default:
			return Eval(expr.positive, env)
		}
	case *DoExpr:
		return evalBody(expr.body, env)
	case *FnExpr:
		res := &Fn{fnExpr: expr}
		if expr.self.NameKey() != nil {
			selfEnv := LocalEnv{bindings: []coretypes.Object{res}, parent: env}
			if env != nil {
				selfEnv.frame = env.frame + 1
			}
			res.env = &selfEnv
			return res
		}
		res.env = env
		return res
	case *LetExpr:
		childEnv := LocalEnv{bindings: make([]coretypes.Object, 0, len(expr.names)), parent: env}
		if env != nil {
			childEnv.frame = env.frame + 1
		}
		for _, bindingExpr := range expr.values {
			childEnv.bindings = append(childEnv.bindings, Eval(bindingExpr, &childEnv))
		}
		return evalBody(expr.body, &childEnv)
	case *LoopExpr:
		childEnv := LocalEnv{bindings: make([]coretypes.Object, 0, len(expr.names)), parent: env}
		if env != nil {
			childEnv.frame = env.frame + 1
		}
		for _, bindingExpr := range expr.values {
			childEnv.bindings = append(childEnv.bindings, Eval(bindingExpr, &childEnv))
		}
		// Try WASM native path first (pure integer loops)
		if prog := irGetCached(expr); prog != nil {
			if wp := wasmGetCached(prog); wp != nil {
				if result := wasmExec(wp, childEnv.bindings); result != nil {
					return result
				}
			}
			// Try IR fast path
			initSlots := childEnv.bindings
			// Resolve captured outer bindings
			if len(prog.captureKeys) > 0 {
				full := make([]coretypes.Object, prog.numSlots)
				copy(full, initSlots)
				for i, key := range prog.captureKeys {
					slot := len(initSlots) + i
					e := env
					for e != nil && e.frame > key.frame {
						e = e.parent
					}
					if e != nil && key.index < len(e.bindings) {
						full[slot] = e.bindings[key.index]
					}
				}
				initSlots = full
			}
			if runtimeExec.CanTryMemNth(prog) && wasmMemNthStaticEligible(prog) {
				if result := wasmMemNthCompileAndExec(prog, initSlots); result != nil {
					return result
				}
				runtimeExec.MarkMemNthFailed(prog)
			}
			if corert.IRTypedEnabled() && runtimeExec.CanExecuteTypedIR(prog) {
				var typedResult coretypes.Object
				func() {
					epoch := currentGRT().CallableEpoch
					defer func() {
						if r := recover(); r != nil {
							if currentGRT().CallableEpoch != epoch {
								panic(r)
							}
							rethrowIRLanguageFailure(r)
							typedResult = nil
						}
					}()
					typedResult = irExecTypedNB(prog, initSlots)
					if typedResult == nil {
						typedResult = irExecTyped(prog, initSlots)
					}
				}()
				if typedResult != nil {
					return typedResult
				}
				runtimeExec.MarkTypedExecutionFailed(prog)
			}
			if wp := wasmGetCachedWithOneHelper(prog, initSlots); wp != nil {
				if result := wasmExec(wp, initSlots); result != nil {
					return result
				}
			}
			if result := wasmMemNthCompileAndExec(prog, initSlots); result != nil {
				return result
			}
			var result coretypes.Object
			func() {
				epoch := currentGRT().CallableEpoch
				defer func() {
					if r := recover(); r != nil {
						if currentGRT().CallableEpoch != epoch {
							panic(r)
						}
						rethrowIRLanguageFailure(r)
						result = nil
					}
				}()
				result = irExec(prog, initSlots)
			}()
			if result != nil {
				return result
			}
			// IR execution failed at runtime; mark as non-compilable
			irCache.Store(expr, irCompileFailed)
		}
		return evalLoop(expr.body, &childEnv)
	default:
		return expr.Eval(env)
	}
}

func (expr *VarRefExpr) Eval(env *LocalEnv) coretypes.Object {
	return expr.vr.Resolve()
}

func (expr *SetMacroExpr) Eval(env *LocalEnv) coretypes.Object {
	expr.vr.isMacro = true
	expr.vr.isUsed = false
	if fn, ok := expr.vr.Value.(*Fn); ok {
		fn.isMacro = true
	}
	setMacroMeta(expr.vr)
	return expr.vr
}

func resolveBinding(env *LocalEnv, binding *Binding) coretypes.Object {
	if env.frame == binding.frame {
		return env.bindings[binding.index]
	}
	if env.parent != nil && env.frame == binding.frame+1 {
		return env.parent.bindings[binding.index]
	}
	for i := env.frame; i > binding.frame; i-- {
		env = env.parent
	}
	return env.bindings[binding.index]
}

func (expr *BindingExpr) Eval(env *LocalEnv) coretypes.Object {
	return resolveBinding(env, expr.binding)
}

func (expr *LiteralExpr) Eval(env *LocalEnv) coretypes.Object {
	return expr.obj
}

func (expr *VectorExpr) Eval(env *LocalEnv) coretypes.Object {
	n := len(expr.v)
	if n == 0 {
		return corecollections.EmptyArrayVector()
	}
	arr := make([]coretypes.Object, n)
	for i, e := range expr.v {
		arr[i] = Eval(e, env)
	}
	return &corecollections.ArrayVector{Arr: arr}
}

func (expr *MapExpr) Eval(env *LocalEnv) coretypes.Object {
	if int64(len(expr.keys)) > corecollections.HASHMAP_THRESHOLD/2 {
		res := corecollections.EmptyHashMap
		for i := range expr.keys {
			key := Eval(expr.keys[i], env)
			if res.ContainsKey(key) {
				panic(RT.NewError("Duplicate key: " + key.ToString(false)))
			}
			res = res.Assoc(key, Eval(expr.values[i], env)).(*corecollections.HashMap)
		}
		return res
	}
	res := corecollections.EmptyArrayMap()
	for i := range expr.keys {
		key := Eval(expr.keys[i], env)
		if !res.Add(key, Eval(expr.values[i], env)) {
			panic(RT.NewError("Duplicate key: " + key.ToString(false)))
		}
	}
	return res
}

func (expr *SetExpr) Eval(env *LocalEnv) coretypes.Object {
	res := corecollections.EmptySet()
	for _, elemExpr := range expr.elements {
		el := Eval(elemExpr, env)
		if !res.Add(el) {
			panic(RT.NewError("Duplicate set element: " + el.ToString(false)))
		}
	}
	return res
}

func (expr *DefExpr) Eval(env *LocalEnv) coretypes.Object {
	if expr.value != nil {
		expr.vr.Value = Eval(expr.value, env)
		// Track the def var on Fn values for var-based self-recursive IR dispatch
		if fn, ok := expr.vr.Value.(*Fn); ok {
			fn.defVar = expr.vr
		}
	}
	meta := corecollections.EmptyArrayMap()
	meta.Add(KEYWORDS.line, coretypes.Int{I: expr.StartLine})
	meta.Add(KEYWORDS.column, coretypes.Int{I: expr.StartColumn})
	meta.Add(KEYWORDS.file, coretypes.String{S: *expr.Filename})
	meta.Add(KEYWORDS.ns, expr.vr.ns)
	meta.Add(KEYWORDS.name, expr.vr.name)
	expr.vr.Meta = meta
	if expr.meta != nil {
		expr.vr.Meta = expr.vr.Meta.Merge(Eval(expr.meta, env).(coretypes.Map))
	}
	// isMacro can be set by set-macro__ during parse stage
	if expr.vr.isMacro {
		expr.vr.Meta = expr.vr.Meta.Assoc(KEYWORDS.macro, coretypes.Boolean{B: true}).(coretypes.Map)
	}
	return expr.vr
}

func (expr *MetaExpr) Eval(env *LocalEnv) coretypes.Object {
	meta := Eval(expr.meta, env)
	res := Eval(expr.expr, env)
	return res.(coretypes.Meta).WithMeta(meta.(coretypes.Map))
}

func evalSeq(exprs []Expr, env *LocalEnv) []coretypes.Object {
	res := make([]coretypes.Object, len(exprs))
	for i, expr := range exprs {
		res[i] = Eval(expr, env)
	}
	return res
}

func (expr *CallExpr) Eval(env *LocalEnv) coretypes.Object {
	if result, ok := evalReducePipelineFast(expr, env); ok {
		return result
	}
	// Fast path: if callable is a VarRefExpr, inline the var resolution
	// to avoid the full Eval dispatch overhead on hot recursive paths.
	var callable coretypes.Object
	if vref, ok := expr.callable.(*VarRefExpr); ok {
		callable = vref.vr.Value
		if callable == nil {
			callable = NIL
		}
	} else {
		callable = Eval(expr.callable, env)
	}
	switch callable := callable.(type) {
	case Proc:
		if callable.Name == "procReduce" && (len(expr.args) == 2 || len(expr.args) == 3) {
			f := Eval(expr.args[0], env)
			if fn, ok := f.(coretypes.Callable); ok {
				if len(expr.args) == 3 {
					init := Eval(expr.args[1], env)
					coll := Eval(expr.args[2], env)
					if r, ok := coll.(coretypes.Reduce); ok {
						return r.ReduceInit(fn, init)
					}
				} else {
					coll := Eval(expr.args[1], env)
					if r, ok := coll.(coretypes.Reduce); ok {
						return r.Reduce(fn)
					}
				}
			}
		}
		switch len(expr.args) {
		case 0:
			return callable.Fn(nil)
		case 1:
			arg := Eval(expr.args[0], env)
			switch callable.Name {
			case "procInc":
				switch x := arg.(type) {
				case coretypes.Int:
					return coretypes.INT_OPS.Add(x, coretypes.MakeInt(1))
				case coretypes.Double:
					return coretypes.Double{D: x.D + 1}
				}
			case "procDec":
				switch x := arg.(type) {
				case coretypes.Int:
					return coretypes.INT_OPS.Subtract(x, coretypes.MakeInt(1))
				case coretypes.Double:
					return coretypes.Double{D: x.D - 1}
				}
			case "procIsZero":
				switch x := arg.(type) {
				case coretypes.Int:
					return coretypes.Boolean{B: x.I == 0}
				case coretypes.Double:
					return coretypes.Boolean{B: x.D == 0}
				}
			case "procSubtract":
				switch x := arg.(type) {
				case coretypes.Int:
					return coretypes.INT_OPS.Subtract(coretypes.MakeInt(0), x)
				case coretypes.Double:
					return coretypes.Double{D: -x.D}
				}
			}
			var args [1]coretypes.Object
			args[0] = arg
			return callable.Fn(args[:])
		case 2:
			ax, bx := Eval(expr.args[0], env), Eval(expr.args[1], env)
			switch callable.Name {
			case "procGet":
				coll := ax
				key := bx
				switch c := coll.(type) {
				case coretypes.Gettable:
					ok, v := c.Get(key)
					if ok {
						return v
					}
					return NIL
				}
			case "procAdd":
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.INT_OPS.Add(a, b)
					case coretypes.Double:
						return coretypes.Double{D: float64(a.I) + b.D}
					}
				case coretypes.Double:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Double{D: a.D + float64(b.I)}
					case coretypes.Double:
						return coretypes.Double{D: a.D + b.D}
					}
				}
			case "procSubtract":
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.INT_OPS.Subtract(a, b)
					case coretypes.Double:
						return coretypes.Double{D: float64(a.I) - b.D}
					}
				case coretypes.Double:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Double{D: a.D - float64(b.I)}
					case coretypes.Double:
						return coretypes.Double{D: a.D - b.D}
					}
				}
			case "procMultiply":
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.INT_OPS.Multiply(a, b)
					case coretypes.Double:
						return coretypes.Double{D: float64(a.I) * b.D}
					}
				case coretypes.Double:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Double{D: a.D * float64(b.I)}
					case coretypes.Double:
						return coretypes.Double{D: a.D * b.D}
					}
				}
			case "procRem":
				if a, ok := ax.(coretypes.Int); ok {
					if b, ok := bx.(coretypes.Int); ok {
						if b.I == 0 {
							coretypes.PanicOnZero(coretypes.INT_OPS, b)
						}
						return coretypes.Int{I: a.I % b.I}
					}
				}
			case "procDivide":
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.INT_OPS.Divide(a, b)
					case coretypes.Double:
						return coretypes.Double{D: float64(a.I) / b.D}
					}
				case coretypes.Double:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Double{D: a.D / float64(b.I)}
					case coretypes.Double:
						return coretypes.Double{D: a.D / b.D}
					}
				}
			case "procLt":
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Boolean{B: a.I < b.I}
					case coretypes.Double:
						return coretypes.Boolean{B: float64(a.I) < b.D}
					}
				case coretypes.Double:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Boolean{B: a.D < float64(b.I)}
					case coretypes.Double:
						return coretypes.Boolean{B: a.D < b.D}
					}
				}
			case "procEq":
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Boolean{B: a.I == b.I}
					case coretypes.Double:
						return coretypes.Boolean{B: float64(a.I) == b.D}
					}
				case coretypes.Double:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Boolean{B: a.D == float64(b.I)}
					case coretypes.Double:
						return coretypes.Boolean{B: a.D == b.D}
					}
				}
			case "procGt":
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Boolean{B: a.I > b.I}
					case coretypes.Double:
						return coretypes.Boolean{B: float64(a.I) > b.D}
					}
				case coretypes.Double:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Boolean{B: a.D > float64(b.I)}
					case coretypes.Double:
						return coretypes.Boolean{B: a.D > b.D}
					}
				}
			case "procGte":
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Boolean{B: a.I >= b.I}
					case coretypes.Double:
						return coretypes.Boolean{B: float64(a.I) >= b.D}
					}
				case coretypes.Double:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Boolean{B: a.D >= float64(b.I)}
					case coretypes.Double:
						return coretypes.Boolean{B: a.D >= b.D}
					}
				}
			case "procLte":
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Boolean{B: a.I <= b.I}
					case coretypes.Double:
						return coretypes.Boolean{B: float64(a.I) <= b.D}
					}
				case coretypes.Double:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Boolean{B: a.D <= float64(b.I)}
					case coretypes.Double:
						return coretypes.Boolean{B: a.D <= b.D}
					}
				}
			}
			var args [2]coretypes.Object
			args[0] = ax
			args[1] = bx
			return callable.Fn(args[:])
		case 3:
			switch callable.Name {
			case "procGet":
				coll := Eval(expr.args[0], env)
				key := Eval(expr.args[1], env)
				def := Eval(expr.args[2], env)
				switch c := coll.(type) {
				case coretypes.Gettable:
					ok, v := c.Get(key)
					if ok {
						return v
					}
				}
				return def
			case "procAssoc":
				coll := Eval(expr.args[0], env)
				key := Eval(expr.args[1], env)
				val := Eval(expr.args[2], env)
				return coretypes.EnsureObjectIsAssociative(coll, "").Assoc(key, val)
			}
			var args [3]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			args[1] = Eval(expr.args[1], env)
			args[2] = Eval(expr.args[2], env)
			return callable.Fn(args[:])
		case 4:
			var args [4]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			args[1] = Eval(expr.args[1], env)
			args[2] = Eval(expr.args[2], env)
			args[3] = Eval(expr.args[3], env)
			return callable.Fn(args[:])
		default:
			args := evalSeq(expr.args, env)
			return callable.Fn(args)
		}
	case *Fn:
		if findFnVarName(callable) == "reduce" && (len(expr.args) == 2 || len(expr.args) == 3) {
			f := Eval(expr.args[0], env)
			if fn, ok := f.(coretypes.Callable); ok {
				if len(expr.args) == 3 {
					init := Eval(expr.args[1], env)
					coll := Eval(expr.args[2], env)
					if r, ok := coll.(coretypes.Reduce); ok {
						return r.ReduceInit(fn, init)
					}
				} else {
					coll := Eval(expr.args[1], env)
					if r, ok := coll.(coretypes.Reduce); ok {
						return r.Reduce(fn)
					}
				}
			}
		}
		switch len(expr.args) {
		case 0:
			return irDispatchFnCall(callable, nil)
		case 1:
			var args [1]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			return irDispatchFnCall(callable, args[:])
		case 2:
			var args [2]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			args[1] = Eval(expr.args[1], env)
			return irDispatchFnCall(callable, args[:])
		case 3:
			var args [3]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			args[1] = Eval(expr.args[1], env)
			args[2] = Eval(expr.args[2], env)
			return irDispatchFnCall(callable, args[:])
		case 4:
			var args [4]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			args[1] = Eval(expr.args[1], env)
			args[2] = Eval(expr.args[2], env)
			args[3] = Eval(expr.args[3], env)
			return irDispatchFnCall(callable, args[:])
		default:
			args := evalSeq(expr.args, env)
			return irDispatchFnCall(callable, args)
		}
	case coretypes.Callable:
		switch len(expr.args) {
		case 0:
			return callable.Call(nil)
		case 1:
			var args [1]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			return callable.Call(args[:])
		case 2:
			var args [2]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			args[1] = Eval(expr.args[1], env)
			return callable.Call(args[:])
		case 3:
			var args [3]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			args[1] = Eval(expr.args[1], env)
			args[2] = Eval(expr.args[2], env)
			return callable.Call(args[:])
		case 4:
			var args [4]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			args[1] = Eval(expr.args[1], env)
			args[2] = Eval(expr.args[2], env)
			args[3] = Eval(expr.args[3], env)
			return callable.Call(args[:])
		default:
			args := evalSeq(expr.args, env)
			return callable.Call(args)
		}
	default:
		panic(RT.NewErrorWithPos(callable.ToString(false)+" is not a Fn", expr.callable.Pos()))
	}
}

func varCallableString(v *Var) string {
	if v.ns == GLOBAL_ENV.CoreNamespace {
		return "core/" + v.name.ToString(false)
	}
	return v.ns.Name.ToString(false) + "/" + v.name.ToString(false)
}

func (expr *CallExpr) Name() string {
	switch c := expr.callable.(type) {
	case *VarRefExpr:
		return varCallableString(c.vr)
	case *BindingExpr:
		return c.binding.name.ToString(false)
	case *LiteralExpr:
		return c.obj.ToString(false)
	default:
		return "fn"
	}
}

func (expr *ThrowExpr) Eval(env *LocalEnv) coretypes.Object {
	e := Eval(expr.e, env)
	switch e.(type) {
	case coretypes.Error:
		panic(e)
	default:
		panic(RT.NewError("Cannot throw " + e.ToString(false)))
	}
}

func (expr *TryExpr) Eval(env *LocalEnv) (obj coretypes.Object) {
	defer func() {
		defer func() {
			if expr.finallyExpr != nil {
				evalBody(expr.finallyExpr, env)
			}
		}()
		if r := recover(); r != nil {
			switch r := r.(type) {
			case coretypes.Error:
				for _, catchExpr := range expr.catches {
					if coretypes.IsInstance(catchExpr.excType, r) {
						obj = evalBody(catchExpr.body, env.addFrame([]coretypes.Object{r}))
						return
					}
				}
				panic(r)
			default:
				panic(r)
			}
		}
	}()
	return evalBody(expr.body, env)
}

func (expr *CatchExpr) Eval(env *LocalEnv) (obj coretypes.Object) {
	panic(RT.NewError("This should never happen!"))
}

func evalBody(body []Expr, env *LocalEnv) coretypes.Object {
	var res coretypes.Object = NIL
	for _, expr := range body {
		res = Eval(expr, env)
	}
	return res
}

func evalLoop(body []Expr, env *LocalEnv) coretypes.Object {
	var res coretypes.Object = NIL
loop:
	for _, expr := range body {
		res = Eval(expr, env)
	}
	switch res := res.(type) {
	default:
		return res
	case coretypes.RecurBindings:
		env.bindings = res
		goto loop
	}
}

func (doExpr *DoExpr) Eval(env *LocalEnv) coretypes.Object {
	return evalBody(doExpr.body, env)
}

func (expr *IfExpr) Eval(env *LocalEnv) coretypes.Object {
	if corert.ToBool(Eval(expr.cond, env)) {
		return Eval(expr.positive, env)
	}
	return Eval(expr.negative, env)
}

func (expr *FnExpr) Eval(env *LocalEnv) coretypes.Object {
	res := &Fn{fnExpr: expr}
	if expr.self.NameKey() != nil {
		selfEnv := LocalEnv{bindings: []coretypes.Object{res}, parent: env}
		if env != nil {
			selfEnv.frame = env.frame + 1
		}
		res.env = &selfEnv
		return res
	}
	res.env = env
	return res
}

func (expr *FnArityExpr) Eval(env *LocalEnv) coretypes.Object {
	panic(RT.NewError("This should never happen!"))
}

func (expr *LetExpr) Eval(env *LocalEnv) coretypes.Object {
	childEnv := LocalEnv{bindings: make([]coretypes.Object, 0, len(expr.names)), parent: env}
	if env != nil {
		childEnv.frame = env.frame + 1
	}
	for _, bindingExpr := range expr.values {
		childEnv.bindings = append(childEnv.bindings, Eval(bindingExpr, &childEnv))
	}
	return evalBody(expr.body, &childEnv)
}

func (expr *LoopExpr) Eval(env *LocalEnv) coretypes.Object {
	childEnv := LocalEnv{bindings: make([]coretypes.Object, 0, len(expr.names)), parent: env}
	if env != nil {
		childEnv.frame = env.frame + 1
	}
	for _, bindingExpr := range expr.values {
		childEnv.bindings = append(childEnv.bindings, Eval(bindingExpr, &childEnv))
	}
	// Try WASM native path first
	if prog := irGetCached(expr); prog != nil {
		if wp := wasmGetCached(prog); wp != nil {
			if result := wasmExec(wp, childEnv.bindings); result != nil {
				return result
			}
		}
		// Try IR fast path
		initSlots := childEnv.bindings
		if len(prog.captureKeys) > 0 {
			full := make([]coretypes.Object, prog.numSlots)
			copy(full, initSlots)
			for i, key := range prog.captureKeys {
				slot := len(initSlots) + i
				e := env
				for e != nil && e.frame > key.frame {
					e = e.parent
				}
				if e != nil && key.index < len(e.bindings) {
					full[slot] = e.bindings[key.index]
				}
			}
			initSlots = full
		}
		if corert.IRTypedEnabled() && runtimeExec.CanExecuteTypedIR(prog) {
			var typedResult coretypes.Object
			func() {
				epoch := currentGRT().CallableEpoch
				defer func() {
					if r := recover(); r != nil {
						if currentGRT().CallableEpoch != epoch {
							panic(r)
						}
						rethrowIRLanguageFailure(r)
						typedResult = nil
					}
				}()
				typedResult = irExecTypedNB(prog, initSlots)
				if typedResult == nil {
					typedResult = irExecTyped(prog, initSlots)
				}
			}()
			if typedResult != nil {
				return typedResult
			}
			runtimeExec.MarkTypedExecutionFailed(prog)
		}
		if wp := wasmGetCachedWithOneHelper(prog, initSlots); wp != nil {
			if result := wasmExec(wp, initSlots); result != nil {
				return result
			}
		}
		if result := wasmMemNthCompileAndExec(prog, initSlots); result != nil {
			return result
		}
		var result coretypes.Object
		func() {
			epoch := currentGRT().CallableEpoch
			defer func() {
				if r := recover(); r != nil {
					if currentGRT().CallableEpoch != epoch {
						panic(r)
					}
					rethrowIRLanguageFailure(r)
					result = nil
				}
			}()
			result = irExec(prog, initSlots)
		}()
		if result != nil {
			return result
		}
		irCache.Store(expr, irCompileFailed)
	}
	return evalLoop(expr.body, &childEnv)
}

func (expr *RecurExpr) Eval(env *LocalEnv) coretypes.Object {
	switch len(expr.args) {
	case 0:
		return coretypes.RecurBindings(nil)
	case 1:
		var args [1]coretypes.Object
		args[0] = Eval(expr.args[0], env)
		return coretypes.RecurBindings(args[:])
	case 2:
		var args [2]coretypes.Object
		args[0] = Eval(expr.args[0], env)
		args[1] = Eval(expr.args[1], env)
		return coretypes.RecurBindings(args[:])
	case 3:
		var args [3]coretypes.Object
		args[0] = Eval(expr.args[0], env)
		args[1] = Eval(expr.args[1], env)
		args[2] = Eval(expr.args[2], env)
		return coretypes.RecurBindings(args[:])
	case 4:
		var args [4]coretypes.Object
		args[0] = Eval(expr.args[0], env)
		args[1] = Eval(expr.args[1], env)
		args[2] = Eval(expr.args[2], env)
		args[3] = Eval(expr.args[3], env)
		return coretypes.RecurBindings(args[:])
	default:
		return coretypes.RecurBindings(evalSeq(expr.args, env))
	}
}

func (expr *MacroCallExpr) Eval(env *LocalEnv) coretypes.Object {
	return expr.macro.Call(expr.args)
}

func (expr *MacroCallExpr) Name() string {
	return expr.name
}

func TryEval(expr Expr) (obj coretypes.Object, err error) {
	defer func() {
		if r := recover(); r != nil {
			switch r.(type) {
			case *corert.EvalError:
				err = r.(error)
			case *ExInfo:
				err = r.(error)
			default:
				panic(r)
			}
		}
	}()
	return Eval(expr, nil), nil
}

// ---- expr.go ----

func (expr *LiteralExpr) InferType() *coretypes.Type {
	if expr.isSurrogate {
		return nil
	}
	return expr.obj.GetType()
}

func dumpPosition(p coretypes.Position) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	res.Add(KEYWORDS.startLine, coretypes.Int{I: p.StartLine})
	res.Add(KEYWORDS.endLine, coretypes.Int{I: p.EndLine})
	res.Add(KEYWORDS.startColumn, coretypes.Int{I: p.StartColumn})
	res.Add(KEYWORDS.endColumn, coretypes.Int{I: p.EndColumn})
	res.Add(KEYWORDS.filename, coretypes.String{S: p.FilenameOrUnknown()})
	return res
}

func exprArrayMap(expr Expr, exprType string, pos bool) *corecollections.ArrayMap {
	res := corecollections.EmptyArrayMap()
	res.Add(KEYWORDS.type_, coretypes.MakeKeyword(STRINGS.Intern, exprType))
	if pos {
		res.Add(KEYWORDS.pos, dumpPosition(expr.Pos()))
	}
	return res
}

func addVector(res *corecollections.ArrayMap, body []Expr, name string, pos bool) {
	b := corecollections.EmptyVector()
	for _, e := range body {
		b = b.Conjoin(e.Dump(pos))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, name), b)
}

func (expr *LiteralExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "literal", pos)
	res.Add(KEYWORDS.object, expr.obj)
	return res
}

func (expr *VectorExpr) InferType() *coretypes.Type {
	return TYPE.Vec
}

func (expr *VectorExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "vector", pos)
	addVector(res, expr.v, "vector", pos)
	return res
}

func (expr *MapExpr) InferType() *coretypes.Type {
	return TYPE.ArrayMap
}

func (expr *MapExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "map", pos)
	addVector(res, expr.keys, "keys", pos)
	addVector(res, expr.values, "values", pos)
	return res
}

func (expr *SetExpr) InferType() *coretypes.Type {
	return TYPE.MapSet
}

func (expr *SetExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "set", pos)
	addVector(res, expr.elements, "set", pos)
	return res
}

func (expr *IfExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *IfExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "if", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "condition"), expr.cond.Dump(pos))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "positive"), expr.positive.Dump(pos))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "negative"), expr.negative.Dump(pos))
	return res
}

func (expr *DefExpr) InferType() *coretypes.Type {
	return TYPE.Var
}

func (expr *DefExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "def", pos)
	res.Add(KEYWORDS.var_, expr.vr)
	res.Add(KEYWORDS.name, expr.name)
	if expr.value != nil {
		res.Add(KEYWORDS.value, expr.value.Dump(pos))
	}
	if expr.meta != nil {
		res.Add(KEYWORDS.meta, expr.meta.Dump(pos))
	}
	return res
}

func (expr *CallExpr) InferType() *coretypes.Type {
	switch callableExpr := expr.callable.(type) {
	case *VarRefExpr:
		switch f := callableExpr.vr.Value.(type) {
		case *Fn:
			if arity := selectArity(f.fnExpr, len(expr.args)); arity != nil && arity.taggedType != nil {
				return arity.taggedType
			}
		}
		return callableExpr.vr.taggedType
	}
	return nil
}

func (expr *CallExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "call", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.String{S: expr.Name()})
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "callable"), expr.callable.Dump(pos))
	addVector(res, expr.args, "args", pos)
	return res
}

func (expr *MacroCallExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *MacroCallExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "macro-call", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.String{S: expr.name})
	args := corecollections.EmptyVector()
	for _, arg := range expr.args {
		args = args.Conjoin(arg)
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "args"), args)
	return res
}

func (expr *RecurExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *RecurExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "recur", pos)
	addVector(res, expr.args, "args", pos)
	return res
}

func (expr *VarRefExpr) InferType() *coretypes.Type {
	// if expr.vr.taggedType != nil {
	// 	return expr.vr.taggedType
	// }
	if expr.vr.expr == nil {
		return nil
	}
	if expr.vr.isDynamic {
		return nil
	}
	return expr.vr.expr.InferType()
}

func (expr *VarRefExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "var-ref", pos)
	res.Add(KEYWORDS.var_, expr.vr)
	return res
}

func (expr *SetMacroExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *SetMacroExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "set-macro", pos)
	res.Add(KEYWORDS.var_, expr.vr)
	return res
}

func (expr *BindingExpr) InferType() *coretypes.Type {
	return expr.binding.inferredType
}

func (expr *BindingExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "binding", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), expr.binding.name)
	return res
}

func (expr *MetaExpr) InferType() *coretypes.Type {
	return expr.expr.InferType()
}

func (expr *MetaExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "meta", pos)
	res.Add(KEYWORDS.meta, expr.meta.Dump(pos))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "expr"), expr.expr.Dump(pos))
	return res
}

func typeOfLast(exprs []Expr) *coretypes.Type {
	n := len(exprs)
	if n > 0 {
		return exprs[n-1].InferType()
	}
	return nil
}

func (expr *DoExpr) InferType() *coretypes.Type {
	return typeOfLast(expr.body)
}

func (expr *DoExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "do", pos)
	addVector(res, expr.body, "body", pos)
	return res
}

func (expr *FnExpr) InferType() *coretypes.Type {
	return TYPE.Fn
}

func (expr *FnArityExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *FnArityExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "arity", pos)
	args := corecollections.EmptyVector()
	for _, arg := range expr.args {
		args = args.Conjoin(arg)
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "args"), args)
	addVector(res, expr.body, "body", pos)
	return res
}

func (expr *FnExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "fn", pos)
	if expr.self.NameKey() != nil {
		res.Add(coretypes.MakeKeyword(STRINGS.Intern, "self"), expr.self)
	}
	if expr.variadic != nil {
		res.Add(coretypes.MakeKeyword(STRINGS.Intern, "variadic"), expr.variadic.Dump(pos))
	}
	arities := corecollections.EmptyVector()
	for _, a := range expr.arities {
		arities = arities.Conjoin(a.Dump(pos))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "arities"), arities)
	return res
}

func (expr *LetExpr) InferType() *coretypes.Type {
	return typeOfLast(expr.body)
}

func (expr *LetExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "let", pos)
	names := corecollections.EmptyVector()
	for _, name := range expr.names {
		names = names.Conjoin(name)
	}
	addVector(res, expr.values, "values", pos)
	addVector(res, expr.body, "body", pos)
	return res
}

func (expr *LoopExpr) InferType() *coretypes.Type {
	return typeOfLast(expr.body)
}

func (expr *LoopExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "loop", pos)
	names := corecollections.EmptyVector()
	for _, name := range expr.names {
		names = names.Conjoin(name)
	}
	addVector(res, expr.values, "values", pos)
	addVector(res, expr.body, "body", pos)
	return res
}

func (expr *ThrowExpr) InferType() *coretypes.Type {
	return nil
}

func (expr *ThrowExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "throw", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "expr"), expr.e.Dump(pos))
	return res
}

func (expr *CatchExpr) InferType() *coretypes.Type {
	return typeOfLast(expr.body)
}

func (expr *CatchExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "catch", pos)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "error-type"), expr.excType)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "error-symbol"), expr.excSymbol)
	addVector(res, expr.body, "body", pos)
	return res
}

func (expr *TryExpr) InferType() *coretypes.Type {
	return typeOfLast(expr.body)
}

func (expr *TryExpr) Dump(pos bool) coretypes.Map {
	res := exprArrayMap(expr, "try", pos)
	addVector(res, expr.body, "body", pos)
	addVector(res, expr.finallyExpr, "finally", pos)
	catches := corecollections.EmptyVector()
	for _, c := range expr.catches {
		catches = catches.Conjoin(c.Dump(pos))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "catches"), catches)
	return res
}

// ---- pack.go ----

const (
	LITERAL_EXPR   = 1
	VECTOR_EXPR    = 2
	MAP_EXPR       = 3
	SET_EXPR       = 4
	IF_EXPR        = 5
	DEF_EXPR       = 6
	CALL_EXPR      = 7
	RECUR_EXPR     = 8
	META_EXPR      = 9
	DO_EXPR        = 10
	FN_ARITY_EXPR  = 11
	FN_EXPR        = 12
	LET_EXPR       = 13
	THROW_EXPR     = 14
	CATCH_EXPR     = 15
	TRY_EXPR       = 16
	VARREF_EXPR    = 17
	BINDING_EXPR   = 18
	LOOP_EXPR      = 19
	SET_MACRO_EXPR = 20
	NULL           = 100
	NOT_NULL       = 101
	SYMBOL_OBJ     = 102
	VAR_OBJ        = 103
	TYPE_OBJ       = 104
)

type (
	PackEnv struct {
		Strings          map[*string]uint16
		Bindings         map[*Binding]int
		nextStringIndex  uint16
		nextBindingIndex int
	}

	PackHeader struct {
		GlobalEnv *Env
		Strings   []*string
		Bindings  []Binding
	}
)

func (b *Binding) Pack(p []byte, env *PackEnv) []byte {
	p = packSymbol(b.name, p, env)
	p = appendInt(p, b.index)
	p = appendInt(p, b.frame)
	p = appendBool(p, b.isUsed)
	return p
}

func unpackBinding(p []byte, header *PackHeader) (Binding, []byte) {
	name, p := unpackSymbol(p, header)
	index, p := extractInt(p)
	frame, p := extractInt(p)
	isUsed, p := extractBool(p)
	return Binding{
		name:   name,
		index:  index,
		frame:  frame,
		isUsed: isUsed,
	}, p
}

func NewPackEnv() *PackEnv {
	return &PackEnv{
		Strings:  make(map[*string]uint16),
		Bindings: make(map[*Binding]int),
	}
}

type BindingPair struct {
	binding *Binding
	index   int
}
type ByIndex []BindingPair

func (a ByIndex) Len() int      { return len(a) }
func (a ByIndex) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByIndex) Less(i, j int) bool {
	return a[i].index < a[j].index
}

type ByString []*string

func (a ByString) Len() int      { return len(a) }
func (a ByString) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByString) Less(i, j int) bool {
	if a[i] == nil {
		return true
	}
	if a[j] == nil {
		return false
	}
	return *a[i] < *a[j]
}

func (env *PackEnv) Pack(p []byte) []byte {
	var bp []byte
	bp = appendInt(bp, len(env.Bindings))
	var bindings []BindingPair
	for k, v := range env.Bindings {
		bindings = append(bindings, BindingPair{k, v})
	}
	sort.Sort(ByIndex(bindings))
	for _, pair := range bindings {
		bp = appendInt(bp, pair.index)
		bp = pair.binding.Pack(bp, env)
	}

	p = appendInt(p, len(env.Strings))
	stringKeys := slices.Collect(maps.Keys(env.Strings))
	sort.Sort(ByString(stringKeys))
	for _, k := range stringKeys {
		p = appendUint16(p, env.Strings[k])
		if k == nil {
			p = appendInt(p, -1)
		} else {
			p = appendInt(p, len(*k))
			p = append(p, *k...)
		}
	}
	p = append(p, bp...)
	return p
}

func UnpackHeader(p []byte, env *Env) (*PackHeader, []byte) {
	stringCount, p := extractInt(p)
	strs := make([]*string, stringCount)
	for i := 0; i < stringCount; i++ {
		var index uint16
		var length int
		index, p = extractUInt16(p)
		length, p = extractInt(p)
		if length == -1 {
			strs[index] = nil
		} else {
			strs[index] = STRINGS.Intern(string(p[:length]))
			p = p[length:]
		}
	}
	header := &PackHeader{
		GlobalEnv: env,
		Strings:   strs,
	}
	bindingCount, p := extractInt(p)
	bindings := make([]Binding, bindingCount)
	for i := 0; i < bindingCount; i++ {
		var index int
		var b Binding
		index, p = extractInt(p)
		b, p = unpackBinding(p, header)
		bindings[index] = b
	}
	header.Bindings = bindings
	return header, p
}

func (env *PackEnv) stringIndex(s *string) uint16 {
	index, ok := env.Strings[s]
	if ok {
		return index
	}
	env.Strings[s] = env.nextStringIndex
	env.nextStringIndex++
	return env.nextStringIndex - 1
}

func (env *PackEnv) bindingIndex(b *Binding) int {
	index, ok := env.Bindings[b]
	if ok {
		return index
	}
	env.Bindings[b] = env.nextBindingIndex
	env.nextBindingIndex++
	return env.nextBindingIndex - 1
}

func appendBool(p []byte, b bool) []byte {
	var bb byte
	if b {
		bb = 1
	}
	return append(p, bb)
}

func extractBool(p []byte) (bool, []byte) {
	var b bool
	if p[0] == 1 {
		b = true
	}
	return b, p[1:]
}

func appendUint16(p []byte, i uint16) []byte {
	pp := make([]byte, 2)
	binary.BigEndian.PutUint16(pp, i)
	p = append(p, pp...)
	return p
}

func extractUInt16(p []byte) (uint16, []byte) {
	return binary.BigEndian.Uint16(p[0:2]), p[2:]
}

func appendUint32(p []byte, i uint32) []byte {
	pp := make([]byte, 4)
	binary.BigEndian.PutUint32(pp, i)
	p = append(p, pp...)
	return p
}

func extractUInt32(p []byte) (uint32, []byte) {
	return binary.BigEndian.Uint32(p[0:4]), p[4:]
}

func appendInt(p []byte, i int) []byte {
	pp := make([]byte, 8)
	binary.BigEndian.PutUint64(pp, uint64(i))
	p = append(p, pp...)
	return p
}

func extractInt(p []byte) (int, []byte) {
	return int(binary.BigEndian.Uint64(p[0:8])), p[8:]
}

func packPosition(pos coretypes.Position, p []byte, env *PackEnv) []byte {
	p = appendInt(p, pos.StartLine)
	p = appendInt(p, pos.EndLine)
	p = appendInt(p, pos.StartColumn)
	p = appendInt(p, pos.EndColumn)
	p = appendUint16(p, env.stringIndex(pos.Filename))
	return p
}

func unpackPosition(p []byte, header *PackHeader) (pos coretypes.Position, pp []byte) {
	pos.StartLine, p = extractInt(p)
	pos.EndLine, p = extractInt(p)
	pos.StartColumn, p = extractInt(p)
	pos.EndColumn, p = extractInt(p)
	i, p := extractUInt16(p)
	pos.Filename = header.Strings[i]
	return pos, p
}

func packObjectInfo(info *coretypes.ObjectInfo, p []byte, env *PackEnv) []byte {
	if info == nil {
		return append(p, NULL)
	}
	p = append(p, NOT_NULL)
	return packPosition(info.Pos(), p, env)
}

func unpackObjectInfo(p []byte, header *PackHeader) (*coretypes.ObjectInfo, []byte) {
	if p[0] == NULL {
		return nil, p[1:]
	}
	p = p[1:]
	pos, p := unpackPosition(p, header)
	return &coretypes.ObjectInfo{Position: pos}, p
}

func PackObjectOrNull(obj coretypes.Object, p []byte, env *PackEnv) []byte {
	if obj == nil {
		return append(p, NULL)
	}
	p = append(p, NOT_NULL)
	return packObject(obj, p, env)
}

func UnpackObjectOrNull(p []byte, header *PackHeader) (coretypes.Object, []byte) {
	if p[0] == NULL {
		return nil, p[1:]
	}
	return unpackObject(p[1:], header)
}

func packSymbol(s coretypes.Symbol, p []byte, env *PackEnv) []byte {
	p = packObjectInfo(s.Info, p, env)
	p = PackObjectOrNull(s.Meta, p, env)
	p = appendUint16(p, env.stringIndex(s.NameKey()))
	p = appendUint16(p, env.stringIndex(s.NamespaceKey()))
	p = appendUint32(p, s.PackedHash())
	return p
}

func unpackSymbol(p []byte, header *PackHeader) (coretypes.Symbol, []byte) {
	info, p := unpackObjectInfo(p, header)
	meta, p := UnpackObjectOrNull(p, header)
	iname, p := extractUInt16(p)
	ins, p := extractUInt16(p)
	hash, p := extractUInt32(p)
	res := coretypes.MakeSymbolFromKeys(header.Strings[ins], header.Strings[iname]).WithPackedHash(hash)
	res.InfoHolder = coretypes.InfoHolder{Info: info}
	if meta != nil {
		res = res.WithMeta(meta.(coretypes.Map)).(coretypes.Symbol)
	}
	return res, p
}

func packType(t *coretypes.Type, p []byte, env *PackEnv) []byte {
	s := coretypes.MakeSymbol(STRINGS.Intern, t.Name)
	return packSymbol(s, p, env)
}

func unpackType(p []byte, header *PackHeader) (*coretypes.Type, []byte) {
	s, p := unpackSymbol(p, header)
	return TYPES.Lookup(s.NameKey()), p
}

func packObject(obj coretypes.Object, p []byte, env *PackEnv) []byte {
	switch obj := obj.(type) {
	case coretypes.Symbol:
		p = append(p, SYMBOL_OBJ)
		return packSymbol(obj, p, env)
	case *Var:
		p = append(p, VAR_OBJ)
		p = obj.Pack(p, env)
		return p
	case *coretypes.Type:
		p = append(p, TYPE_OBJ)
		p = packType(obj, p, env)
		return p
	default:
		p = append(p, NULL)
		var buf bytes.Buffer
		PrintObject(obj, &buf)
		bb := buf.Bytes()
		p = appendInt(p, len(bb))
		p = append(p, bb...)
		return p
	}
}

func unpackObject(p []byte, header *PackHeader) (coretypes.Object, []byte) {
	switch p[0] {
	case SYMBOL_OBJ:
		return unpackSymbol(p[1:], header)
	case VAR_OBJ:
		return unpackVar(p[1:], header)
	case TYPE_OBJ:
		return unpackType(p[1:], header)
	case NULL:
		var size int
		size, p = extractInt(p[1:])
		obj := readFromReader(bytes.NewReader(p[:size]))
		return obj, p[size:]
	default:
		panic(RT.NewError(fmt.Sprintf("Unknown object tag: %d", p[0])))
	}
}

func (expr *LiteralExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, LITERAL_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = appendBool(p, expr.isSurrogate)
	p = packObject(expr.obj, p, env)
	return p
}

func (expr *MacroCallExpr) Pack(p []byte, env *PackEnv) []byte {
	panic(RT.NewError("cannot pack macro call expression"))
}

func unpackLiteralExpr(p []byte, header *PackHeader) (*LiteralExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	isSurrogate, p := extractBool(p)
	obj, p := unpackObject(p, header)
	res := &LiteralExpr{
		obj:         obj,
		Position:    pos,
		isSurrogate: isSurrogate,
	}
	return res, p
}

func packSeq(p []byte, s []Expr, env *PackEnv) []byte {
	p = appendInt(p, len(s))
	for _, e := range s {
		p = e.Pack(p, env)
	}
	return p
}

func unpackSeq(p []byte, header *PackHeader) ([]Expr, []byte) {
	c, p := extractInt(p)
	res := make([]Expr, c)
	for i := 0; i < c; i++ {
		res[i], p = UnpackExpr(p, header)
	}
	return res, p
}

func packSymbolSeq(p []byte, s []coretypes.Symbol, env *PackEnv) []byte {
	p = appendInt(p, len(s))
	for _, e := range s {
		p = packSymbol(e, p, env)
	}
	return p
}

func unpackSymbolSeq(p []byte, header *PackHeader) ([]coretypes.Symbol, []byte) {
	c, p := extractInt(p)
	res := make([]coretypes.Symbol, c)
	for i := 0; i < c; i++ {
		res[i], p = unpackSymbol(p, header)
	}
	return res, p
}

func packFnArityExprSeq(p []byte, s []FnArityExpr, env *PackEnv) []byte {
	p = appendInt(p, len(s))
	for _, e := range s {
		p = e.Pack(p, env)
	}
	return p
}

func unpackFnArityExprSeq(p []byte, header *PackHeader) ([]FnArityExpr, []byte) {
	c, p := extractInt(p)
	res := make([]FnArityExpr, c)
	for i := 0; i < c; i++ {
		var e *FnArityExpr
		e, p = unpackFnArityExpr(p, header)
		res[i] = *e
	}
	return res, p
}

func packCatchExprSeq(p []byte, s []*CatchExpr, env *PackEnv) []byte {
	p = appendInt(p, len(s))
	for _, e := range s {
		p = e.Pack(p, env)
	}
	return p
}

func unpackCatchExprSeq(p []byte, header *PackHeader) ([]*CatchExpr, []byte) {
	c, p := extractInt(p)
	res := make([]*CatchExpr, c)
	for i := 0; i < c; i++ {
		res[i], p = unpackCatchExpr(p, header)
	}
	return res, p
}

func (expr *VectorExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, VECTOR_EXPR)
	p = packPosition(expr.Pos(), p, env)
	return packSeq(p, expr.v, env)
}

func unpackVectorExpr(p []byte, header *PackHeader) (*VectorExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	v, p := unpackSeq(p, header)
	return readerConstruction.VectorExpr(v, pos), p
}

func (expr *SetExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, SET_EXPR)
	p = packPosition(expr.Pos(), p, env)
	return packSeq(p, expr.elements, env)
}

func unpackSetExpr(p []byte, header *PackHeader) (*SetExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	v, p := unpackSeq(p, header)
	return readerConstruction.SetExprFrom(v, pos), p
}

func (expr *MapExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, MAP_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = packSeq(p, expr.keys, env)
	p = packSeq(p, expr.values, env)
	return p
}

func unpackMapExpr(p []byte, header *PackHeader) (*MapExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	ks, p := unpackSeq(p, header)
	vs, p := unpackSeq(p, header)
	return readerConstruction.MapExprFrom(ks, vs, pos), p
}

func (expr *IfExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, IF_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = expr.cond.Pack(p, env)
	p = expr.positive.Pack(p, env)
	p = expr.negative.Pack(p, env)
	return p
}

func unpackIfExpr(p []byte, header *PackHeader) (*IfExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	cond, p := UnpackExpr(p, header)
	positive, p := UnpackExpr(p, header)
	negative, p := UnpackExpr(p, header)
	res := &IfExpr{
		Position: pos,
		positive: positive,
		negative: negative,
		cond:     cond,
	}
	return res, p
}

func (expr *DefExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, DEF_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = packSymbol(expr.name, p, env)
	p = PackExprOrNull(expr.value, p, env)
	p = PackExprOrNull(expr.meta, p, env)
	p = packObjectInfo(expr.vr.Info, p, env)
	return p
}

func unpackDefExpr(p []byte, header *PackHeader) (*DefExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	name, p := unpackSymbol(p, header)
	varName := coretypes.MakeSymbolFromKeys(nil, name.NameKey())
	vr := header.GlobalEnv.CurrentNamespace().Intern(varName)
	value, p := UnpackExprOrNull(p, header)
	meta, p := UnpackExprOrNull(p, header)
	varInfo, p := unpackObjectInfo(p, header)
	updateVar(vr, varInfo, value, name)
	res := &DefExpr{
		Position: pos,
		vr:       vr,
		name:     name,
		value:    value,
		meta:     meta,
	}
	return res, p
}

func (expr *CallExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, CALL_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = expr.callable.Pack(p, env)
	p = packSeq(p, expr.args, env)
	return p
}

func unpackCallExpr(p []byte, header *PackHeader) (*CallExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	callable, p := UnpackExpr(p, header)
	args, p := unpackSeq(p, header)
	res := &CallExpr{
		Position: pos,
		callable: callable,
		args:     args,
	}
	return res, p
}

func (expr *RecurExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, RECUR_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = packSeq(p, expr.args, env)
	return p
}

func unpackRecurExpr(p []byte, header *PackHeader) (*RecurExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	args, p := unpackSeq(p, header)
	res := &RecurExpr{
		Position: pos,
		args:     args,
	}
	return res, p
}

func (vr *Var) Pack(p []byte, env *PackEnv) []byte {
	p = packSymbol(vr.ns.Name, p, env)
	p = packSymbol(vr.name, p, env)
	return p
}

func unpackVar(p []byte, header *PackHeader) (*Var, []byte) {
	nsName, p := unpackSymbol(p, header)
	name, p := unpackSymbol(p, header)
	vr := GLOBAL_ENV.FindNamespace(nsName).mappings[name.NameKey()]
	if vr == nil {
		panic(RT.NewError("coretypes.Error unpacking var: cannot find var " + nsName.Name() + "/" + name.Name()))
	}
	return vr, p
}

func (expr *VarRefExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, VARREF_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = expr.vr.Pack(p, env)
	return p
}

func unpackVarRefExpr(p []byte, header *PackHeader) (*VarRefExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	vr, p := unpackVar(p, header)
	res := &VarRefExpr{
		Position: pos,
		vr:       vr,
	}
	return res, p
}

func (expr *SetMacroExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, SET_MACRO_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = expr.vr.Pack(p, env)
	return p
}

func unpackSetMacroExpr(p []byte, header *PackHeader) (*SetMacroExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	vr, p := unpackVar(p, header)
	res := &SetMacroExpr{
		Position: pos,
		vr:       vr,
	}
	return res, p
}

func (expr *BindingExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, BINDING_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = appendInt(p, env.bindingIndex(expr.binding))
	return p
}

func unpackBindingExpr(p []byte, header *PackHeader) (*BindingExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	index, p := extractInt(p)
	res := &BindingExpr{
		Position: pos,
		binding:  &header.Bindings[index],
	}
	return res, p
}

func (expr *MetaExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, META_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = expr.meta.Pack(p, env)
	p = expr.expr.Pack(p, env)
	return p
}

func unpackMetaExpr(p []byte, header *PackHeader) (*MetaExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	meta, p := unpackMapExpr(p, header)
	expr, p := UnpackExpr(p, header)
	res := &MetaExpr{
		Position: pos,
		meta:     meta,
		expr:     expr,
	}
	return res, p
}

func (expr *DoExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, DO_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = packSeq(p, expr.body, env)
	return p
}

func unpackDoExpr(p []byte, header *PackHeader) (*DoExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	body, p := unpackSeq(p, header)
	res := &DoExpr{
		Position: pos,
		body:     body,
	}
	return res, p
}

func (expr *FnArityExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, FN_ARITY_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = packSymbolSeq(p, expr.args, env)
	p = packSeq(p, expr.body, env)
	if expr.taggedType != nil {
		p = append(p, NOT_NULL)
		p = appendUint16(p, env.stringIndex(STRINGS.Intern(expr.taggedType.Name)))
	} else {
		p = append(p, NULL)
	}
	return p
}

func unpackFnArityExpr(p []byte, header *PackHeader) (*FnArityExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	args, p := unpackSymbolSeq(p, header)
	body, p := unpackSeq(p, header)
	var taggedType *coretypes.Type
	if p[0] == NULL {
		p = p[1:]
	} else {
		p = p[1:]
		var i uint16
		i, p = extractUInt16(p)
		taggedType = TYPES.Lookup(header.Strings[i])
	}
	res := &FnArityExpr{
		Position:   pos,
		body:       body,
		args:       args,
		taggedType: taggedType,
	}
	return res, p
}

func (expr *FnExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, FN_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = packFnArityExprSeq(p, expr.arities, env)
	if expr.variadic == nil {
		p = append(p, NULL)
	} else {
		p = append(p, NOT_NULL)
		p = expr.variadic.Pack(p, env)
	}
	p = packSymbol(expr.self, p, env)
	return p
}

func unpackFnExpr(p []byte, header *PackHeader) (*FnExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	arities, p := unpackFnArityExprSeq(p, header)
	var variadic *FnArityExpr
	if p[0] == NULL {
		p = p[1:]
	} else {
		p = p[1:]
		variadic, p = unpackFnArityExpr(p, header)
	}
	self, p := unpackSymbol(p, header)
	res := &FnExpr{
		Position: pos,
		arities:  arities,
		variadic: variadic,
		self:     self,
	}
	return res, p
}

func (expr *LetExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, LET_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = packSymbolSeq(p, expr.names, env)
	p = packSeq(p, expr.values, env)
	p = packSeq(p, expr.body, env)
	return p
}

func unpackLetExpr(p []byte, header *PackHeader) (*LetExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	names, p := unpackSymbolSeq(p, header)
	values, p := unpackSeq(p, header)
	body, p := unpackSeq(p, header)
	res := &LetExpr{
		Position: pos,
		names:    names,
		values:   values,
		body:     body,
	}
	return res, p
}

func (expr *LoopExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, LOOP_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = packSymbolSeq(p, expr.names, env)
	p = packSeq(p, expr.values, env)
	p = packSeq(p, expr.body, env)
	return p
}

func unpackLoopExpr(p []byte, header *PackHeader) (*LoopExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	names, p := unpackSymbolSeq(p, header)
	values, p := unpackSeq(p, header)
	body, p := unpackSeq(p, header)
	res := &LoopExpr{
		Position: pos,
		names:    names,
		values:   values,
		body:     body,
	}
	return res, p
}

func (expr *ThrowExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, THROW_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = expr.e.Pack(p, env)
	return p
}

func unpackThrowExpr(p []byte, header *PackHeader) (*ThrowExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	e, p := UnpackExpr(p, header)
	res := &ThrowExpr{
		Position: pos,
		e:        e,
	}
	return res, p
}

func (expr *CatchExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, CATCH_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = appendUint16(p, env.stringIndex(STRINGS.Intern(expr.excType.Name)))
	p = packSymbol(expr.excSymbol, p, env)
	p = packSeq(p, expr.body, env)
	return p
}

func unpackCatchExpr(p []byte, header *PackHeader) (*CatchExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	i, p := extractUInt16(p)
	typeName := header.Strings[i]
	excSymbol, p := unpackSymbol(p, header)
	body, p := unpackSeq(p, header)
	res := &CatchExpr{
		Position:  pos,
		excSymbol: excSymbol,
		body:      body,
		excType:   TYPES.Lookup(typeName),
	}
	return res, p
}

func (expr *TryExpr) Pack(p []byte, env *PackEnv) []byte {
	p = append(p, TRY_EXPR)
	p = packPosition(expr.Pos(), p, env)
	p = packSeq(p, expr.body, env)
	p = packCatchExprSeq(p, expr.catches, env)
	p = packSeq(p, expr.finallyExpr, env)
	return p
}

func unpackTryExpr(p []byte, header *PackHeader) (*TryExpr, []byte) {
	p = p[1:]
	pos, p := unpackPosition(p, header)
	body, p := unpackSeq(p, header)
	catches, p := unpackCatchExprSeq(p, header)
	finallyExpr, p := unpackSeq(p, header)
	res := &TryExpr{
		Position:    pos,
		body:        body,
		catches:     catches,
		finallyExpr: finallyExpr,
	}
	return res, p
}

func PackExprOrNull(expr Expr, p []byte, env *PackEnv) []byte {
	if expr == nil {
		return append(p, NULL)
	}
	p = append(p, NOT_NULL)
	return expr.Pack(p, env)
}

func UnpackExprOrNull(p []byte, header *PackHeader) (Expr, []byte) {
	if p[0] == NULL {
		return nil, p[1:]
	}
	return UnpackExpr(p[1:], header)
}

func UnpackExpr(p []byte, header *PackHeader) (Expr, []byte) {
	switch p[0] {
	case LITERAL_EXPR:
		return unpackLiteralExpr(p, header)
	case VECTOR_EXPR:
		return unpackVectorExpr(p, header)
	case MAP_EXPR:
		return unpackMapExpr(p, header)
	case SET_EXPR:
		return unpackSetExpr(p, header)
	case IF_EXPR:
		return unpackIfExpr(p, header)
	case DEF_EXPR:
		return unpackDefExpr(p, header)
	case CALL_EXPR:
		return unpackCallExpr(p, header)
	case RECUR_EXPR:
		return unpackRecurExpr(p, header)
	case META_EXPR:
		return unpackMetaExpr(p, header)
	case DO_EXPR:
		return unpackDoExpr(p, header)
	case FN_ARITY_EXPR:
		return unpackFnArityExpr(p, header)
	case FN_EXPR:
		return unpackFnExpr(p, header)
	case LET_EXPR:
		return unpackLetExpr(p, header)
	case LOOP_EXPR:
		return unpackLoopExpr(p, header)
	case THROW_EXPR:
		return unpackThrowExpr(p, header)
	case CATCH_EXPR:
		return unpackCatchExpr(p, header)
	case TRY_EXPR:
		return unpackTryExpr(p, header)
	case VARREF_EXPR:
		return unpackVarRefExpr(p, header)
	case SET_MACRO_EXPR:
		return unpackSetMacroExpr(p, header)
	case BINDING_EXPR:
		return unpackBindingExpr(p, header)
	default:
		panic(RT.NewError(fmt.Sprintf("Unknown pack tag: %d", p[0])))
	}
}

func init() {
	installTransducerCompat()
	maybeOverrideRange()
}

// ---- parse.go ----

type (
	Expr interface {
		Eval(env *LocalEnv) coretypes.Object
		InferType() *coretypes.Type
		Pos() coretypes.Position
		Dump(includePosition bool) coretypes.Map
		Pack(p []byte, env *PackEnv) []byte
	}
	LiteralExpr struct {
		coretypes.Position
		obj         coretypes.Object
		isSurrogate bool
	}
	VectorExpr struct {
		coretypes.Position
		v []Expr
	}
	MapExpr struct {
		coretypes.Position
		keys   []Expr
		values []Expr
	}
	SetExpr struct {
		coretypes.Position
		elements []Expr
	}
	IfExpr struct {
		coretypes.Position
		cond     Expr
		positive Expr
		negative Expr
	}
	DefExpr struct {
		coretypes.Position
		vr               *Var
		name             coretypes.Symbol
		value            Expr
		meta             Expr
		isCreatedByMacro bool
	}
	CallExpr struct {
		coretypes.Position
		callable Expr
		args     []Expr
	}
	MacroCallExpr struct {
		coretypes.Position
		macro coretypes.Callable
		args  []coretypes.Object
		name  string
	}
	RecurExpr struct {
		coretypes.Position
		args []Expr
	}
	VarRefExpr struct {
		coretypes.Position
		vr *Var
	}
	BindingExpr struct {
		coretypes.Position
		binding *Binding
	}
	MetaExpr struct {
		coretypes.Position
		meta *MapExpr
		expr Expr
	}
	DoExpr struct {
		coretypes.Position
		body             []Expr
		isCreatedByMacro bool
	}
	FnArityExpr struct {
		coretypes.Position
		args       []coretypes.Symbol
		body       []Expr
		taggedType *coretypes.Type
	}
	FnExpr struct {
		coretypes.Position
		arities       []FnArityExpr
		variadic      *FnArityExpr
		self          coretypes.Symbol
		traceName     string
		tailRewritten bool
	}
	LetExpr struct {
		coretypes.Position
		names  []coretypes.Symbol
		values []Expr
		body   []Expr
	}
	LoopExpr  LetExpr
	ThrowExpr struct {
		coretypes.Position
		e Expr
	}
	CatchExpr struct {
		coretypes.Position
		excType   *coretypes.Type
		excSymbol coretypes.Symbol
		body      []Expr
	}
	TryExpr struct {
		coretypes.Position
		body        []Expr
		catches     []*CatchExpr
		finallyExpr []Expr
	}
	SetMacroExpr struct {
		coretypes.Position
		vr *Var
	}
	ParseError struct {
		obj coretypes.Object
		msg string
	}
	Binding struct {
		name         coretypes.Symbol
		index        int
		frame        int
		isUsed       bool
		inferredType *coretypes.Type
		value        Expr // the bound expression (for fn inlining)
	}
	Bindings struct {
		bindings map[*string]*Binding
		parent   *Bindings
		frame    int
	}
	LocalEnv struct {
		bindings []coretypes.Object
		parent   *LocalEnv
		frame    int
	}
	ParseContext struct {
		GlobalEnv              *Env
		localBindings          *Bindings
		loopBindings           [][]coretypes.Symbol
		linterBindings         *Bindings
		recur                  bool
		noRecurAllowed         bool
		isUnknownCallableScope bool
	}
	Warnings struct {
		ifWithoutElse           bool
		unusedFnParameters      bool
		fnWithEmptyBody         bool
		ignoredUnusedNamespaces coretypes.Set
		IgnoredFileRegexes      []*regexp.Regexp
		entryPoints             coretypes.Set
	}
	Keywords struct {
		tag                coretypes.Keyword
		skipUnused         coretypes.Keyword
		private            coretypes.Keyword
		line               coretypes.Keyword
		column             coretypes.Keyword
		file               coretypes.Keyword
		ns                 coretypes.Keyword
		macro              coretypes.Keyword
		message            coretypes.Keyword
		form               coretypes.Keyword
		data               coretypes.Keyword
		cause              coretypes.Keyword
		arglist            coretypes.Keyword
		doc                coretypes.Keyword
		added              coretypes.Keyword
		meta               coretypes.Keyword
		knownMacros        coretypes.Keyword
		rules              coretypes.Keyword
		ifWithoutElse      coretypes.Keyword
		unusedFnParameters coretypes.Keyword
		fnWithEmptyBody    coretypes.Keyword
		_prefix            coretypes.Keyword
		pos                coretypes.Keyword
		startLine          coretypes.Keyword
		endLine            coretypes.Keyword
		startColumn        coretypes.Keyword
		endColumn          coretypes.Keyword
		filename           coretypes.Keyword
		object             coretypes.Keyword
		type_              coretypes.Keyword
		var_               coretypes.Keyword
		value              coretypes.Keyword
		vector             coretypes.Keyword
		name               coretypes.Keyword
		dynamic            coretypes.Keyword
		require            coretypes.Keyword
		_import            coretypes.Keyword
		else_              coretypes.Keyword
		none               coretypes.Keyword
		validIdent         coretypes.Keyword
		characterSet       coretypes.Keyword
		encodingRange      coretypes.Keyword
		core               coretypes.Keyword
		symbol             coretypes.Keyword
		visible            coretypes.Keyword
		ascii              coretypes.Keyword
		unicode            coretypes.Keyword
		any                coretypes.Keyword
	}
	Symbols struct {
		joker_core         coretypes.Symbol
		underscore         coretypes.Symbol
		catch              coretypes.Symbol
		finally            coretypes.Symbol
		amp                coretypes.Symbol
		_if                coretypes.Symbol
		quote              coretypes.Symbol
		fn_                coretypes.Symbol
		fn                 coretypes.Symbol
		let_               coretypes.Symbol
		let                coretypes.Symbol
		letfn_             coretypes.Symbol
		letfn              coretypes.Symbol
		loop_              coretypes.Symbol
		loop               coretypes.Symbol
		recur              coretypes.Symbol
		setMacro_          coretypes.Symbol
		def                coretypes.Symbol
		defLinter          coretypes.Symbol
		_var               coretypes.Symbol
		do                 coretypes.Symbol
		throw              coretypes.Symbol
		try                coretypes.Symbol
		unquoteSplicing    coretypes.Symbol
		list               coretypes.Symbol
		concat             coretypes.Symbol
		seq                coretypes.Symbol
		apply              coretypes.Symbol
		emptySymbol        coretypes.Symbol
		unquote            coretypes.Symbol
		vector             coretypes.Symbol
		hashMap            coretypes.Symbol
		hashSet            coretypes.Symbol
		defaultDataReaders coretypes.Symbol
		backslash          coretypes.Symbol
		deref              coretypes.Symbol
		ns                 coretypes.Symbol
		defrecord          coretypes.Symbol
		defprotocol        coretypes.Symbol
		extendProtocol     coretypes.Symbol
		extendType         coretypes.Symbol
		deftype            coretypes.Symbol
		proxy              coretypes.Symbol
		reify              coretypes.Symbol
	}
	Str struct {
		_if          *string
		quote        *string
		fn_          *string
		let_         *string
		letfn_       *string
		loop_        *string
		recur        *string
		setMacro_    *string
		def          *string
		defLinter    *string
		_var         *string
		do           *string
		throw        *string
		try          *string
		coreFilename *string
	}
)

// coretypes.Stack-allocated helper calls for hot coretypes.Callable paths.
// Avoids repeated []coretypes.Object literal allocation at call sites such as reduce,
// transducers, watches, and comparators.
func call0(c coretypes.Callable) coretypes.Object {
	return c.Call(nil)
}

func call1(c coretypes.Callable, a coretypes.Object) coretypes.Object {
	var args [1]coretypes.Object
	args[0] = a
	return c.Call(args[:])
}

func call2(c coretypes.Callable, a, b coretypes.Object) coretypes.Object {
	var args [2]coretypes.Object
	args[0] = a
	args[1] = b
	return c.Call(args[:])
}

func call3(c coretypes.Callable, a, b, d coretypes.Object) coretypes.Object {
	var args [3]coretypes.Object
	args[0] = a
	args[1] = b
	args[2] = d
	return c.Call(args[:])
}

func call4(c coretypes.Callable, a, b, d, e coretypes.Object) coretypes.Object {
	var args [4]coretypes.Object
	args[0] = a
	args[1] = b
	args[2] = d
	args[3] = e
	return c.Call(args[:])
}

var (
	LOCAL_BINDINGS *Bindings = nil
	KNOWN_MACROS   *Var
	REQUIRE_VAR    *Var
	ALIAS_VAR      *Var
	REFER_VAR      *Var
	CREATE_NS_VAR  *Var
	IN_NS_VAR      *Var
	WARNINGS       = Warnings{
		fnWithEmptyBody: true,
		entryPoints:     corecollections.EmptySet(),
	}
)

func (b *Bindings) ToMap() coretypes.Map {
	var res coretypes.Map = corecollections.EmptyArrayMap()
	for b != nil {
		for _, v := range b.bindings {
			res = res.Assoc(v.name, NIL).(coretypes.Map)
		}
		b = b.parent
	}
	return res
}

func (localEnv *LocalEnv) addEmptyFrame(capacity int) *LocalEnv {
	res := LocalEnv{
		bindings: make([]coretypes.Object, 0, capacity),
		parent:   localEnv,
	}
	if localEnv != nil {
		res.frame = localEnv.frame + 1
	}
	return &res
}

func (localEnv *LocalEnv) addBinding(obj coretypes.Object) {
	localEnv.bindings = append(localEnv.bindings, obj)
}

func (localEnv *LocalEnv) addFrame(values []coretypes.Object) *LocalEnv {
	res := LocalEnv{
		bindings: values,
		parent:   localEnv,
	}
	if localEnv != nil {
		res.frame = localEnv.frame + 1
	}
	return &res
}

func (localEnv *LocalEnv) replaceFrame(values []coretypes.Object) *LocalEnv {
	res := LocalEnv{
		bindings: values,
		parent:   localEnv.parent,
		frame:    localEnv.frame,
	}
	return &res
}

func (ctx *ParseContext) PushLoopBindings(bindings []coretypes.Symbol) {
	ctx.loopBindings = append(ctx.loopBindings, bindings)
}

func (ctx *ParseContext) PopLoopBindings() {
	ctx.loopBindings = ctx.loopBindings[:len(ctx.loopBindings)-1]
}

func (ctx *ParseContext) GetLoopBindings() []coretypes.Symbol {
	n := len(ctx.loopBindings)
	if n == 0 {
		return nil
	}
	return ctx.loopBindings[n-1]
}

func (b *Bindings) PushFrame() *Bindings {
	frame := 0
	if b != nil {
		frame = b.frame + 1
	}
	return &Bindings{
		bindings: make(map[*string]*Binding),
		parent:   b,
		frame:    frame,
	}
}

func (b *Bindings) PopFrame() *Bindings {
	return b.parent
}

func (b *Bindings) AddBinding(sym coretypes.Symbol, index int, skipUnused bool, inferredType *coretypes.Type) {
	nameKey := sym.NameKey()
	if LINTER_MODE && !skipUnused {
		old := b.bindings[nameKey]
		if old != nil && needsUnusedWarning(old) {
			printParseWarning(GetPosition(old.name), "Unused binding: "+old.name.ToString(false))
		}
	}
	b.bindings[nameKey] = &Binding{
		name:         sym,
		frame:        b.frame,
		index:        index,
		inferredType: inferredType,
	}
}

func (b *Bindings) GetBinding(sym coretypes.Symbol) *Binding {
	nameKey := sym.NameKey()
	for b != nil {
		if binding, ok := b.bindings[nameKey]; ok {
			return binding
		}
		b = b.parent
	}
	return nil
}

func (ctx *ParseContext) PushEmptyLocalFrame() {
	ctx.localBindings = ctx.localBindings.PushFrame()
}

func (ctx *ParseContext) PushLocalFrame(names []coretypes.Symbol) {
	ctx.PushEmptyLocalFrame()
	for i, sym := range names {
		ctx.localBindings.AddBinding(sym, i, true, nil)
	}
}

func (ctx *ParseContext) PopLocalFrame() {
	ctx.localBindings = ctx.localBindings.PopFrame()
}

func (ctx *ParseContext) GetLocalBinding(sym coretypes.Symbol) *Binding {
	if sym.NamespaceKey() != nil {
		return nil
	}
	return ctx.localBindings.GetBinding(sym)
}

func (expr *LetExpr) Name() string {
	return "let"
}

func (expr *LoopExpr) Name() string {
	return "loop"
}

func printError(pos coretypes.Position, msg string) {
	PROBLEM_COUNT++
	fmt.Fprintf(Stderr, "%s:%d:%d: %s\n", pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn, msg)
}

func printParseWarning(pos coretypes.Position, msg string) {
	printError(pos, "Parse warning: "+msg)
}

func printParseError(pos coretypes.Position, msg string) {
	printError(pos, "Parse error: "+msg)
}

func printReadWarning(reader *Reader, msg string) {
	pos := coretypes.Position{
		Filename:    reader.filename,
		StartColumn: reader.Column(),
		StartLine:   reader.Line(),
	}
	printError(pos, "Read warning: "+msg)
}

func printReadError(reader *Reader, msg string) {
	pos := coretypes.Position{
		Filename:    reader.filename,
		StartColumn: reader.Column(),
		StartLine:   reader.Line(),
	}
	printError(pos, "Read error: "+msg)
}

func isIgnoredUnusedNamespace(ns *Namespace) bool {
	if WARNINGS.ignoredUnusedNamespaces == nil {
		return false
	}
	ok, _ := WARNINGS.ignoredUnusedNamespaces.Get(ns.Name)
	return ok
}

func ResetUsage() {
	for _, ns := range GLOBAL_ENV.Namespaces {
		if ns == GLOBAL_ENV.CoreNamespace {
			continue
		}
		ns.isUsed = true
		for _, vr := range ns.mappings {
			vr.isUsed = true
		}
	}
}

func isEntryPointNs(ns *Namespace) bool {
	ok, _ := WARNINGS.entryPoints.Get(ns.Name)
	return ok
}

func WarnOnGloballyUnusedNamespaces() {
	var names []string
	positions := make(map[string]coretypes.Position)

	for _, ns := range GLOBAL_ENV.Namespaces {
		if !ns.isGloballyUsed && !isIgnoredUnusedNamespace(ns) && !isEntryPointNs(ns) {
			pos := ns.Name.GetInfo()
			if pos != nil && pos.FilenameOrUnknown() != "<joker.core>" && pos.FilenameOrUnknown() != "<user>" {
				name := ns.Name.ToString(false)
				names = append(names, name)
				positions[name] = pos.Position
			}
		}
	}

	sort.Strings(names)
	for _, name := range names {
		printParseWarning(positions[name], "globally unused namespace "+name)
	}
}

func WarnOnUnusedNamespaces() {
	var names []string
	positions := make(map[string]coretypes.Position)

	for _, ns := range GLOBAL_ENV.Namespaces {
		if ns != GLOBAL_ENV.CurrentNamespace() && !ns.isUsed && !isIgnoredUnusedNamespace(ns) {
			pos := ns.Name.GetInfo()
			if pos != nil && pos.FilenameOrUnknown() != "<joker.core>" && pos.FilenameOrUnknown() != "<user>" {
				name := ns.Name.ToString(false)
				names = append(names, name)
				positions[name] = pos.Position
			}
		}
	}

	sort.Strings(names)
	for _, name := range names {
		printParseWarning(positions[name], "unused namespace "+name)
	}
}

func isEntryPointVar(vr *Var) bool {
	if isEntryPointNs(vr.ns) {
		return true
	}
	sym := coretypes.MakeSymbolFromKeys(vr.ns.Name.NameKey(), vr.name.NameKey())
	ok, _ := WARNINGS.entryPoints.Get(sym)
	return ok
}

func WarnOnGloballyUnusedVars() {
	var names []string
	positions := make(map[string]coretypes.Position)

	for _, ns := range GLOBAL_ENV.Namespaces {
		if ns == GLOBAL_ENV.CoreNamespace {
			continue
		}
		for _, vr := range ns.mappings {
			if vr.ns == ns && !vr.isGloballyUsed && !vr.isPrivate && !isRecordConstructor(vr.name) && !isEntryPointVar(vr) {
				pos := vr.GetInfo()
				if pos != nil {
					varName := vr.Name()
					names = append(names, varName)
					positions[varName] = pos.Position
				}
			}
		}
	}

	sort.Strings(names)
	for _, name := range names {
		printParseWarning(positions[name], "globally unused var "+name)
	}
}

func WarnOnUnusedVars() {
	var names []string
	positions := make(map[string]coretypes.Position)

	for _, ns := range GLOBAL_ENV.Namespaces {
		if ns == GLOBAL_ENV.CoreNamespace {
			continue
		}
		for _, vr := range ns.mappings {
			if vr.ns == ns && !vr.isUsed && vr.isPrivate {
				pos := vr.GetInfo()
				if pos != nil {
					name := vr.name.Name()
					names = append(names, name)
					positions[name] = pos.Position
				}
			}
		}
	}

	sort.Strings(names)
	for _, name := range names {
		printParseWarning(positions[name], "unused var "+name)
	}
}

func NewLiteralExpr(obj coretypes.Object) *LiteralExpr {
	res := LiteralExpr{obj: obj}
	info := obj.GetInfo()
	if info != nil {
		res.Position = info.Position
	}
	return &res
}

func NewSurrogateExpr(obj coretypes.Object) *LiteralExpr {
	res := readerConstruction.LiteralExpr(obj)
	res.isSurrogate = true
	return res
}

func (err *ParseError) ToString(escape bool) string {
	return err.Error()
}

func (err *ParseError) Equals(other interface{}) bool {
	return err == other
}

func (err *ParseError) GetInfo() *coretypes.ObjectInfo {
	return nil
}

func (err *ParseError) GetType() *coretypes.Type {
	return TYPE.ParseError
}

func (err *ParseError) Hash() uint32 {
	return hashutil.Ptr(uintptr(unsafe.Pointer(err)))
}

func (err *ParseError) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	return err
}

func (err *ParseError) Message() coretypes.Object {
	return coretypes.MakeString(err.msg)
}

func (err ParseError) Error() string {
	line, column, filename := 0, 0, "<file>"
	if err.obj != nil {
		if info := err.obj.GetInfo(); info != nil {
			line, column, filename = info.StartLine, info.StartColumn, info.FilenameOrUnknown()
		}
	}
	return fmt.Sprintf("%s:%d:%d: Parse error: %s", filename, line, column, err.msg)
}

func parseSeq(seq coretypes.Seq, ctx *ParseContext) []Expr {
	res := make([]Expr, 0)
	for !seq.IsEmpty() {
		res = append(res, Parse(seq.First(), ctx))
		seq = seq.Rest()
	}
	return res
}

func parseVector(v coretypes.Vec, pos coretypes.Position, ctx *ParseContext) Expr {
	r := make([]Expr, v.Count())
	for i := 0; i < v.Count(); i++ {
		r[i] = Parse(v.At(i), ctx)
	}
	return readerConstruction.VectorExpr(r, pos)
}

func parseMap(m coretypes.Map, pos coretypes.Position, ctx *ParseContext) *MapExpr {
	res := readerConstruction.MapExpr(m.Count(), pos)
	for iter, i := m.Iter(), 0; iter.HasNext(); i++ {
		p := iter.Next()
		res.keys[i] = Parse(p.Key, ctx)
		res.values[i] = Parse(p.Value, ctx)
	}
	return res
}

func parseSet(s *corecollections.MapSet, pos coretypes.Position, ctx *ParseContext) Expr {
	res := readerConstruction.SetExpr(s.M.Count(), pos)
	for iter, i := corecollections.NewSeqIterator(s.Seq()), 0; iter.HasNext(); i++ {
		res.elements[i] = Parse(iter.Next(), ctx)
	}
	return res
}

func checkForm(obj coretypes.Object, min int, max int) int {
	seq := obj.(coretypes.Seq)
	c := corecollections.SeqCount(seq)
	if c < min {
		panic(&ParseError{obj: obj, msg: "Too few arguments to " + seq.First().ToString(false)})
	}
	if c > max {
		panic(&ParseError{obj: obj, msg: "Too many arguments to " + seq.First().ToString(false)})
	}
	return c
}

func GetPosition(obj coretypes.Object) coretypes.Position {
	info := obj.GetInfo()
	if info != nil {
		return info.Position
	}
	return coretypes.Position{}
}

func updateVar(vr *Var, info *coretypes.ObjectInfo, valueExpr Expr, sym coretypes.Symbol) {
	vr.WithInfo(info)
	vr.expr = valueExpr
	meta := sym.GetMeta()
	if meta != nil {
		if ok, p := meta.Get(KEYWORDS.private); ok {
			vr.isPrivate = corert.ToBool(p)
		}
		if ok, p := meta.Get(KEYWORDS.dynamic); ok {
			vr.isDynamic = corert.ToBool(p)
		}
		vr.taggedType = getTaggedType(sym)
	}
}

func isCreatedByMacro(formSeq coretypes.Seq) bool {
	if formSeq == nil || formSeq.IsEmpty() {
		return false
	}
	first := formSeq.First()
	if first == nil {
		return false
	}
	info := first.GetInfo()
	if info == nil {
		return false
	}
	return info.Pos().Filename == STR.coreFilename
}

func parseDef(obj coretypes.Object, ctx *ParseContext, isForLinter bool) *DefExpr {
	count := checkForm(obj, 2, 4)
	seq := obj.(coretypes.Seq)
	s := corecollections.Second(seq)
	var meta coretypes.Map
	switch sym := s.(type) {
	case coretypes.Symbol:
		if sym.NamespaceKey() != nil && coretypes.MakeSymbolFromKeys(nil, sym.NamespaceKey()) != ctx.GlobalEnv.CurrentNamespace().Name {
			panic(&ParseError{
				msg: "Can't create defs outside of current ns",
				obj: obj,
			})
		}
		symWithoutNs := coretypes.MakeSymbolFromKeys(nil, sym.NameKey())
		vr := ctx.GlobalEnv.CurrentNamespace().Intern(symWithoutNs)
		if isForLinter {
			vr.isGloballyUsed = true
		}
		res := &DefExpr{
			vr:               vr,
			name:             sym,
			value:            nil,
			Position:         GetPosition(obj),
			isCreatedByMacro: isCreatedByMacro(seq),
		}
		meta = sym.GetMeta()
		if count == 3 {
			res.value = Parse(corecollections.Third(seq), ctx)
		} else if count == 4 {
			res.value = Parse(corecollections.Fourth(seq), ctx)
			docstring := corecollections.Third(seq)
			switch docstring.(type) {
			case coretypes.String:
				if meta != nil {
					meta = meta.Assoc(KEYWORDS.doc, docstring).(coretypes.Map)
				} else {
					meta = corecollections.EmptyArrayMap().Assoc(KEYWORDS.doc, docstring).(coretypes.Map)
				}
			default:
				panic(&ParseError{obj: docstring, msg: "Docstring must be a string"})
			}
		}
		updateVar(vr, obj.GetInfo(), res.value, sym)
		if meta != nil {
			res.meta = Parse(DeriveReadObject(obj, meta), ctx)
		}
		return res
	default:
		panic(&ParseError{obj: s, msg: "First argument to def must be a coretypes.Symbol"})
	}
}

func skipRedundantDo(obj coretypes.Object) bool {
	if meta, ok := obj.(coretypes.Meta); ok {
		if m := meta.GetMeta(); m != nil {
			if ok, res := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "skip-redundant-do")); ok {
				return res.Equals(coretypes.Boolean{B: true})
			}
		}
	}
	return false
}

func parseBody(seq coretypes.Seq, ctx *ParseContext) []Expr {
	recur := ctx.recur
	ctx.recur = false
	defer func() { ctx.recur = recur }()
	res := make([]Expr, 0)
	for !seq.IsEmpty() {
		ro := seq.First()
		expr := Parse(ro, ctx)
		seq = seq.Rest()
		if ctx.recur && !seq.IsEmpty() && !LINTER_MODE {
			panic(&ParseError{obj: ro, msg: "Can only recur from tail position"})
		}
		res = append(res, expr)
		if LINTER_MODE {
			if defExpr, ok := expr.(*DefExpr); ok && !defExpr.isCreatedByMacro {
				printParseWarning(defExpr.Pos(), "inline def")
			} else if doExpr, ok := expr.(*DoExpr); ok && !doExpr.isCreatedByMacro && !skipRedundantDo(ro) {
				printParseWarning(doExpr.Pos(), "redundant do form")
			}
		}
	}
	return res
}

func parseParams(params coretypes.Object) (bindings []coretypes.Symbol, isVariadic bool) {
	res := make([]coretypes.Symbol, 0)
	v := params.(coretypes.Vec)
	for i := 0; i < v.Count(); i++ {
		ro := v.At(i)
		sym := ro
		if !coretypes.IsSymbol(sym) {
			if LINTER_MODE {
				sym = generateSymbol("linter")
			} else {
				panic(&ParseError{obj: ro, msg: "Unsupported binding form: " + sym.ToString(false)})
			}
		}
		if SYMBOLS.amp.Equals(sym) {
			if v.Count() > i+2 {
				ro := v.At(i + 2)
				panic(&ParseError{obj: ro, msg: "Unexpected parameter: " + ro.ToString(false)})
			}
			if v.Count() == i+2 {
				variadic := v.At(i + 1)
				if !coretypes.IsSymbol(variadic) {
					if LINTER_MODE {
						variadic = generateSymbol("linter")
					} else {
						panic(&ParseError{obj: variadic, msg: "Unsupported binding form: " + variadic.ToString(false)})
					}
				}
				res = append(res, variadic.(coretypes.Symbol))
				return res, true
			} else {
				return res, false
			}
		}
		res = append(res, sym.(coretypes.Symbol))
	}
	return res, false
}

func needsUnusedWarning(b *Binding) bool {
	return !b.isUsed &&
		!corestr.IsIgnorableBindingName(b.name.Name()) &&
		!isSkipUnused(b.name)
}

func addArity(fn *FnExpr, sig coretypes.Seq, ctx *ParseContext) {
	params := sig.First()
	body := sig.Rest()
	args, isVariadic := parseParams(params)
	ctx.PushLocalFrame(args)
	defer ctx.PopLocalFrame()
	ctx.PushLoopBindings(args)
	defer ctx.PopLoopBindings()

	noRecurAllowed := ctx.noRecurAllowed
	ctx.noRecurAllowed = false
	defer func() { ctx.noRecurAllowed = noRecurAllowed }()

	arity := FnArityExpr{
		Position:   GetPosition(sig),
		args:       args,
		body:       parseBody(body, ctx),
		taggedType: getTaggedType(params.(coretypes.Meta)),
	}
	if isVariadic {
		if fn.variadic != nil {
			panic(&ParseError{obj: params, msg: "Can't have more than 1 variadic overload"})
		}
		for _, arity := range fn.arities {
			if len(arity.args) >= len(args) {
				panic(&ParseError{obj: params, msg: "Can't have fixed arity function with more params than variadic function"})
			}
		}
		fn.variadic = &arity
	} else {
		for _, arity := range fn.arities {
			if len(arity.args) == len(args) {
				panic(&ParseError{obj: params, msg: "Can't have 2 overloads with same arity"})
			}
		}
		if fn.variadic != nil && len(args) >= len(fn.variadic.args) {
			panic(&ParseError{obj: params, msg: "Can't have fixed arity function with more params than variadic function"})
		}
		fn.arities = append(fn.arities, arity)
	}

	if LINTER_MODE {
		if WARNINGS.fnWithEmptyBody {
			if len(arity.body) == 0 {
				printParseWarning(arity.Position, "fn form with empty body")
			}
		}

		if WARNINGS.unusedFnParameters {
			var unused []coretypes.Symbol
			for _, b := range ctx.localBindings.bindings {
				if needsUnusedWarning(b) {
					unused = append(unused, b.name)
				}
			}
			sort.Sort(coretypes.NamedSlice[coretypes.Symbol](unused))
			for _, u := range unused {
				printParseWarning(GetPosition(u), "unused parameter: "+u.ToString(false))
			}
		}
	}
}

func wrapWithMeta(fnExpr *FnExpr, obj coretypes.Object, ctx *ParseContext) Expr {
	meta := obj.(coretypes.Meta).GetMeta()
	if meta != nil {
		return &MetaExpr{
			meta:     parseMap(meta, fnExpr.Pos(), ctx),
			expr:     fnExpr,
			Position: fnExpr.Pos(),
		}
	}
	return fnExpr
}

// Examples:
// (fn f [] 1 2)
// (fn f ([] 1 2)
//
//	([a] a 3)
//	([a & b] a b))
func parseFn(obj coretypes.Object, ctx *ParseContext) Expr {
	res := &FnExpr{Position: GetPosition(obj)}
	bodies := obj.(coretypes.Seq).Rest()
	p := bodies.First()
	if coretypes.IsSymbol(p) { // self reference
		res.self = p.(coretypes.Symbol)
		res.traceName = res.self.ToString(false)
		bodies = bodies.Rest()
		p = bodies.First()
		ctx.PushLocalFrame([]coretypes.Symbol{res.self})
		defer ctx.PopLocalFrame()
	}
	if coretypes.IsVector(p) { // single arity
		addArity(res, bodies, ctx)
		return wrapWithMeta(res, obj, ctx)
	}
	// multiple arities
	if bodies.IsEmpty() {
		panic(&ParseError{obj: p, msg: "Parameter declaration missing"})
	}
	for !bodies.IsEmpty() {
		body := bodies.First()
		switch s := body.(type) {
		case coretypes.Seq:
			params := s.First()
			if !coretypes.IsVector(params) {
				panic(&ParseError{obj: params, msg: "Parameter declaration must be a vector. Got: " + params.ToString(false)})
			}
			addArity(res, s, ctx)
		default:
			panic(&ParseError{obj: body, msg: "Function body must be a list. Got: " + s.ToString(false)})
		}
		bodies = bodies.Rest()
	}
	return wrapWithMeta(res, obj, ctx)
}

func isCatch(obj coretypes.Object) bool {
	return coretypes.IsSeq(obj) && obj.(coretypes.Seq).First().Equals(SYMBOLS.catch)
}

func isFinally(obj coretypes.Object) bool {
	return coretypes.IsSeq(obj) && obj.(coretypes.Seq).First().Equals(SYMBOLS.finally)
}

func resolveType(obj coretypes.Object, ctx *ParseContext) *coretypes.Type {
	excType := Parse(obj, ctx)
	switch excType := excType.(type) {
	case *LiteralExpr:
		switch t := excType.obj.(type) {
		case *coretypes.Type:
			return t
		}
	}
	if LINTER_MODE {
		return TYPE.Error
	}
	panic(&ParseError{obj: obj, msg: "Unable to resolve type: " + obj.ToString(false)})
}

func parseCatch(obj coretypes.Object, ctx *ParseContext) *CatchExpr {
	seq := obj.(coretypes.Seq).Rest()
	if seq.IsEmpty() || seq.Rest().IsEmpty() {
		panic(&ParseError{obj: obj, msg: "catch requires at least two arguments: type symbol and binding symbol"})
	}
	excSymbol := corecollections.Second(seq)
	excType := resolveType(seq.First(), ctx)
	if !coretypes.IsSymbol(excSymbol) {
		panic(&ParseError{obj: excSymbol, msg: "Bad binding form, expected symbol, got: " + excSymbol.ToString(false)})
	}
	ctx.PushLocalFrame([]coretypes.Symbol{excSymbol.(coretypes.Symbol)})
	defer ctx.PopLocalFrame()
	return &CatchExpr{
		Position:  GetPosition(obj),
		excType:   excType,
		excSymbol: excSymbol.(coretypes.Symbol),
		body:      parseBody(seq.Rest().Rest(), ctx),
	}
}

func parseFinally(body coretypes.Seq, ctx *ParseContext) []Expr {
	return parseBody(body, ctx)
}

func parseTry(obj coretypes.Object, ctx *ParseContext) *TryExpr {
	const (
		Regular = iota
		Catch   = iota
		Finally = iota
	)
	res := &TryExpr{Position: GetPosition(obj)}
	lastType := Regular
	seq := obj.(coretypes.Seq).Rest()

	noRecurAllowed := ctx.noRecurAllowed
	ctx.noRecurAllowed = true
	defer func() { ctx.noRecurAllowed = noRecurAllowed }()

	for !seq.IsEmpty() {
		obj = seq.First()
		if lastType == Finally {
			panic(&ParseError{obj: obj, msg: "finally clause must be last in try expression"})
		}
		if isCatch(obj) {
			res.catches = append(res.catches, parseCatch(obj, ctx))
			lastType = Catch
		} else if isFinally(obj) {
			res.finallyExpr = parseFinally(obj.(coretypes.Seq).Rest(), ctx)
			lastType = Finally
		} else {
			if lastType == Catch {
				panic(&ParseError{obj: obj, msg: "Only catch or finally clause can follow catch in try expression"})
			}
			res.body = append(res.body, Parse(obj, ctx))
		}
		seq = seq.Rest()
	}
	if LINTER_MODE {
		if res.body == nil {
			printParseWarning(res.Pos(), "try form with empty body")
		}
		if res.catches == nil && res.finallyExpr == nil {
			printParseWarning(res.Pos(), "try form without catch or finally")
		}
		if res.finallyExpr != nil && len(res.finallyExpr) == 0 {
			printParseWarning(GetPosition(obj), "finally form with empty body")
		}
	}
	return res
}

func parseLet(obj coretypes.Object, ctx *ParseContext) *LetExpr {
	return parseLetLoop(obj, "let", ctx)
}

func parseLoop(obj coretypes.Object, ctx *ParseContext) *LoopExpr {
	return (*LoopExpr)(parseLetLoop(obj, "loop", ctx))
}

func parseLetfn(obj coretypes.Object, ctx *ParseContext) *LoopExpr {
	return (*LoopExpr)(parseLetLoop(obj, "letfn", ctx))
}

func isSkipUnused(obj coretypes.Meta) bool {
	if m := obj.GetMeta(); m != nil {
		if ok, v := m.Get(KEYWORDS.skipUnused); ok {
			return corert.ToBool(v)
		}
	}
	return false
}

func parseLetLoop(obj coretypes.Object, formName string, ctx *ParseContext) *LetExpr {
	res := &LetExpr{
		Position: GetPosition(obj),
	}
	bindings := corecollections.Second(obj.(coretypes.Seq))
	switch b := bindings.(type) {
	case coretypes.Vec:
		cnt := b.Count()
		if cnt%2 != 0 {
			panic(&ParseError{obj: bindings, msg: formName + " requires an even number of forms in binding vector"})
		}
		if LINTER_MODE && formName != "loop" && cnt == 0 {
			pos := GetPosition(obj)
			printParseWarning(pos, formName+" form with empty bindings vector")
		}
		skipUnused := isSkipUnused(b)
		res.names = make([]coretypes.Symbol, cnt/2)
		res.values = make([]Expr, cnt/2)
		ctx.PushEmptyLocalFrame()
		defer ctx.PopLocalFrame()

		for i := 0; i < cnt/2; i++ {
			s := b.At(i * 2)
			switch sym := s.(type) {
			case coretypes.Symbol:
				if sym.NamespaceKey() != nil {
					msg := "Can't let qualified name: " + sym.ToString(false)
					if LINTER_MODE {
						printParseError(GetPosition(s), msg)
					} else {
						panic(&ParseError{obj: s, msg: msg})
					}
				}
				res.names[i] = sym
			default:
				if LINTER_MODE {
					res.names[i] = generateSymbol("linter")
				} else {
					panic(&ParseError{obj: s, msg: "Unsupported binding form: " + sym.ToString(false)})
				}
			}
			var inferredType *coretypes.Type
			if formName != "letfn" {
				res.values[i] = Parse(b.At(i*2+1), ctx)
				if LINTER_MODE {
					inferredType = res.values[i].InferType()
				}
			}
			ctx.localBindings.AddBinding(res.names[i], i, skipUnused, inferredType)
			// Store value on binding for IR inlining (after AddBinding creates it)
			if formName != "letfn" && res.values[i] != nil {
				if bind := ctx.localBindings.GetBinding(res.names[i]); bind != nil {
					bind.value = res.values[i]
				}
			}
		}

		if formName == "letfn" {
			for i := 0; i < cnt/2; i++ {
				res.values[i] = Parse(b.At(i*2+1), ctx)
				if bind := ctx.localBindings.GetBinding(res.names[i]); bind != nil {
					bind.value = res.values[i]
					// Rewrite tail-self-calls to recur
					if fnExpr, ok := res.values[i].(*FnExpr); ok {
						if fnExpr.traceName == "" {
							fnExpr.traceName = res.names[i].ToString(false)
						}
						rewriteTailCallsToRecur(fnExpr, bind)
					}
				}
			}
		}

		if formName == "loop" {
			ctx.PushLoopBindings(res.names)
			defer ctx.PopLoopBindings()

			noRecurAllowed := ctx.noRecurAllowed
			ctx.noRecurAllowed = false
			defer func() { ctx.noRecurAllowed = noRecurAllowed }()
		}

		res.body = parseBody(obj.(coretypes.Seq).Rest().Rest(), ctx)

		if LINTER_MODE {
			if len(res.body) == 0 {
				pos := GetPosition(obj)
				printParseWarning(pos, formName+" form with empty body")
			}

			if !skipUnused {
				var unused []coretypes.Symbol
				for _, b := range ctx.localBindings.bindings {
					if needsUnusedWarning(b) {
						unused = append(unused, b.name)
					}
				}
				sort.Sort(coretypes.NamedSlice[coretypes.Symbol](unused))
				for _, u := range unused {
					printParseWarning(GetPosition(u), "unused binding: "+u.ToString(false))
				}
			}
		}

	default:
		panic(&ParseError{obj: obj, msg: formName + " requires a vector for its bindings, got " + bindings.GetType().ToString(false)})
	}
	return res
}

func parseRecur(obj coretypes.Object, ctx *ParseContext) *RecurExpr {
	if ctx.noRecurAllowed {
		panic(&ParseError{obj: obj, msg: "Cannot recur across try"})
	}
	loopBindings := ctx.GetLoopBindings()
	if loopBindings == nil {
		panic(&ParseError{obj: obj, msg: "No recursion point for recur"})
	}
	seq := obj.(coretypes.Seq)
	args := parseSeq(seq.Rest(), ctx)
	if len(loopBindings) != len(args) {
		panic(&ParseError{obj: obj, msg: fmt.Sprintf("Mismatched argument count to recur, expected: %d args, got: %d", len(loopBindings), len(args))})
	}
	ctx.recur = true
	return &RecurExpr{
		args:     args,
		Position: GetPosition(obj),
	}
}

func resolveMacro(obj coretypes.Object, ctx *ParseContext) *Var {
	switch sym := obj.(type) {
	case coretypes.Symbol:
		if ctx.GetLocalBinding(sym) != nil {
			return nil
		}
		vr, ok := ctx.GlobalEnv.Resolve(sym)
		if !ok || !vr.isMacro || vr.Value == nil {
			return nil
		}
		vr.isUsed = true
		vr.isGloballyUsed = true
		if vr.ns == nil {
			// This very likely represents a Joker
			// bug. E.g. often seen while developing the
			// fast-init (fast-startup) version of
			// Joker. But it's much easier to debug when
			// presented as a parse error (so the
			// "offending" .joke source info is provided)
			// along with the problematic var name.
			panic(&ParseError{obj: obj, msg: fmt.Sprintf("No namespace for %s", vr.name.ToString(false))})
		}
		vr.ns.isUsed = true
		vr.ns.isGloballyUsed = true
		return vr
	default:
		return nil
	}
}

func fixInfo(obj coretypes.Object, info *coretypes.ObjectInfo) coretypes.Object {
	switch s := obj.(type) {
	case Nil:
		return obj
	case coretypes.Seq:
		objs := make([]coretypes.Object, 0, 8)
		for !s.IsEmpty() {
			t := fixInfo(s.First(), info)
			objs = append(objs, t)
			s = s.Rest()
		}
		res := corecollections.NewListFrom(objs...)
		if s, ok := obj.(coretypes.Meta); ok {
			res.Meta = s.GetMeta()
		}
		if objInfo := obj.GetInfo(); objInfo != nil {
			return res.WithInfo(objInfo)
		}
		return res.WithInfo(info)
	case coretypes.Vec:
		res := corecollections.EmptyArrayVector()
		res.Meta = s.(coretypes.Meta).GetMeta()
		for i := 0; i < s.Count(); i++ {
			t := fixInfo(s.At(i), info)
			res.Append(t)
		}
		if objInfo := obj.GetInfo(); objInfo != nil {
			return res.WithInfo(objInfo)
		}
		return res.WithInfo(info)
	case coretypes.Map:
		res := corecollections.EmptyArrayMap()
		iter := s.Iter()
		for iter.HasNext() {
			p := iter.Next()
			key := fixInfo(p.Key, info)
			value := fixInfo(p.Value, info)
			res.Add(key, value)
		}
		res.Meta = s.(coretypes.Meta).GetMeta()
		if objInfo := obj.GetInfo(); objInfo != nil {
			return res.WithInfo(objInfo)
		}
		return res.WithInfo(info)
	default:
		return obj
	}
}

func macroexpand1(seq coretypes.Seq, ctx *ParseContext) coretypes.Object {
	op := seq.First()
	vr := resolveMacro(op, ctx)
	if vr != nil {
		expr := &MacroCallExpr{
			Position: GetPosition(seq),
			macro:    vr.Value.(coretypes.Callable),
			args:     corecollections.ToSlice(seq.Rest().Cons(ctx.localBindings.ToMap()).Cons(seq)),
			name:     varCallableString(vr),
		}
		return fixInfo(Eval(expr, nil), seq.GetInfo())
	} else {
		return seq
	}
}

func reportNotAFunction(pos coretypes.Position, name string) {
	printParseWarning(pos, name+" is not a function")
}

func getTaggedType(obj coretypes.Meta) *coretypes.Type {
	if m := obj.GetMeta(); m != nil {
		if ok, typeName := m.Get(KEYWORDS.tag); ok {
			if typeSym, ok := typeName.(coretypes.Symbol); ok {
				if t := TYPES.Lookup(typeSym.NameKey()); t != nil {
					return t
				}
			}
		}
	}
	return nil
}

func getTaggedTypes(obj coretypes.Meta) []*coretypes.Type {
	var res []*coretypes.Type
	if m := obj.GetMeta(); m != nil {
		if ok, typeName := m.Get(KEYWORDS.tag); ok {
			switch typeDecl := typeName.(type) {
			case coretypes.Symbol:
				if t := TYPES.Lookup(typeDecl.NameKey()); t != nil {
					res = append(res, t)
				}
			case coretypes.String:
				parts := corestr.Split(typeDecl.S, '|')
				for _, p := range parts {
					if t := TYPES.Lookup(coretypes.MakeSymbol(STRINGS.Intern, p).NameKey()); t != nil {
						res = append(res, t)
					}
				}
			}
		}
	}
	return res
}

func isTypeOneOf(abstractTypes []*coretypes.Type, concreteType *coretypes.Type) bool {
	for _, t := range abstractTypes {
		if coretypes.IsEqualOrImplements(t, concreteType) {
			return true
		}
	}
	return false
}

func typesString(types []*coretypes.Type) string {
	var b bytes.Buffer
	for i, t := range types {
		b.WriteString(t.ToString(false))
		if i < len(types)-1 {
			b.WriteString(" or ")
		}
	}
	return b.String()
}

func checkTypes(declaredArgs []coretypes.Symbol, call *CallExpr) bool {
	res := false
	for i, da := range declaredArgs {
		if declaredTypes := getTaggedTypes(da); len(declaredTypes) > 0 {
			passedType := call.args[i].InferType()
			if passedType != nil {
				if !isTypeOneOf(declaredTypes, passedType) {
					printParseWarning(call.args[i].Pos(), fmt.Sprintf("arg[%d] of %s must have type %s, got %s", i, call.Name(), typesString(declaredTypes), passedType.ToString(false)))
					res = true
				}
			}
		}
	}
	return res
}

func selectArity(expr *FnExpr, passedArgsCount int) *FnArityExpr {
	for _, arity := range expr.arities {
		if len(arity.args) == passedArgsCount {
			return &arity
		}
	}
	if expr.variadic != nil && passedArgsCount >= len(expr.variadic.args)-1 {
		return expr.variadic
	}
	return nil
}

func reportWrongArity(expr *FnExpr, isMacro bool, call *CallExpr, pos coretypes.Position) bool {
	passedArgsCount := len(call.args)
	if isMacro {
		passedArgsCount += 2
	}
	if v := selectArity(expr, passedArgsCount); v != nil {
		return checkTypes(v.args, call)
	}
	printParseWarning(pos, fmt.Sprintf("Wrong number of args (%d) passed to %s", len(call.args), call.Name()))
	return true
}

func checkArglist(arglist coretypes.Seq, passedArgsCount int) bool {
	for !arglist.IsEmpty() {
		if v, ok := arglist.First().(coretypes.Vec); ok {
			if v.Count() == passedArgsCount ||
				v.Count() >= 2 && v.Nth(v.Count()-2).Equals(SYMBOLS.amp) && passedArgsCount >= (v.Count()-2) {
				return true
			}
		}
		arglist = arglist.Rest()
	}
	return false
}

func setMacroMeta(vr *Var) {
	if vr.Meta == nil {
		vr.Meta = corecollections.EmptyArrayMap().Assoc(KEYWORDS.macro, coretypes.Boolean{B: true}).(coretypes.Map)
	} else {
		vr.Meta = vr.Meta.Assoc(KEYWORDS.macro, coretypes.Boolean{B: true}).(coretypes.Map)
	}
}

func parseSetMacro(obj coretypes.Object, ctx *ParseContext) Expr {
	expr := Parse(corecollections.Second(obj.(coretypes.Seq)), ctx)
	switch expr := expr.(type) {
	case *LiteralExpr:
		switch vr := expr.obj.(type) {
		case *Var:
			res := &SetMacroExpr{
				vr: vr,
			}
			res.Eval(nil)
			return res
		}
	}
	panic(&ParseError{obj: obj, msg: "set-macro__ argument must be a var"})
}

func isKnownMacros(sym coretypes.Symbol) (bool, coretypes.Seq) {
	if KNOWN_MACROS == nil {
		knownMacros := GLOBAL_ENV.CoreNamespace.Resolve("*known-macros*")
		if knownMacros == nil {
			return false, nil
		}
		KNOWN_MACROS = knownMacros
	}
	if ok, v := KNOWN_MACROS.Value.(coretypes.Map).Get(sym); ok {
		switch v := v.(type) {
		case coretypes.Seqable:
			return true, v.Seq()
		default:
			return true, nil
		}
	}
	return false, nil
}

func isUnknownCallable(expr Expr) (bool, coretypes.Seq) {
	if !LINTER_MODE {
		return false, nil
	}
	if c, ok := expr.(*VarRefExpr); ok {
		if c.vr.isMacro {
			return true, nil
		}
		var sym coretypes.Symbol
		if c.vr.ns != GLOBAL_ENV.CurrentNamespace() && c.vr.ns != GLOBAL_ENV.CoreNamespace {
			sym = coretypes.MakeSymbolFromKeys(c.vr.ns.Name.NameKey(), c.vr.name.NameKey())
		} else {
			sym = coretypes.MakeSymbol(STRINGS.Intern, c.vr.name.Name())
		}
		b, s := isKnownMacros(sym)
		if b {
			return b, s
		}
		if c.vr.expr != nil {
			return false, nil
		}
		if sym.NamespaceKey() == nil && c.vr.isFake && c.vr.ns != GLOBAL_ENV.CoreNamespace {
			return true, nil
		}
	}
	return false, nil
}

func areAllLiteralExprs(exprs []Expr) bool {
	for _, expr := range exprs {
		if _, ok := expr.(*LiteralExpr); !ok {
			return false
		}
	}
	return true
}

func getRequireVar(ctx *ParseContext) *Var {
	if REQUIRE_VAR == nil {
		REQUIRE_VAR = ctx.GlobalEnv.CoreNamespace.Resolve("require")
	}
	return REQUIRE_VAR
}

func getReferVar(ctx *ParseContext) *Var {
	if REFER_VAR == nil {
		REFER_VAR = ctx.GlobalEnv.CoreNamespace.Resolve("refer")
	}
	return REFER_VAR
}

func getAliasVar(ctx *ParseContext) *Var {
	if ALIAS_VAR == nil {
		ALIAS_VAR = ctx.GlobalEnv.CoreNamespace.Resolve("alias")
	}
	return ALIAS_VAR
}

func getCreateNsVar(ctx *ParseContext) *Var {
	if CREATE_NS_VAR == nil {
		CREATE_NS_VAR = ctx.GlobalEnv.CoreNamespace.Resolve("create-ns")
	}
	return CREATE_NS_VAR
}

func getInNsVar(ctx *ParseContext) *Var {
	if IN_NS_VAR == nil {
		IN_NS_VAR = ctx.GlobalEnv.CoreNamespace.Resolve("in-ns")
	}
	return IN_NS_VAR
}

func checkCall(expr Expr, isMacro bool, call *CallExpr, pos coretypes.Position) {
	argsCount := len(call.args)
	switch expr := expr.(type) {
	case *FnExpr:
		reportWrongArity(expr, isMacro, call, pos)
	case *MapExpr:
		if argsCount == 0 || argsCount > 2 {
			printParseWarning(pos, fmt.Sprintf("Wrong number of args (%d) passed to a map", argsCount))
		}
	case *SetExpr:
		if argsCount == 0 || argsCount > 1 {
			printParseWarning(pos, fmt.Sprintf("Wrong number of args (%d) passed to a set", argsCount))
		}
	case *LiteralExpr:
		if _, ok := expr.obj.(coretypes.Callable); !ok && !expr.isSurrogate {
			reportNotAFunction(pos, call.Name())
			return
		}
		switch expr.obj.(type) {
		case coretypes.Keyword:
			if argsCount == 0 || argsCount > 2 {
				printParseWarning(pos, fmt.Sprintf("Wrong number of args (%d) passed to %s", argsCount, call.Name()))
			}
		}
	case *RecurExpr:
		reportNotAFunction(pos, call.Name())
	case *ThrowExpr:
		reportNotAFunction(pos, call.Name())
	}
}

func parseList(obj coretypes.Object, ctx *ParseContext) Expr {
	expanded := macroexpand1(obj.(coretypes.Seq), ctx)
	if expanded != obj {
		return Parse(expanded, ctx)
	}
	seq := obj.(coretypes.Seq)
	if seq.IsEmpty() {
		return readerConstruction.LiteralExpr(obj)
	}

	currentIsUnknownCallableScope := ctx.isUnknownCallableScope
	defer func() {
		ctx.isUnknownCallableScope = currentIsUnknownCallableScope
	}()

	ctx.isUnknownCallableScope = false

	pos := GetPosition(obj)
	first := seq.First()
	if v, ok := first.(coretypes.Symbol); ok && v.NamespaceKey() == nil {
		switch v.NameKey() {
		case STR.quote:
			return readerConstruction.LiteralExpr(corecollections.Second(seq))
		case STR._if:
			checkForm(obj, 3, 4)
			if LINTER_MODE && corecollections.SeqCount(seq) < 4 && WARNINGS.ifWithoutElse {
				printParseWarning(pos, "missing else branch")
			}
			return &IfExpr{
				cond:     Parse(corecollections.Second(seq), ctx),
				positive: Parse(corecollections.Third(seq), ctx),
				negative: Parse(corecollections.Fourth(seq), ctx),
				Position: pos,
			}
		case STR.fn_:
			return parseFn(obj, ctx)
		case STR.let_:
			return parseLet(obj, ctx)
		case STR.letfn_:
			return parseLetfn(obj, ctx)
		case STR.loop_:
			return parseLoop(obj, ctx)
		case STR.recur:
			return parseRecur(obj, ctx)

		// Vars' isMacro has to be properly set during parse stage
		// for linter mode to correctly handle arguments count.
		case STR.setMacro_:
			return parseSetMacro(obj, ctx)

		case STR.def:
			return parseDef(obj, ctx, false)
		case STR.defLinter:
			return parseDef(obj, ctx, true)
		case STR._var:
			checkForm(obj, 2, 2)
			switch sym := corecollections.Second(seq).(type) {
			case coretypes.Symbol:
				vr, ok := ctx.GlobalEnv.Resolve(sym)
				if !ok {
					if !LINTER_MODE {
						panic(&ParseError{obj: obj, msg: "Unable to resolve var " + sym.ToString(false) + " in this context"})
					}
					symNs := ctx.GlobalEnv.NamespaceFor(ctx.GlobalEnv.CurrentNamespace(), sym)
					if !ctx.isUnknownCallableScope {
						if symNs == nil || symNs == ctx.GlobalEnv.CurrentNamespace() {
							printParseError(GetPosition(obj), "Unable to resolve symbol: "+sym.ToString(false))
						}
					}
					vr = InternFakeSymbol(symNs, sym)
				}
				vr.isUsed = true
				vr.isGloballyUsed = true
				vr.ns.isUsed = true
				vr.ns.isGloballyUsed = true
				return &LiteralExpr{
					obj:      vr,
					Position: pos,
				}
			default:
				panic(&ParseError{obj: obj, msg: "var's argument must be a symbol"})
			}
		case STR.do:
			res := &DoExpr{
				body:             parseBody(seq.Rest(), ctx),
				Position:         pos,
				isCreatedByMacro: isCreatedByMacro(seq),
			}
			if LINTER_MODE {
				if len(res.body) == 0 {
					printParseWarning(pos, "do form with empty body")
				} else if len(res.body) == 1 {
					printParseWarning(pos, "redundant do form")
				}
			}
			return res
		case STR.throw:
			return &ThrowExpr{
				Position: pos,
				e:        Parse(corecollections.Second(seq), ctx),
			}
		case STR.try:
			return parseTry(obj, ctx)
		}
	}

	ctx.isUnknownCallableScope = currentIsUnknownCallableScope
	callable := Parse(first, ctx)
	unknown, syms := isUnknownCallable(callable)
	if unknown {
		ctx.isUnknownCallableScope = true
		if syms != nil {
			ctx.linterBindings = ctx.linterBindings.PushFrame()
			defer func() {
				ctx.linterBindings = ctx.linterBindings.PopFrame()
			}()
			for !syms.IsEmpty() {
				if sym, ok := syms.First().(coretypes.Symbol); ok {
					ctx.linterBindings.AddBinding(sym, 0, true, nil)
				}
				syms = syms.Rest()
			}
		}
	} else {
		ctx.isUnknownCallableScope = false
	}
	res := &CallExpr{
		callable: callable,
		args:     parseSeq(seq.Rest(), ctx),
		Position: pos,
	}
	if LINTER_MODE {
		switch c := res.callable.(type) {
		case *VarRefExpr:
			if c.vr.Value != nil {
				switch f := c.vr.Value.(type) {
				case *Fn:
					if !reportWrongArity(f.fnExpr, c.vr.isMacro, res, pos) {
						require := getRequireVar(ctx)
						refer := getReferVar(ctx)
						alias := getAliasVar(ctx)
						createNs := getCreateNsVar(ctx)
						inNs := getInNsVar(ctx)
						if (c.vr.Value.Equals(require.Value) ||
							c.vr.Value.Equals(alias.Value) ||
							c.vr.Value.Equals(refer.Value) ||
							c.vr.Value.Equals(inNs.Value) ||
							c.vr.Value.Equals(createNs.Value)) &&
							areAllLiteralExprs(res.args) {
							Eval(res, nil)
						}
					}
				case coretypes.Callable:
					if m := c.vr.GetMeta(); m != nil {
						if ok, arglist := m.Get(KEYWORDS.arglist); ok {
							if arglist, ok := arglist.(coretypes.Seq); ok {
								if !checkArglist(arglist, len(res.args)) {
									printParseWarning(pos, fmt.Sprintf("Wrong number of args (%d) passed to %s", len(res.args), res.Name()))
								}
							}
						}
					}
					return res
				default:
					reportNotAFunction(pos, res.Name())
				}
			} else {
				checkCall(c.vr.expr, c.vr.isMacro, res, pos)
			}
		default:
			checkCall(res.callable, false, res, pos)
		}
	}
	return res
}

func InternFakeSymbol(ns *Namespace, sym coretypes.Symbol) *Var {
	if ns != nil {
		fakeSym := coretypes.MakeSymbolFromKeys(nil, sym.NameKey())
		return ns.InternFake(fakeSym)
	}
	fakeSym := coretypes.MakeSymbolFromKeys(nil, STRINGS.Intern(sym.ToString(false)))
	return GLOBAL_ENV.CurrentNamespace().InternFake(fakeSym)
}

func isInteropSymbol(sym coretypes.Symbol) bool {
	return sym.NamespaceKey() == nil && corestr.IsInteropName(sym.Name())
}

func isRecordConstructor(sym coretypes.Symbol) bool {
	return sym.NamespaceKey() == nil && corestr.IsRecordConstructorName(sym.Name())
}

var fullClassNameRe = regexp.MustCompile(`.+\..+\.[A-Z].+`)

func isJavaSymbol(sym coretypes.Symbol) bool {
	s := sym.Name()
	if ns := sym.Namespace(); ns != "" {
		s = ns
	}
	return fullClassNameRe.MatchString(s)
}

func MakeVarRefExpr(vr *Var, obj coretypes.Object) *VarRefExpr {
	vr.isUsed = true
	vr.isGloballyUsed = true
	vr.ns.isUsed = true
	vr.ns.isGloballyUsed = true
	return &VarRefExpr{
		vr:       vr,
		Position: GetPosition(obj),
	}
}

func parseSymbol(obj coretypes.Object, ctx *ParseContext) Expr {
	sym := obj.(coretypes.Symbol)
	b := ctx.GetLocalBinding(sym)
	if b != nil {
		b.isUsed = true
		return &BindingExpr{
			binding:  b,
			Position: GetPosition(obj),
		}
	}
	if vr, ok := ctx.GlobalEnv.Resolve(sym); ok {
		return MakeVarRefExpr(vr, obj)
	}
	if sym.NamespaceKey() == nil && TYPES.Contains(sym.NameKey()) {
		return &LiteralExpr{
			Position: GetPosition(obj),
			obj:      TYPES.Lookup(sym.NameKey()),
		}
	}
	if !LINTER_MODE {
		panic(&ParseError{obj: obj, msg: "Unable to resolve symbol: " + sym.ToString(false)})
	}
	if DIALECT == corereader.CLJSDialect && sym.NamespaceKey() == nil {
		// Check if this is a "callable namespace"
		ns := ctx.GlobalEnv.FindNamespace(sym)
		if ns == nil {
			ns = ctx.GlobalEnv.CurrentNamespace().aliases[sym.NameKey()]
		}
		if ns != nil {
			ns.isUsed = true
			ns.isGloballyUsed = true
			return readerConstruction.SurrogateExpr(obj)
		}
		// See if this is a JS interop (i.e. Math.PI)
		parts := corestr.Split(sym.Name(), '.')
		if len(parts) > 1 && parts[0] != "" && parts[len(parts)-1] != "" {
			return parseSymbol(DeriveReadObject(obj, coretypes.MakeSymbol(STRINGS.Intern, corestr.JoinDotted(parts[:len(parts)-1]))), ctx)
		}
		// Check if this is a constructor call
		if len(parts) == 2 && parts[0] != "" && parts[len(parts)-1] == "" {
			if vr, ok := ctx.GlobalEnv.Resolve(coretypes.MakeSymbol(STRINGS.Intern, parts[0])); ok {
				return MakeVarRefExpr(vr, obj)
			}
		}
	}
	symNs := ctx.GlobalEnv.NamespaceFor(ctx.GlobalEnv.CurrentNamespace(), sym)
	if symNs == nil || symNs == ctx.GlobalEnv.CurrentNamespace() {
		if isInteropSymbol(sym) || isJavaSymbol(sym) {
			return readerConstruction.SurrogateExpr(sym)
		}
		if !ctx.isUnknownCallableScope {
			if ctx.linterBindings.GetBinding(sym) == nil {
				printParseError(GetPosition(obj), "Unable to resolve symbol: "+sym.ToString(false))
			}
		}
	}
	return MakeVarRefExpr(InternFakeSymbol(symNs, sym), obj)
}

func Parse(obj coretypes.Object, ctx *ParseContext) Expr {
	pos := GetPosition(obj)
	var res Expr
	canHaveMeta := false
	switch v := obj.(type) {
	case Nil:
		res = readerConstruction.LiteralExpr(obj)
	case coretypes.Vec:
		canHaveMeta = true
		res = parseVector(v, pos, ctx)
	case coretypes.Map:
		canHaveMeta = true
		res = parseMap(v, pos, ctx)
	case *corecollections.MapSet:
		canHaveMeta = true
		res = parseSet(v, pos, ctx)
	case coretypes.Seq:
		res = parseList(obj, ctx)
	case coretypes.Symbol:
		res = parseSymbol(obj, ctx)
	default:
		res = readerConstruction.LiteralExpr(obj)
	}
	if canHaveMeta {
		meta := obj.(coretypes.Meta).GetMeta()
		if meta != nil {
			return &MetaExpr{
				meta:     parseMap(meta, pos, ctx),
				expr:     res,
				Position: pos,
			}
		}
	}
	return res
}

func TryParse(obj coretypes.Object, ctx *ParseContext) (expr Expr, err error) {
	defer func() {
		if r := recover(); r != nil {
			PROBLEM_COUNT++
			switch r.(type) {
			case *ParseError:
				err = r.(error)
			case *corert.EvalError:
				err = r.(error)
			case *ExInfo:
				err = r.(error)
			default:
				panic(r)
			}
		}
	}()
	return Parse(obj, ctx), nil
}

// ---- read.go ----

type (
	ReadError struct {
		line     int
		column   int
		filename *string
		msg      string
	}
	ReadFunc func(reader *Reader) coretypes.Object
	Reader   struct {
		*corereader.RuneStream
		filename *string
		posStack corereader.PositionStack
	}
)

func NewReader(runeReader io.RuneReader, filename string) *Reader {
	return &Reader{
		RuneStream: corereader.NewRuneStream(runeReader, func(err error) {
			panic(RT.NewError(err.Error()))
		}),
		filename: STRINGS.Intern(filename),
		posStack: corereader.NewPositionStack(8),
	}
}

const EOF = corereader.EOF

var (
	LINTER_MODE   bool = false
	FORMAT_MODE   bool = false
	PROBLEM_COUNT      = 0
	DIALECT       corereader.Dialect
	LINTER_CONFIG *Var
	SUPPRESS_READ bool = false
)

var (
	ARGS   map[int]coretypes.Symbol
	GENSYM int
)

func pushPos(reader *Reader) {
	reader.posStack.Push(corereader.Position{Line: reader.Line(), Column: reader.Column()})
}

func popPos(reader *Reader) corereader.Position {
	p, ok := reader.posStack.Pop()
	if !ok {
		panic("reader position stack underflow")
	}
	return p
}

func makeReadError(reader *Reader, msg string) ReadError {
	return ReadError{
		line:     reader.Line(),
		column:   reader.Column(),
		filename: reader.filename,
		msg:      msg,
	}
}

func MakeReadError(reader *Reader, msg string) ReadError {
	return readerConstruction.ReadError(reader, msg)
}

func makeReadObject(reader *Reader, obj coretypes.Object) coretypes.Object {
	p := popPos(reader)
	return coretypes.WithInfo(obj, &coretypes.ObjectInfo{Position: coretypes.Position{
		StartColumn: p.Column,
		StartLine:   p.Line,
		EndLine:     reader.Line(),
		EndColumn:   reader.Column(),
		Filename:    reader.filename,
	}})
}

func MakeReadObject(reader *Reader, obj coretypes.Object) coretypes.Object {
	return readerConstruction.ReadObject(reader, obj)
}

func deriveReadObject(base coretypes.Object, obj coretypes.Object) coretypes.Object {
	baseInfo := base.GetInfo()
	if baseInfo != nil {
		bi := *baseInfo
		return coretypes.WithInfo(obj, &bi)
	}
	return obj
}

func DeriveReadObject(base coretypes.Object, obj coretypes.Object) coretypes.Object {
	return readerConstruction.DeriveReadObject(base, obj)
}

func (err ReadError) Message() coretypes.Object {
	return readerConstruction.String(err.msg)
}

func (err ReadError) Error() string {
	return fmt.Sprintf("%s:%d:%d: Read error: %s", corereader.FilenameOrDefault(err.filename), err.line, err.column, err.msg)
}

func eatString(reader *Reader, str string) {
	if r, ok := corereader.ConsumeExpected(reader, str); !ok {
		panic(MakeReadError(reader, fmt.Sprintf("Unexpected character %U", r)))
	}
}

func peekExpectedDelimiter(reader *Reader) {
	if !corereader.PeekDelimiter(reader) {
		panic(MakeReadError(reader, "Character not followed by delimiter"))
	}
}

func readSpecialCharacter(reader *Reader, ending string, r rune) coretypes.Object {
	eatString(reader, ending)
	peekExpectedDelimiter(reader)
	return MakeReadObject(reader, readerConstruction.Char(r))
}

func readComment(reader *Reader) coretypes.Object {
	return MakeReadObject(reader, readerConstruction.Comment(corereader.ReadCommentText(reader)))
}

func eatWhitespace(reader *Reader) {
	r := reader.Get()
	for r != EOF {
		if corereader.ShouldPreserveComma(FORMAT_MODE, r) {
			reader.Unget()
			break
		}
		if corereader.IsWhitespace(r) {
			r = reader.Get()
			continue
		}
		if r == ';' || r == '#' && corereader.ShouldSkipReaderComment(FORMAT_MODE, r, reader.Peek()) {
			r = corereader.SkipLine(reader, r)
			continue
		}
		if r == '#' && corereader.ShouldDiscardNextForm(FORMAT_MODE, r, reader.Peek()) {
			reader.Get()
			readerConstruction.Read(reader)
			r = reader.Get()
			continue
		}
		reader.Unget()
		break
	}
}

func readUnicodeCharacter(reader *Reader, length, base int) coretypes.Object {
	str := corereader.ScanUntilDelimiter(reader)
	r, ok := corereader.ParseExactUnicodeCode(str, length, base)
	if !ok {
		panic(MakeReadError(reader, "Invalid unicode character: \\o"+str))
	}
	peekExpectedDelimiter(reader)
	return MakeReadObject(reader, readerConstruction.Char(r))
}

func readCharacter(reader *Reader) coretypes.Object {
	r := reader.Get()
	if r == EOF {
		panic(MakeReadError(reader, "Incomplete character literal"))
	}
	if ending, value, ok := corereader.NamedCharacter(r, reader.Peek()); ok {
		return readSpecialCharacter(reader, ending, value)
	}
	switch corereader.ClassifyCharacterLiteral(r, reader.Peek()) {
	case corereader.CharacterLiteralUnicode:
		return readUnicodeCharacter(reader, 4, 16)
	case corereader.CharacterLiteralOctal:
		return readUnicodeCharacter(reader, 3, 8)
	}
	peekExpectedDelimiter(reader)
	return MakeReadObject(reader, readerConstruction.Char(r))
}

func invalidNumberError(reader *Reader, str string) error {
	return MakeReadError(reader, fmt.Sprintf("Invalid number: %s", str))
}

func scanBigInt(orig, str string, base int, reader *Reader) coretypes.Object {
	var bi = &big.Int{}
	if _, ok := bi.SetString(str, base); !ok {
		panic(invalidNumberError(reader, str))
	}
	return MakeReadObject(reader, readerConstruction.BigInt(bi, orig))
}

func scanRatio(str string, reader *Reader) coretypes.Object {
	var rat = &big.Rat{}
	if _, ok := rat.SetString(str); !ok {
		panic(invalidNumberError(reader, str))
	}
	return MakeReadObject(reader, readerConstruction.RatioOrInt(str, rat))
}

func scanBigFloat(orig, str string, reader *Reader) coretypes.Object {
	if f, ok := readerConstruction.BigFloatFromString(str, orig); ok {
		return MakeReadObject(reader, f)
	}
	panic(invalidNumberError(reader, str))
}

func scanInt(orig, str string, base int, reader *Reader) coretypes.Object {
	i, e := numerical.ParseInt(str, base, strconv.IntSize)
	if e != nil {
		return scanBigInt(orig, str, base, reader)
	}
	return MakeReadObject(reader, readerConstruction.Int(int(i)))
}

func scanFloat(str string, reader *Reader) coretypes.Object {
	dbl, e := numerical.ParseFloat64(str)
	if e != nil {
		panic(invalidNumberError(reader, str))
	}
	return MakeReadObject(reader, readerConstruction.Double(dbl))
}

func numberFromToken(reader *Reader, token corereader.NumberToken) coretypes.Object {
	switch token.Kind {
	case corereader.NumberTokenRatio:
		return scanRatio(token.Digits, reader)
	case corereader.NumberTokenBigInt:
		return scanBigInt(token.Original, token.Digits, token.Base, reader)
	case corereader.NumberTokenBigFloat:
		return scanBigFloat(token.Original, token.Digits, reader)
	case corereader.NumberTokenFloat:
		return scanFloat(token.Digits, reader)
	default:
		return scanInt(token.Original, token.Digits, token.Base, reader)
	}
}

func readNumber(reader *Reader) coretypes.Object {
	str := corereader.ScanUntilDelimiter(reader)
	token, err := corereader.AnalyzeNumberToken(str)
	if err != nil {
		panic(invalidNumberError(reader, str))
	}
	return readerConstruction.NumberFromToken(reader, token)
}

/* Reads (lexes) a token and returns either a coretypes.Symbol or coretypes.Keyword. */
func readIdent(reader *Reader, first rune) coretypes.Object {
	str, lastAdded, scanErr := corereader.ScanIdentToken(reader, first)
	if scanErr != nil {
		panic(MakeReadError(reader, scanErr.Error()))
	}
	if err := corereader.ValidateIdentToken(first, str, lastAdded); err != nil {
		panic(MakeReadError(reader, err.Error()))
	}
	switch {
	case corereader.IsKeywordIdent(first):
		if corereader.IsAutoResolvedKeywordIdent(first, str) {
			if FORMAT_MODE {
				return MakeReadObject(reader, readerConstruction.Keyword(str))
			}
			sym := readerConstruction.Symbol(str[1:]).(coretypes.Symbol)
			ns := GLOBAL_ENV.NamespaceFor(GLOBAL_ENV.CurrentNamespace(), sym)
			if ns == nil {
				msg := fmt.Sprintf("Unable to resolve namespace %s in keyword %s", sym.Namespace(), ":"+str)
				if LINTER_MODE {
					printReadWarning(reader, msg)
					return MakeReadObject(reader, readerConstruction.Keyword(sym.Name()))
				}
				panic(MakeReadError(reader, msg))
			}
			ns.isUsed = true
			ns.isGloballyUsed = true
			return MakeReadObject(reader, readerConstruction.Keyword(ns.Name.Name()+"/"+sym.Name()))
		}
		return MakeReadObject(reader, readerConstruction.Keyword(str))
	default:
		switch corereader.ClassifyIdentLiteral(str) {
		case corereader.IdentLiteralNil:
			return MakeReadObject(reader, readerConstruction.Nil())
		case corereader.IdentLiteralTrue:
			return MakeReadObject(reader, readerConstruction.Bool(true))
		case corereader.IdentLiteralFalse:
			return MakeReadObject(reader, readerConstruction.Bool(false))
		default:
			return MakeReadObject(reader, readerConstruction.Symbol(str))
		}
	}
}

/* When validating symbol/keyword names, which is done only in
   LINTER_MODE given the appropriate :rules in place, use function
   variables for a) simplicity of functions, b) ease of adding new
   ones (if new rules are desired), and c) hope for reasonably good
   performance. */

/*
	Returns whether a rune is a character that is inherently allowed in

/* identifiers (symbols, keywords) by dint of the fact that
/* clojure.core and other core packages define identifiers with these
/* characters. While not important for parsing (Clojure is extremely
/* permissive regarding which characters can be lexed into an
/* identifier), linting can helpfully find and warn about characters
/* outside of this set (as extended via configuration).
*/
var identValidationConfig = corereader.DefaultIdentValidationConfig()

func warnInvalidIdent(reader *Reader, s *string) {
	for _, issue := range identValidationConfig.FindIssues(s) {
		msg := fmt.Sprintf("Impermissible character %q at %d in %q (%s)", issue.Rune, issue.Index, *s, issue.Reason)
		printReadWarning(reader, msg)
	}
}

func readValidatedIdent(reader *Reader, first rune) coretypes.Object {
	obj := readIdent(reader, first)
	switch o := obj.(type) {
	case coretypes.Keyword:
		warnInvalidIdent(reader, o.NamespaceKey())
		if o.Name() != "/" {
			warnInvalidIdent(reader, o.NameKey())
		}
	case coretypes.Symbol:
		warnInvalidIdent(reader, o.NamespaceKey())
		warnInvalidIdent(reader, o.NameKey())
	}
	return obj
}

var readIdentFn = readIdent

func EnableIdentValidation() {
	readIdentFn = readValidatedIdent
}

func SetIdentSetCore() {
	identValidationConfig = identValidationConfig.WithCoreSet()
}

func SetIdentSetSymbol() {
	identValidationConfig = identValidationConfig.WithSymbolSet()
}

func SetIdentSetVisible() {
	identValidationConfig = identValidationConfig.WithVisibleSet()
}

func SetIdentSetAny() {
	identValidationConfig = identValidationConfig.WithAnySet()
}

func SetIdentRangeUnicode() {
	identValidationConfig = identValidationConfig.WithUnicodeRange()
}

func SetIdentRangeASCII() {
	identValidationConfig = identValidationConfig.WithASCIIRange()
}

func SetIdentRangeAny() {
	identValidationConfig = identValidationConfig.WithAnyRange()
}

func readRegex(reader *Reader) coretypes.Object {
	s, ok := corereader.ScanRegexLiteral(reader)
	if !ok {
		panic(MakeReadError(reader, "Non-terminated regex literal"))
	}
	regex, err := regexp.Compile(s)
	if err != nil {
		switch corereader.ClassifyInvalidRegexAction(LINTER_MODE, FORMAT_MODE) {
		case corereader.InvalidRegexPlaceholder:
			return MakeReadObject(reader, readerConstruction.Regex(nil))
		case corereader.InvalidRegexPreserveString:
			res := MakeReadObject(reader, readerConstruction.String(s))
			addPrefix(res, "#")
			return res
		default:
			panic(MakeReadError(reader, "Invalid regex: "+err.Error()))
		}
	}
	return MakeReadObject(reader, readerConstruction.Regex(regex))
}

func readUnicodeCharacterInString(reader *Reader, initial rune, length, base int, exactLength bool) rune {
	str := corereader.ScanStringEscapeCode(reader, initial, length)
	r, err := corereader.DecodeStringEscapeCode(str, length, base, exactLength)
	if err != nil {
		panic(MakeReadError(reader, err.Error()))
	}
	return r
}

func readString(reader *Reader) coretypes.Object {
	s, err := corereader.ScanStringLiteral(reader, FORMAT_MODE, func(initial rune, length, base int, exactLength bool) rune {
		return readUnicodeCharacterInString(reader, initial, length, base, exactLength)
	})
	if err != nil {
		panic(MakeReadError(reader, err.Error()))
	}
	return MakeReadObject(reader, readerConstruction.String(s))
}

func readMulti(reader *Reader, previouslyRead []coretypes.Object) (coretypes.Object, []coretypes.Object) {
	for len(previouslyRead) == 0 {
		obj, multi := readerConstruction.Read(reader)
		if !multi {
			return obj, previouslyRead
		}
		v := obj.(coretypes.Vec)
		for i := 0; i < v.Count(); i++ {
			previouslyRead = append(previouslyRead, v.At(i))
		}
		// If a splice produced no forms, keep reading.
	}
	obj, previouslyRead, _ := corereader.PopLastForm(previouslyRead)
	return obj, previouslyRead
}

func readError(reader *Reader, msg string) {
	if corereader.ShouldReportReadError(LINTER_MODE) {
		printReadError(reader, msg)
	} else {
		panic(MakeReadError(reader, msg))
	}
}

func readCondList(reader *Reader) coretypes.Object {
	previousSuppressRead := SUPPRESS_READ
	defer func() {
		SUPPRESS_READ = previousSuppressRead
	}()

	var forms []coretypes.Object
	eatWhitespace(reader)
	r := reader.Peek()
	var res coretypes.Object = nil
	for corereader.ContinueDelimitedForms(r, ')', len(forms)) {
		if res == nil {
			var feature coretypes.Object
			feature, forms = readMulti(reader, forms)
			if feature.Equals(KEYWORDS.none) || feature.Equals(KEYWORDS.else_) {
				panic(MakeReadError(reader, "Feature name "+feature.ToString(false)+" is reserved"))
			}
			if !coretypes.IsKeyword(feature) {
				panic(MakeReadError(reader, "Feature should be a keyword"))
			}
			eatWhitespace(reader)
			if corereader.NeedsConditionalPair(len(forms), reader.Peek(), ')') {
				reader.Get()
				readError(reader, "Reader conditional requires an even number of forms")
				return feature
			}
			featureEnabled, _ := GLOBAL_ENV.Features.Get(feature)
			if !corereader.ShouldSuppressUnreadConditionalBranch(res != nil, featureEnabled) {
				res, forms = readMulti(reader, forms)
			} else {
				SUPPRESS_READ = true
				_, forms = readMulti(reader, forms)
				SUPPRESS_READ = false
			}
		} else if corereader.ShouldSuppressUnreadConditionalBranch(res != nil, false) {
			SUPPRESS_READ = true
			_, forms = readMulti(reader, forms)
			SUPPRESS_READ = false
		}
		eatWhitespace(reader)
		r = reader.Peek()
	}
	reader.Get()
	return res
}

func readList(reader *Reader) coretypes.Object {
	s := make([]coretypes.Object, 0, 10)
	eatWhitespace(reader)
	r := reader.Peek()
	for r != ')' {
		obj, multi := readerConstruction.Read(reader)
		if multi {
			v := obj.(coretypes.Vec)
			for i := 0; i < v.Count(); i++ {
				s = append(s, v.At(i))
			}
		} else {
			s = append(s, obj)
		}
		eatWhitespace(reader)
		r = reader.Peek()
	}
	reader.Get()
	return MakeReadObject(reader, readerConstruction.ListFrom(s))
}

func readVector(reader *Reader) coretypes.Object {
	items := make([]coretypes.Object, 0, 10)
	eatWhitespace(reader)
	r := reader.Peek()
	for r != ']' {
		obj, multi := readerConstruction.Read(reader)
		if multi {
			v := obj.(coretypes.Vec)
			for i := 0; i < v.Count(); i++ {
				items = append(items, v.At(i))
			}
		} else {
			items = append(items, obj)
		}
		eatWhitespace(reader)
		r = reader.Peek()
	}
	reader.Get()
	return MakeReadObject(reader, readerConstruction.VectorFrom(items))
}

func resolveKey(key coretypes.Object, nsname string) coretypes.Object {
	if nsname == "" {
		return key
	}
	switch key := key.(type) {
	case coretypes.Keyword:
		if key.NamespaceKey() == nil {
			return DeriveReadObject(key, readerConstruction.Keyword(nsname+"/"+key.Name()))
		}
		if key.Namespace() == "_" {
			return DeriveReadObject(key, readerConstruction.Keyword(key.Name()))
		}
	case coretypes.Symbol:
		if key.NamespaceKey() == nil {
			return DeriveReadObject(key, readerConstruction.Symbol(nsname+"/"+key.Name()))
		}
		if key.Namespace() == "_" {
			return DeriveReadObject(key, readerConstruction.Symbol(key.Name()))
		}
	}
	return key
}

func readMap(reader *Reader) coretypes.Object {
	return readMapWithNamespace(reader, "")
}

func appendMapElement(objs []coretypes.Object, obj coretypes.Object) []coretypes.Object {
	objs = append(objs, obj)
	if corereader.ShouldAppendMapCommentSurrogate(FORMAT_MODE, corert.IsComment(obj)) {
		// Add surrogate object to always have even number of elements in the map.
		// Use rand to avoid duplicate keys.
		objs = append(objs, readerConstruction.Double(rand.Float64()))
	}
	return objs
}

func readMapWithNamespace(reader *Reader, nsname string) coretypes.Object {
	eatWhitespace(reader)
	r := reader.Peek()
	objs := []coretypes.Object{}
	for r != '}' {
		obj, multi := readerConstruction.Read(reader)
		if !multi {
			objs = appendMapElement(objs, obj)
		} else {
			v := obj.(coretypes.Vec)
			for i := 0; i < v.Count(); i++ {
				objs = appendMapElement(objs, v.At(i))
			}
		}
		eatWhitespace(reader)
		r = reader.Peek()
	}
	reader.Get()
	if !corereader.HasEvenFormCount(len(objs)) {
		panic(MakeReadError(reader, "Map literal must contain an even number of forms"))
	}
	return MakeReadObject(reader, readerConstruction.MapLiteral(reader, objs, nsname))
}

func readSet(reader *Reader) coretypes.Object {
	items := make([]coretypes.Object, 0, 8)
	eatWhitespace(reader)
	r := reader.Peek()
	for r != '}' {
		obj, multi := readerConstruction.Read(reader)
		if !multi {
			items = append(items, obj)
		} else {
			v := obj.(coretypes.Vec)
			for i := 0; i < v.Count(); i++ {
				items = append(items, v.At(i))
			}
		}
		eatWhitespace(reader)
		r = reader.Peek()
	}
	reader.Get()
	return MakeReadObject(reader, readerConstruction.SetLiteral(reader, items))
}

func makeQuote(obj coretypes.Object, quote coretypes.Symbol) coretypes.Object {
	res := readerConstruction.ListFrom([]coretypes.Object{quote, obj})
	return DeriveReadObject(obj, res)
}

func metadataFromObject(obj coretypes.Object) (*corecollections.ArrayMap, bool) {
	switch v := obj.(type) {
	case *corecollections.ArrayMap:
		return v, true
	case coretypes.String, coretypes.Symbol:
		return &corecollections.ArrayMap{Arr: []coretypes.Object{DeriveReadObject(obj, KEYWORDS.tag), obj}}, true
	case coretypes.Keyword:
		return &corecollections.ArrayMap{Arr: []coretypes.Object{obj, DeriveReadObject(obj, readerConstruction.Bool(true))}}, true
	default:
		return nil, false
	}
}

func readMeta(reader *Reader) *corecollections.ArrayMap {
	obj := readFirst(reader)
	meta, ok := readerConstruction.MetadataFromObject(obj)
	if !ok {
		panic(MakeReadError(reader, "Metadata must be coretypes.Symbol, coretypes.Keyword, String or coretypes.Map"))
	}
	return meta
}

func fillInMissingArgs(args map[int]coretypes.Symbol) {
	corereader.FillMissingArgIndexes(args, func() coretypes.Symbol { return generateSymbol("p__") })
}

func makeFnForm(args map[int]coretypes.Symbol, body coretypes.Object) coretypes.Object {
	fillInMissingArgs(args)
	a, ok := corereader.OrderedArgValues(args, SYMBOLS.amp)
	if !ok {
		panic(RT.NewError("Invalid arg literal index"))
	}
	argObjects := make([]coretypes.Object, 0, len(a))
	for _, v := range a {
		argObjects = append(argObjects, v)
	}
	argVector := readerConstruction.PersistentVectorFromSeq(readerConstruction.VectorFrom(argObjects).(coretypes.Seqable).Seq())
	if LINTER_MODE {
		if _, ok := body.(coretypes.Meta); ok {
			body, _ = readerConstruction.WithMeta(body, readerConstruction.SkipRedundantDoMeta())
		}
	}
	return DeriveReadObject(body, readerConstruction.ListFrom([]coretypes.Object{readerConstruction.Symbol("joker.core/fn"), argVector, body}))
}

func genSym(prefix string, postfix string) coretypes.Symbol {
	GENSYM++
	return readerConstruction.Symbol(fmt.Sprintf("%s%d%s", prefix, GENSYM, postfix)).(coretypes.Symbol)
}

func generateSymbol(prefix string) coretypes.Symbol {
	return genSym(prefix, "#")
}

func registerArg(index int) coretypes.Symbol {
	if s, ok := ARGS[index]; ok {
		return s
	}
	ARGS[index] = generateSymbol("p__")
	return ARGS[index]
}

func readArgSymbol(reader *Reader) coretypes.Object {
	r := reader.Peek()
	if corereader.IsBareArgLiteral(r) {
		return MakeReadObject(reader, registerArg(1))
	}
	obj := readFirst(reader)
	if obj.Equals(SYMBOLS.amp) {
		return MakeReadObject(reader, registerArg(-1))
	}
	switch n := obj.(type) {
	case coretypes.Int:
		return MakeReadObject(reader, registerArg(n.I))
	default:
		panic(MakeReadError(reader, "Arg literal must be %, %& or %integer"))
	}
}

func isSelfEvaluating(obj coretypes.Object) bool {
	if obj == corecollections.EmptyList {
		return true
	}
	switch obj.(type) {
	case coretypes.Boolean, coretypes.Double, coretypes.Int, coretypes.Char, coretypes.Keyword, coretypes.String:
		return true
	default:
		return false
	}
}

func isCall(obj coretypes.Object, name coretypes.Symbol) bool {
	switch seq := obj.(type) {
	case coretypes.Seq:
		return seq.First().Equals(name)
	default:
		return false
	}
}

func syntaxQuoteSeq(seq coretypes.Seq, env map[*string]coretypes.Symbol, reader *Reader) coretypes.Seq {
	res := make([]coretypes.Object, 0)
	for iter := corecollections.NewSeqIterator(seq); iter.HasNext(); {
		obj := iter.Next()
		if isCall(obj, SYMBOLS.unquoteSplicing) {
			res = append(res, (obj).(coretypes.Seq).Rest().First())
		} else {
			q := makeSyntaxQuote(obj, env, reader)
			res = append(res, DeriveReadObject(q, readerConstruction.ListFrom([]coretypes.Object{SYMBOLS.list, q})))
		}
	}
	return &corecollections.ArraySeq{Arr: res}
}

func syntaxQuoteColl(seq coretypes.Seq, env map[*string]coretypes.Symbol, reader *Reader, ctor coretypes.Symbol, info *coretypes.ObjectInfo) coretypes.Object {
	q := syntaxQuoteSeq(seq, env, reader)
	concat := q.Cons(SYMBOLS.concat)
	seqList := readerConstruction.ListFrom([]coretypes.Object{SYMBOLS.seq, concat})
	var res coretypes.Object = seqList
	if ctor != SYMBOLS.emptySymbol {
		res = readerConstruction.ListFrom([]coretypes.Object{ctor, seqList}).(coretypes.Seq).Cons(SYMBOLS.apply)
	}
	return coretypes.WithInfo(res, info)
}

func isSpecialSymbolName(name string) bool {
	switch name {
	case "if", "quote", "fn", "let", "letfn", "loop", "recur", "set!", "def", "def-linter", "var", "do", "throw", "try", "catch", "finally":
		return true
	default:
		return false
	}
}

func makeSyntaxQuote(obj coretypes.Object, env map[*string]coretypes.Symbol, reader *Reader) coretypes.Object {
	if isSelfEvaluating(obj) {
		return obj
	}
	if coretypes.IsSpecialSymbol(obj) {
		return makeQuote(obj, SYMBOLS.quote)
	}
	info := obj.GetInfo()
	switch s := obj.(type) {
	case coretypes.Symbol:
		if s.NamespaceKey() == nil && isSpecialSymbolName(s.Name()) {
			return makeQuote(obj, SYMBOLS.quote)
		}
		str := s.Name()
		nameKey := s.NameKey()
		if corereader.IsAutoGensymSymbolName(str, s.NamespaceKey() != nil) {
			sym, ok := env[nameKey]
			if !ok {
				sym = generateSymbol(corereader.AutoGensymPrefix(str))
				env[nameKey] = sym
			}
			obj = DeriveReadObject(obj, sym)
		} else {
			obj = DeriveReadObject(obj, GLOBAL_ENV.ResolveSymbol(s))
		}
		return makeQuote(obj, SYMBOLS.quote)
	case coretypes.Seq:
		if isCall(obj, SYMBOLS.unquote) {
			return corecollections.Second(s)
		}
		if isCall(obj, SYMBOLS.unquoteSplicing) {
			panic(MakeReadError(reader, "Splice not in list"))
		}
		return syntaxQuoteColl(s, env, reader, SYMBOLS.emptySymbol, info)
	case coretypes.Vec:
		return syntaxQuoteColl(s.Seq(), env, reader, SYMBOLS.vector, info)
	case *corecollections.ArrayMap:
		return syntaxQuoteColl(corecollections.ArraySeqFromArrayMap(s), env, reader, SYMBOLS.hashMap, info)
	case *corecollections.MapSet:
		return syntaxQuoteColl(s.Seq(), env, reader, SYMBOLS.hashSet, info)
	default:
		return obj
	}
}

func handleNoReaderError(reader *Reader, s coretypes.Symbol) coretypes.Object {
	return handleNoReaderErrorValue(reader, s, readFirst(reader))
}

func handleNoReaderErrorValue(reader *Reader, s coretypes.Symbol, value coretypes.Object) coretypes.Object {
	msg := "No reader function for tag " + s.ToString(false)
	switch corereader.ClassifyMissingTaggedReaderAction(SUPPRESS_READ, LINTER_MODE, DIALECT == corereader.EDNDialect) {
	case corereader.MissingTaggedReaderReturnValue:
		return value
	case corereader.MissingTaggedReaderWarnAndReturnValue:
		printReadWarning(reader, msg)
		return value
	default:
		panic(MakeReadError(reader, msg))
	}
}

func lookupDataReader(s coretypes.Symbol) (coretypes.Object, bool) {
	for _, name := range corereader.DataReaderVarNames() {
		vr := GLOBAL_ENV.CoreNamespace.Resolve(name)
		if vr == nil {
			continue
		}
		readersMap, ok := vr.Value.(coretypes.Map)
		if !ok {
			continue
		}
		if ok, readFunc := readersMap.Get(s); ok {
			return readFunc, true
		}
	}
	return nil, false
}

func lookupDefaultDataReaderFn() (coretypes.Callable, bool) {
	vr := GLOBAL_ENV.CoreNamespace.Resolve(corereader.DefaultDataReaderFnVarName())
	if vr == nil || vr.Value == nil || corert.IsNil(vr.Value) {
		return nil, false
	}
	return coretypes.EnsureObjectIsCallable(vr.Value, "*default-data-reader-fn* must be callable, got %s"), true
}

func readTagged(reader *Reader) coretypes.Object {
	obj := readFirst(reader)
	if FORMAT_MODE {
		next := readFirst(reader)
		addPrefix(next, corereader.TaggedLiteralPrefix(obj.ToString(false)))
		return next
	}
	switch s := obj.(type) {
	case coretypes.Symbol:
		value := readFirst(reader)
		if readFunc, ok := lookupDataReader(s); ok {
			return call1(coretypes.EnsureObjectIsCallable(readFunc, "data reader must be callable, got %s"), value)
		}
		if fallback, ok := lookupDefaultDataReaderFn(); ok {
			return call2(fallback, s, value)
		}
		return handleNoReaderErrorValue(reader, s, value)
	default:
		panic(MakeReadError(reader, "Reader tag must be a symbol"))
	}
}

func readConditional(reader *Reader) (coretypes.Object, bool) {
	isSplicing := corereader.IsConditionalSplice(reader.Peek())
	if isSplicing {
		reader.Get()
	}
	eatWhitespace(reader)
	r := reader.Get()
	if r != '(' {
		panic(MakeReadError(reader, "Reader conditional body must be a list"))
	}
	if FORMAT_MODE {
		cond := readList(reader).(*corecollections.List)
		addPrefix(cond, corereader.ConditionalPrefix(isSplicing))
		return cond, false
	}
	v := readCondList(reader)
	s, seqable := v.(coretypes.Seqable)
	switch corereader.ClassifyConditionalResult(v != nil, isSplicing, seqable) {
	case corereader.ConditionalResultEmptySplice:
		return readerConstruction.VectorFrom(nil), true
	case corereader.ConditionalResultSpliceSeq:
		return DeriveReadObject(v, readerConstruction.PersistentVectorFromSeq(s.Seq())), true
	case corereader.ConditionalResultSpliceError:
		readError(reader, "Spliced form in reader conditional must be coretypes.Seqable, got "+v.GetType().ToString(false))
		return readerConstruction.VectorFrom(nil), true
	default:
		return v, false
	}
}

func readNamespacedMap(reader *Reader) coretypes.Object {
	auto := reader.Get() == ':'
	if !auto {
		reader.Unget()
	}
	var sym coretypes.Object
	r := reader.Get()
	switch corereader.ClassifyNamespacedMapStart(r, auto) {
	case corereader.NamespacedMapStartMissingNamespace:
		reader.Unget()
		panic(MakeReadError(reader, "Namespaced map must specify a namespace"))
	case corereader.NamespacedMapStartMap:
		if corereader.IsWhitespace(r) {
			r = corereader.SkipWhitespaceRun(reader, r)
			if r != '{' {
				reader.Unget()
				panic(MakeReadError(reader, "Namespaced map must specify a namespace"))
			}
		}
	case corereader.NamespacedMapStartNamespace:
		reader.Unget()
		var multi bool
		sym, multi = readerConstruction.Read(reader)
		if multi {
			panic(MakeReadError(reader, "Namespaced map must specify a single namespace symbol"))
		}
		r = corereader.SkipWhitespaceRun(reader, reader.Get())
	}
	if r != '{' {
		panic(MakeReadError(reader, "Namespaced map must specify a map"))
	}
	if FORMAT_MODE {
		obj := readMap(reader)
		namespace := ""
		if sym != nil {
			namespace = sym.ToString(false)
		}
		addPrefix(obj, corereader.NamespacedMapPrefix(auto, namespace))
		return obj
	}
	var nsname string
	if auto {
		if sym == nil {
			nsname = GLOBAL_ENV.CurrentNamespace().Name.Name()
		} else {
			sym, ok := sym.(coretypes.Symbol)
			if !ok || sym.NamespaceKey() != nil {
				panic(MakeReadError(reader, "Namespaced map must specify a valid namespace: "+sym.ToString(false)))
			}
			nameKey := sym.NameKey()
			ns := GLOBAL_ENV.CurrentNamespace().aliases[nameKey]
			if ns == nil {
				ns = GLOBAL_ENV.Namespaces[nameKey]
			}
			if ns == nil {
				panic(MakeReadError(reader, "Unknown auto-resolved namespace alias: "+sym.ToString(false)))
			}
			ns.isUsed = true
			ns.isGloballyUsed = true
			nsname = ns.Name.Name()
		}
	} else {
		if sym == nil {
			panic(MakeReadError(reader, "Namespaced map must specify a valid namespace"))
		}
		sym, ok := sym.(coretypes.Symbol)
		if !ok || sym.NamespaceKey() != nil {
			panic(MakeReadError(reader, "Namespaced map must specify a valid namespace: "+sym.ToString(false)))
		}
		nsname = sym.Name()
	}
	return readMapWithNamespace(reader, nsname)
}

func readSymbolicValue(reader *Reader) coretypes.Object {
	obj := readFirst(reader)
	switch o := obj.(type) {
	case coretypes.Symbol:
		if v, found := corereader.SymbolicValue(o.ToString(false)); found {
			return readerConstruction.Double(v)
		}
		panic(MakeReadError(reader, "Unknown symbolic value: ##"+o.ToString(false)))
	default:
		panic(MakeReadError(reader, "Invalid token: ##"+o.ToString(false)))
	}
}

func readDispatch(reader *Reader) (coretypes.Object, bool) {
	r := reader.Get()
	kind := corereader.ClassifyDispatch(r)
	switch kind {
	case corereader.DispatchRegex:
		return readRegex(reader), false
	case corereader.DispatchVar:
		popPos(reader)
		nextObj := readFirst(reader)
		if FORMAT_MODE {
			prefix, _ := corereader.DispatchFormatPrefix(kind)
			addPrefix(nextObj, prefix)
			return nextObj, false
		}
		return DeriveReadObject(nextObj, readerConstruction.ListFrom([]coretypes.Object{DeriveReadObject(nextObj, SYMBOLS._var), nextObj})), false
	case corereader.DispatchDiscard:
		// Only possible in FORMAT mode, otherwise
		// eatWhitespaces eats #_
		popPos(reader)
		nextObj := readFirst(reader)
		prefix, _ := corereader.DispatchFormatPrefix(kind)
		addPrefix(nextObj, prefix)
		return nextObj, false
	case corereader.DispatchMeta:
		popPos(reader)
		if FORMAT_MODE {
			nextObj := readFirst(reader)
			prefix, _ := corereader.DispatchFormatPrefix(kind)
			addPrefix(nextObj, prefix)
			return nextObj, false
		}
		return readWithMeta(reader), false
	case corereader.DispatchSet:
		return readSet(reader), false
	case corereader.DispatchFn:
		popPos(reader)
		reader.Unget()
		if FORMAT_MODE {
			nextObj := readFirst(reader)
			prefix, _ := corereader.DispatchFormatPrefix(kind)
			addPrefix(nextObj, prefix)
			return nextObj, false
		}
		ARGS = make(map[int]coretypes.Symbol)
		fn := readFirst(reader)
		res := makeFnForm(ARGS, fn)
		ARGS = nil
		return res, false
	case corereader.DispatchConditional:
		return readConditional(reader)
	case corereader.DispatchNamespacedMap:
		return readNamespacedMap(reader), false
	case corereader.DispatchSymbolicValue:
		return readSymbolicValue(reader), false
	}
	popPos(reader)
	reader.Unget()
	return readTagged(reader), false
}

func readWithMeta(reader *Reader) coretypes.Object {
	meta := readMeta(reader)
	nextObj := readFirst(reader)
	obj, ok := readerConstruction.WithMeta(nextObj, meta)
	if !ok {
		panic(MakeReadError(reader, "Metadata cannot be applied to "+nextObj.ToString(false)))
	}
	return obj
}

func readFirst(reader *Reader) coretypes.Object {
	obj, multi := readerConstruction.Read(reader)
	if !multi {
		return obj
	}
	v := obj.(coretypes.Vec)
	if v.Count() == 0 {
		return readFirst(reader)
	}
	return v.At(0)
}

func addPrefix(obj coretypes.Object, prefix string) {
	obj.GetInfo().Prefix = prefix + obj.GetInfo().Prefix
}

func Read(reader *Reader) (coretypes.Object, bool) {
	eatWhitespace(reader)
	r := reader.Get()
	pushPos(reader)
	// This is only possible in format mode, otherwise
	// eatWhitespace eats comments.
	peek := rune(0)
	if r == '#' {
		peek = reader.Peek()
	}
	switch corereader.ClassifyTopLevelTrivia(r, peek) {
	case corereader.TopLevelTriviaComma:
		return MakeReadObject(reader, readerConstruction.Comment(",")), false
	case corereader.TopLevelTriviaComment:
		reader.Unget()
		return readComment(reader), false
	}

	peek = 0
	if corereader.NeedsReadFormPeek(r) {
		peek = reader.Peek()
	}
	switch corereader.ClassifyReadForm(r, peek, ARGS != nil, FORMAT_MODE, DIALECT == corereader.CLJSDialect) {
	case corereader.ReadFormCharacter:
		return readCharacter(reader), false
	case corereader.ReadFormNumber:
		reader.Unget()
		return readNumber(reader), false
	case corereader.ReadFormArgSymbol:
		return readArgSymbol(reader), false
	case corereader.ReadFormString:
		return readString(reader), false
	case corereader.ReadFormList:
		return readList(reader), false
	case corereader.ReadFormVector:
		return readVector(reader), false
	case corereader.ReadFormMap:
		return readMap(reader), false
	case corereader.ReadFormStandaloneSlash:
		return MakeReadObject(reader, SYMBOLS.backslash), false
	case corereader.ReadFormQuote:
		popPos(reader)
		nextObj := readFirst(reader)
		if FORMAT_MODE {
			prefix, _ := corereader.ReaderMacroPrefix(r)
			addPrefix(nextObj, prefix)
			return nextObj, false
		}
		return makeQuote(nextObj, SYMBOLS.quote), false
	case corereader.ReadFormDeref:
		popPos(reader)
		nextObj := readFirst(reader)
		if FORMAT_MODE {
			prefix, _ := corereader.ReaderMacroPrefix(r)
			addPrefix(nextObj, prefix)
			return nextObj, false
		}
		return DeriveReadObject(nextObj, readerConstruction.ListFrom([]coretypes.Object{DeriveReadObject(nextObj, SYMBOLS.deref), nextObj})), false
	case corereader.ReadFormUnquote:
		popPos(reader)
		isSplicing := corereader.IsUnquoteSplice(reader.Peek())
		if isSplicing {
			reader.Get()
		}
		nextObj := readFirst(reader)
		if FORMAT_MODE {
			addPrefix(nextObj, corereader.UnquotePrefix(isSplicing))
			return nextObj, false
		}
		if isSplicing {
			return makeQuote(nextObj, SYMBOLS.unquoteSplicing), false
		}
		return makeQuote(nextObj, SYMBOLS.unquote), false
	case corereader.ReadFormSyntaxQuote:
		popPos(reader)
		nextObj := readFirst(reader)
		if FORMAT_MODE {
			prefix, _ := corereader.ReaderMacroPrefix(r)
			addPrefix(nextObj, prefix)
			return nextObj, false
		}
		return makeSyntaxQuote(nextObj, make(map[*string]coretypes.Symbol), reader), false
	case corereader.ReadFormMeta:
		popPos(reader)
		if FORMAT_MODE {
			nextObj := readFirst(reader)
			prefix, _ := corereader.ReaderMacroPrefix(r)
			addPrefix(nextObj, prefix)
			return nextObj, false
		}
		return readWithMeta(reader), false
	case corereader.ReadFormDispatch:
		return readDispatch(reader)
	case corereader.ReadFormEOF:
		panic(MakeReadError(reader, "Unexpected end of file"))
	case corereader.ReadFormClosingDelimiter:
		panic(MakeReadError(reader, "Unmatched delimiter: "+string(r)))
	case corereader.ReadFormIdent:
		return readIdentFn(reader, r), false
	default:
		return readIdentFn(reader, r), false
	}
}

func TryRead(reader *Reader) (obj coretypes.Object, err error) {
	defer func() {
		if r := recover(); r != nil {
			PROBLEM_COUNT++
			switch r.(type) {
			case ReadError:
				err = r.(error)
			case *ParseError:
				err = r.(error)
			case *corert.EvalError:
				err = r.(error)
			case *ExInfo:
				err = r.(error)
			default:
				panic(r)
			}
		}
	}()
	for {
		eatWhitespace(reader)
		if reader.Peek() == EOF {
			return NIL, io.EOF
		}
		obj, multi := readerConstruction.Read(reader)
		if !multi {
			return obj, nil
		}
		// Check for obj's info to distinguish between
		// legitimate empty vector as read from the source
		// and surrogate value that means "no object was read".
		if corereader.IsTopLevelSpliceSurrogate(obj.GetInfo() != nil) {
			PROBLEM_COUNT++
			return NIL, MakeReadError(reader, "Reader conditional splicing not allowed at the top level.")
		}
	}
}

// ---- native_loop_wrapper.go ----
// buildNativeLoopWrapper builds a native f64 wrapper for a fn whose body
// is a single loop. Resolves captures from both fn params (dynamic) and
// outer scope (constant, resolved from fn.env at wrapper creation time).
func buildNativeLoopWrapper(fn *Fn, arity FnArityExpr, loop *LoopExpr, loopProg *IRProgram) nativeF64Fn {
	le := (*LetExpr)(loop)
	nLoopBinds := len(le.names)
	nSlots := loopProg.numSlots
	capKeys := loopProg.captureKeys

	// Pre-compute loop init values (must be numeric literals)
	initVals := make([]float64, nLoopBinds)
	for i, v := range le.values {
		lit, ok := v.(*LiteralExpr)
		if !ok {
			return nil
		}
		switch lv := lit.obj.(type) {
		case coretypes.Int:
			initVals[i] = float64(lv.I)
		case coretypes.Double:
			initVals[i] = lv.D
		default:
			return nil
		}
	}

	// Identify fn param frame: the frame that has indices 0..len(args)-1
	// used as captures. Multiple captures from same frame with valid param indices.
	fnParamFrame := -1
	for _, ck := range capKeys {
		if ck.index < len(arity.args) {
			if fnParamFrame < 0 {
				fnParamFrame = ck.frame
			} else if fnParamFrame != ck.frame {
				// Conflicting frames — can't determine param frame
				break
			}
		}
	}
	if fnParamFrame < 0 {
		return nil
	}

	// Classify captures
	type capInfo struct {
		isDynamic bool
		argIdx    int
		constVal  float64
	}
	caps := make([]capInfo, len(capKeys))
	for ci, ck := range capKeys {
		if ck.frame == fnParamFrame && ck.index < len(arity.args) {
			caps[ci] = capInfo{isDynamic: true, argIdx: ck.index}
		} else if ck.frame == fnParamFrame {
			// Same frame as params but index >= nparams: let binding inside fn body.
			// The loop native helper will compute it; use 0 as placeholder.
			caps[ci] = capInfo{constVal: 0}
		} else {
			// Try to resolve from fn's env chain by walking parents
			resolved := false
			e := fn.env
			for e != nil {
				if ck.index < len(e.bindings) {
					obj := e.bindings[ck.index]
					switch v := obj.(type) {
					case coretypes.Int:
						caps[ci] = capInfo{constVal: float64(v.I)}
						resolved = true
					case coretypes.Double:
						caps[ci] = capInfo{constVal: v.D}
						resolved = true
					case *Fn:
						caps[ci] = capInfo{constVal: 0}
						resolved = true
					}
					if resolved {
						break
					}
				}
				e = e.parent
			}
			if !resolved {
				return nil
			}
		}
	}

	loopNative, ok := runtimeExec.NativeHelper(loopProg)
	if !ok {
		return nil
	}
	return func(fnArgs []float64) float64 {
		var buf [16]float64
		var loopArgs []float64
		if nSlots <= len(buf) {
			loopArgs = buf[:nSlots]
		} else {
			loopArgs = make([]float64, nSlots)
		}
		copy(loopArgs[:nLoopBinds], initVals)
		for ci, cap := range caps {
			if cap.isDynamic {
				loopArgs[nLoopBinds+ci] = fnArgs[cap.argIdx]
			} else {
				loopArgs[nLoopBinds+ci] = cap.constVal
			}
		}
		return loopNative(loopArgs)
	}
}

// ---- fn_ir_compile.go ----
// ---------- Fn compilation ----------

// irCompileFn attempts to compile a single-arity Fn body into an IRProgram.
// The args become slots 0..n-1. Returns nil if the body can't be compiled.
// selfBinding optionally identifies the binding key for self-recursive calls.
func irCompileFn(fn *Fn) *IRProgram {
	// Variadic-only fn (fn [x & rest] ...)
	if len(fn.fnExpr.arities) == 0 && fn.fnExpr.variadic != nil {
		return irCompileVariadicFn(fn)
	}
	if len(fn.fnExpr.arities) == 0 {
		return nil
	}
	// Single arity: original fast path
	if len(fn.fnExpr.arities) == 1 && fn.fnExpr.variadic == nil {
		arity := fn.fnExpr.arities[0]
		return irCompileSingleArity(fn, arity)
	}
	// Multi-arity: compile each arity separately
	return irCompileMultiArity(fn)
}

func irCompileSingleArity(fn *Fn, arity FnArityExpr) *IRProgram {
	arityKey := &fn.fnExpr.arities[0]
	if cached, ok := irFnCache.Load(arityKey); ok {
		prog := cached.(*IRProgram)
		if prog == irCompileFailed {
			return nil
		}
		return prog
	}

	// Determine the frame from the body
	fnFrame := guessLoopFrame(arity.body)
	if fnFrame < 0 {
		fnFrame = guessFnParamFrame(arity.body, len(arity.args))
	}
	if fnFrame < 0 {
		fnFrame = 1
	}
	// Try compilation with guessed frame; if it fails, retry with frame+1
	// (the guess can pick a capture frame instead of the param frame)
	for attempt := 0; attempt < 2; attempt++ {
		trialFrame := fnFrame + attempt
		prog := irCompileFnWithFrame(fn, arity, trialFrame)
		if prog != nil {
			irFnCache.Store(arityKey, prog)
			return prog
		}
	}
	irFnCache.Store(arityKey, irCompileFailed)
	return nil
}

func irCompileFnWithFrame(fn *Fn, arity FnArityExpr, fnFrame int) *IRProgram {
	// Auto-detect frame if -1
	if fnFrame < 0 {
		fnFrame = guessLoopFrame(arity.body)
		if fnFrame < 0 {
			fnFrame = guessFnParamFrame(arity.body, len(arity.args))
		}
		if fnFrame < 0 {
			fnFrame = 1
		}
	}
	c := &irCompiler{
		bindingMap: make(map[bindingKey]int),
		loopFrame:  -1,
	}
	c.numSlots = len(arity.args)
	c.loopFrame = fnFrame
	for i := range arity.args {
		c.bindingMap[bindingKey{frame: fnFrame, index: i}] = i
	}

	// If the fn is tail-rewritten, its body has RecurExpr nodes
	// that need a recur target (like a loop body)
	if fn.fnExpr.tailRewritten {
		c.recurTargets = []recurTarget{{pc: 0, baseSlot: 0, nBinds: len(arity.args)}}
	}

	// If the fn has a self-binding, mark it for self-recursive IR dispatch
	if fn.fnExpr.self.NameKey() != nil {
		// The self-binding is typically at frame fnFrame-1, index 0
		// (the letfn/fn frame that holds the fn itself)
		c.selfSlot = 0 // will use special irCallSelf opcode
		c.hasSelf = true
		c.selfNArgs = len(arity.args)
	}

	// If the fn was defined via defn, enable var-based self-call detection
	if fn.defVar != nil {
		c.hasSelf = true
		c.selfVar = fn.defVar
		c.selfNArgs = len(arity.args)
	}

	// Compile body
	for i, expr := range arity.body {
		if !c.compileExpr(expr, i == len(arity.body)-1) {
			return nil
		}
	}
	if len(c.code) == 0 {

		return nil
	}
	if c.code[len(c.code)-1] != irReturn {
		c.emit(irReturn)
	}
	// Compute capture slot indices (where each capture goes in the slot array).
	// Actual capture VALUES are resolved dynamically at call time from fn.env.
	if len(c.captureKeys) > 0 {
		capIdxs := make([]int, len(c.captureKeys))
		for ci, ck := range c.captureKeys {
			capIdxs[ci] = c.bindingMap[ck]
		}
		c.captureSlotIdxs = capIdxs
	}
	prog := &IRProgram{
		code:            c.code,
		constants:       c.constants,
		numSlots:        c.numSlots,
		captureKeys:     c.captureKeys,
		captureSlots:    c.captureSlots,
		captureSlotIdxs: c.captureSlotIdxs,
		hasSelf:         c.hasSelf,
		fnExprs:         c.fnExprs,
		traceName:       fn.fnExpr.traceName,
	}
	// Eagerly compile native f64 helper if eligible
	// Pre-compute capture slot set for fast irCallSelf
	if len(c.captureSlotIdxs) > 0 && c.hasSelf {
		prog.captureSlotSet = make([]bool, c.numSlots)
		for _, idx := range c.captureSlotIdxs {
			prog.captureSlotSet[idx] = true
		}
	}
	prog.refreshModel()
	runtimeExec.InstallNativeHelper(prog, irCompileNativeHelper(prog))
	return prog
}

// irCompileMultiArity compiles a multi-arity fn into an IRProgram with
// arityPrograms map for dispatch by arg count.
func irCompileMultiArity(fn *Fn) *IRProgram {
	// Check cache using first arity
	firstArityKey := &fn.fnExpr.arities[0]
	if cached, ok := irFnCache.Load(firstArityKey); ok {
		prog := cached.(*IRProgram)
		if prog == irCompileFailed {
			return nil
		}
		return prog
	}

	programs := make(map[int]*IRProgram)
	for _, arity := range fn.fnExpr.arities {
		prog := irCompileFnWithFrame(fn, arity, -1) // -1 means auto-detect
		if prog == nil {
			// If any arity fails, mark the whole fn as failed
			irFnCache.Store(firstArityKey, irCompileFailed)
			return nil
		}
		programs[len(arity.args)] = prog
	}

	// Handle variadic arity
	var varProg *IRProgram
	varMinArgs := 0
	if fn.fnExpr.variadic != nil {
		va := *fn.fnExpr.variadic
		varProg = irCompileFnWithFrame(fn, va, -1)
		if varProg != nil {
			varMinArgs = len(va.args)
		}
	}

	// Create wrapper program that dispatches by arity
	wrapper := (&IRProgram{
		arityPrograms:   programs,
		variadicProg:    varProg,
		variadicMinArgs: varMinArgs,
		traceName:       fn.fnExpr.traceName,
	}).refreshModel()
	irFnCache.Store(firstArityKey, wrapper)
	return wrapper
}

// irCompileVariadicFn compiles a variadic fn (fn [x & rest] ...).
// The rest parameter is packed into a vector from remaining args.
func irCompileVariadicFn(fn *Fn) *IRProgram {
	va := *fn.fnExpr.variadic
	variadicKey := fn.fnExpr.variadic

	if cached, ok := irFnCache.Load(variadicKey); ok {
		prog := cached.(*IRProgram)
		if prog == irCompileFailed {
			return nil
		}
		return prog
	}

	// The variadic arity has named args + one rest arg.
	// args slice passed to the fn has arbitrary length >= len(va.args)-1
	// (the last arg in va.args is the rest parameter).
	// We compile the body with all named args as slots, plus the rest slot.
	prog := irCompileFnWithFrame(fn, va, -1)
	if prog == nil || len(prog.captureKeys) > 0 {
		// Variadic functions with closed-over bindings need exact rest-slot
		// frame handling. Keep them on the tree-walker until the IR variadic
		// closure path is capture-safe; otherwise forms like (constantly x)
		// can read the packed rest argument instead of the captured value.
		irFnCache.Store(variadicKey, irCompileFailed)
		return nil
	}
	// Mark as variadic so the executor knows to pack rest args
	prog.variadicMinArgs = len(va.args) - 1 // exclude the & rest param from required count
	prog.refreshModel()
	irFnCache.Store(variadicKey, prog)
	return prog
}

// ---- program_envelope.go ----
// ir.go — tiny lowered IR for hot loop/arithmetic subsets.
//
// The IR represents a small subset of Joker expressions as a flat
// instruction sequence with slot-resolved locals. It is interpreted
// by a simple switch loop that avoids the overhead of tree-walking
// Eval, interface dispatch, defer, and frame allocation.
//
// The IR is lowered lazily from LoopExpr bodies when all contained
// expressions fall within the supported subset. Compiled programs
// are cached so the compile cost is only paid once per loop site.

// Opcodes
const (
	irLiteral        = coreir.Literal        // operand: index into constants pool
	irLoadSlot       = coreir.LoadSlot       // operand: slot index in locals
	irStoreSlot      = coreir.StoreSlot      // operand: slot index in locals
	irAdd            = coreir.Add            // pop 2, push sum (Int fast path)
	irSub            = coreir.Sub            // pop 2, push difference
	irMul            = coreir.Mul            // pop 2, push product
	irRem            = coreir.Rem            // pop 2, push remainder
	irDiv            = coreir.Div            // pop 2, push quotient (Double)
	irInc            = coreir.Inc            // pop 1, push +1
	irDec            = coreir.Dec            // pop 1, push -1
	irLt             = coreir.Lt             // pop 2, push Boolean
	irEq             = coreir.Eq             // pop 2, push Boolean
	irIsZero         = coreir.IsZero         // pop 1, push Boolean
	irJumpIfNot      = coreir.JumpIfNot      // operand: target PC (uint16 big-endian in next 2 bytes)
	irJump           = coreir.Jump           // operand: target PC
	irRecur          = coreir.Recur          // operand: nargs (2 bytes) + target PC (2 bytes)
	irReturn         = coreir.Return         // pop 1, return it
	irGet            = coreir.Get            // pop 2 (coll, key), push result or NIL
	irGet3           = coreir.Get3           // pop 3 (coll, key, default), push result
	irAssoc          = coreir.Assoc          // pop 3 (coll, key, val), push new map
	irNth            = coreir.Nth            // pop 2 (coll, index), push element
	irConj           = coreir.Conj           // pop 2 (coll, val), push conj'd
	irSqrt           = coreir.Sqrt           // pop 1, push sqrt
	irCallSlot       = coreir.CallSlot       // operand1: slot (2 bytes), operand2: nargs (2 bytes)
	irCallSelf       = coreir.CallSelf       // operand: nargs (2 bytes)
	irFirst          = coreir.First          // pop 1, push first element
	irBuildVec       = coreir.BuildVec       // operand: n elements; pop n, push new vector
	irStr2           = coreir.Str2           // pop 2, push string concatenation
	irStr1           = coreir.Str1           // pop 1, push string conversion
	irNthStringASCII = coreir.NthStringASCII // operand: constant string index; pop idx, push char
	irCount          = coreir.Count          // pop 1, push count
	irToTransient    = coreir.ToTransient    // pop 1 (corecollections.ArrayVector), push TransientVector
	irAssocBang      = coreir.AssocBang      // pop 3 (tv, key, val), mutate in place, push tv
	irToPersistent   = coreir.ToPersistent   // pop 1 (TransientVector), push corecollections.ArrayVector
	irFallback       = coreir.Fallback       // cannot execute in IR; fall back to tree Eval
	irIntCast        = coreir.IntCast        // pop 1 (Char or coretypes.Number), push Int
	irSubs           = coreir.Subs           // pop 2 or 3 (string, start [, end]), push substring
	irGte            = coreir.Gte            // pop 2, push a >= b
	irGt             = coreir.Gt             // pop 2, push a > b
	irLte            = coreir.Lte            // pop 2, push a <= b
	irCursorChar     = coreir.CursorChar     // pop cursor, push char (rune as Char)
	irCursorNext     = coreir.CursorNext     // pop cursor, push new cursor (advanced by 1)
	irCursorDone     = coreir.CursorDone     // pop cursor, push boolean (done?)
	irPackRest       = coreir.PackRest       // operand: startIdx — pack slots[startIdx:nargs] into vector, store to slot
	irApply          = coreir.Apply          // pop fn + args-seq, call fn with unpacked args, push result
	irThrow          = coreir.Throw          // pop value, panic with it
	irTryCatch       = coreir.TryCatch       // operands: catchPC(2) + bindSlot(2) — set up catch handler
	irPop            = coreir.Pop            // pop and discard top of stack
	irMakeFn         = coreir.MakeFn         // operand: constant index (FnExpr) — creates *Fn with current env
	irCase           = coreir.Case           // operands: slot(2) + nCases(2) + [value(2)+targetPC(2)]*n + defaultPC(2)
	irBitAnd         = coreir.BitAnd         // pop 2, push a & b
	irBitOr          = coreir.BitOr          // pop 2, push a | b
	irBitNot         = coreir.BitNot         // pop 1, push ^a
	irBitShiftLeft   = coreir.BitShiftLeft   // pop 2, push a << b
	irBitShiftRight  = coreir.BitShiftRight  // pop 2, push a >> b (arithmetic)
)

// AnalyzeIRProgram returns a conservative program-shape summary for diagnostics
// and optimization gates.
func AnalyzeIRProgram(prog *IRProgram) coreir.Analysis {
	if prog == nil {
		return coreir.Analysis{SuggestedPath: "none"}
	}
	prog.stateMu.Lock()
	defer prog.stateMu.Unlock()
	return analyzeIRProgramLocked(prog)
}

func analyzeIRProgramLocked(prog *IRProgram) coreir.Analysis {
	if prog.analysis != nil {
		return *prog.analysis
	}
	if prog.escapeInfo == nil {
		prog.escapeInfo = analyzeEscapes(prog)
	}
	floatConsts := prog.neutralFloatConsts()
	a := coreir.Analyze(
		prog.code,
		prog.numSlots,
		len(prog.captureKeys),
		corewasm.UsesFloat(prog.code, len(floatConsts) > 0),
		prog.escapeInfo.StringBuilderSlots,
		prog.escapeInfo.StringPrependSlots,
	)
	prog.analysis = &a
	if prog.model == nil {
		prog.refreshModelLocked()
	}
	prog.model.Analysis = &a
	return a
}

// ---------- Cache ----------

var irCache sync.Map   // map[*LoopExpr]*IRProgram
var irFnCache sync.Map // map[*FnArityExpr]*IRProgram

var irCompileFailed = &IRProgram{} // sentinel

func irGetCached(loop *LoopExpr) *IRProgram {
	if cached, ok := irCache.Load(loop); ok {
		prog := cached.(*IRProgram)
		if prog == irCompileFailed {
			return nil
		}
		return prog
	}
	prog := irCompile(loop)
	if prog == nil {
		irCache.Store(loop, irCompileFailed)
		return nil
	}
	irCache.Store(loop, prog)
	return prog
}

// ---------- Program ----------

type IRProgram struct {
	stateMu         sync.RWMutex
	model           *coreir.Program
	code            []byte
	constants       []coretypes.Object
	numSlots        int
	captureKeys     []bindingKey
	captureSlots    []coretypes.Object // resolved capture values from fn.env
	captureSlotIdxs []int              // slot indices for each capture
	hasSelf         bool
	escapeInfo      *EscapeInfo
	analysis        *coreir.Analysis
	typedFailed     atomic.Bool
	execFailed      atomic.Bool // both typed AND boxed failed — skip IR entirely
	memNthFailed    atomic.Bool
	nativeHelper    nativeF64Fn
	nativeHelper2   nativeF64Fn2
	nativeChecked   bool
	floatConsts     []float64
	// Multi-arity support: map from arg count to sub-program
	arityPrograms   map[int]*IRProgram
	variadicProg    *IRProgram // for variadic arity (min args)
	variadicMinArgs int
	fnExprs         []*FnExpr // for irMakeFn opcode
	traceName       string
	captureSlotSet  []bool // captureSlotSet[i] = true if slot i holds a capture (skip clearing)
}

func (p *IRProgram) neutralModel() *coreir.Program {
	if p == nil {
		return nil
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.model == nil {
		p.refreshModelLocked()
	}
	return p.model
}

func (p *IRProgram) neutralFloatConsts() []float64 {
	if p == nil {
		return nil
	}
	if len(p.floatConsts) > 0 {
		return append([]float64(nil), p.floatConsts...)
	}
	var floats []float64
	for _, constant := range p.constants {
		if v, ok := constant.(coretypes.Double); ok {
			floats = append(floats, v.D)
		}
	}
	return floats
}

func (p *IRProgram) refreshModel() *IRProgram {
	if p == nil {
		return nil
	}
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	return p.refreshModelLocked()
}

func (p *IRProgram) refreshModelLocked() *IRProgram {
	model := coreir.NewProgram(p.code, p.numSlots, len(p.constants))
	model.HasSelf = p.hasSelf
	model.FloatConsts = p.neutralFloatConsts()
	model.WithCaptures(p.captureSlotIdxs, p.captureSlotSet)
	if p.analysis != nil {
		analysis := coreir.Analyze(p.code, p.numSlots, len(p.captureKeys), corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0), p.analysis.StringAppendSlots, p.analysis.StringPrependSlots)
		model.Analysis = &analysis
	}
	if len(p.arityPrograms) > 0 || p.variadicProg != nil || p.variadicMinArgs != 0 {
		arityPrograms := make(map[int]*coreir.Program, len(p.arityPrograms))
		for arity, prog := range p.arityPrograms {
			if prog != nil {
				arityPrograms[arity] = prog.refreshModel().model
			}
		}
		var variadic *coreir.Program
		if p.variadicProg != nil {
			variadic = p.variadicProg.refreshModel().model
		}
		model.WithArityPrograms(arityPrograms, variadic, p.variadicMinArgs)
	}
	p.model = model
	return p
}

// ---- escape_analysis.go ----
// escape_analysis.go — determines which IR slots can safely use in-place mutation.
//
// A slot is "non-escaping" if:
// 1. It is only read via irLoadSlot
// 2. It is only written via irStoreSlot or irRecur
// 3. It is only consumed by irAssoc/irNth/irGet/irGet3 (collection ops that
//    produce new values without retaining references to the original)
// 4. It is NOT passed to irCallSlot/irCallSelf (which could alias it)
//
// Non-escaping vector slots can use in-place mutation (transient optimization).

type EscapeInfo = coreir.EscapeInfo

// analyzeEscapes adapts the root-owned IRProgram to the cycle-free bytecode analyzer.
func analyzeEscapes(prog *IRProgram) *EscapeInfo {
	if prog == nil {
		return &EscapeInfo{}
	}
	return coreir.AnalyzeEscapes(prog.code, prog.numSlots)
}

// ---- runtime_ir_exports.go ----
// ir_exported.go — exported functions for the joker.runtime namespace.
// These bridge internal IR/WASM/escape analysis to the public API.

// IrDisassemble returns a human-readable disassembly of an IR program.
func IrDisassemble(prog *IRProgram) string {
	if prog == nil {
		return "; nil program"
	}
	model := prog.neutralModel()
	return coreir.Disassemble(model.Code, func(idx int) string {
		if idx < len(prog.constants) && prog.constants[idx] != nil {
			return prog.constants[idx].ToString(false)
		}
		return ""
	})
}

// ExplainWASMEligibility exposes the WASM diagnostic for a program.
func ExplainWASMEligibility(prog *IRProgram) corewasm.Diagnostic {
	return explainWASMEligibility(prog)
}

// AnalyzeEscapesExported returns the safe-mutable-slots boolean array.
func AnalyzeEscapesExported(prog *IRProgram) []bool {
	info := analyzeEscapes(prog)
	return info.SafeMutableSlots
}

// IRProgram accessor methods for external packages.
func (p *IRProgram) CodeLen() int {
	model := p.neutralModel()
	return len(model.Code)
}

func (p *IRProgram) CodeBytes() []byte {
	model := p.neutralModel()
	return append([]byte(nil), model.Code...)
}

func (p *IRProgram) ConstLen() int { return len(p.constants) }
func (p *IRProgram) Constants() []coretypes.Object {
	return append([]coretypes.Object(nil), p.constants...)
}
func (p *IRProgram) NumSlots() int {
	model := p.neutralModel()
	return model.NumSlots
}
func (p *IRProgram) HasSelf() bool                    { return p.hasSelf }
func (p *IRProgram) CaptureSlots() []coretypes.Object { return p.captureSlots }
func (p *IRProgram) GetNativeHelper() func([]float64) float64 {
	if nativeHelper, ok := runtimeExec.NativeHelper(p); ok {
		return func(args []float64) float64 { return nativeHelper(args) }
	}
	return nil
}

// Exports for std/jit and std/runtime namespaces.
func IrCompileFn(fn *Fn) *IRProgram                                      { return irCompileFn(fn) }
func IrExecTyped(prog *IRProgram, s []coretypes.Object) coretypes.Object { return irExecTyped(prog, s) }
func IrExec(prog *IRProgram, s []coretypes.Object) coretypes.Object      { return irExec(prog, s) }

func WasmCompileFnExported(fn *Fn) (coretypes.Object, string) {
	if len(fn.fnExpr.arities) == 1 {
		arity := fn.fnExpr.arities[0]
		if len(arity.body) == 1 {
			if loop, ok := arity.body[0].(*LoopExpr); ok {
				if prog := irCompile(loop); prog != nil {
					if wrapper := buildWasmLoopWrapper(fn, arity, loop, prog); wrapper != nil {
						return wrapper, ""
					}
					if d := explainWASMEligibility(prog); !d.Eligible && d.Reason != "" {
						return nil, d.Reason
					}
				}
			}
		}
	}
	prog := irCompileFn(fn)
	if prog == nil {
		return nil, "function cannot be compiled to IR"
	}
	wp := wasmCompile(prog)
	if wp == nil {
		d := ExplainWASMEligibility(prog)
		if d.Reason != "" {
			return nil, d.Reason
		}
		return nil, "unsupported IR shape"
	}
	numSlots := prog.numSlots
	numArgs := len(fn.fnExpr.arities[0].args)
	return Proc{
		Fn: func(args []coretypes.Object) coretypes.Object {
			// Pad args to fill all WASM params (slots beyond fn args are loop vars, init to 0)
			slots := make([]coretypes.Object, numSlots)
			copy(slots, args)
			for i := numArgs; i < numSlots; i++ {
				slots[i] = coretypes.Int{I: 0}
			}
			result := wasmExec(wp, slots)
			if result == nil {
				panic(RT.NewError("jit/compile-wasm: WASM execution failed"))
			}
			return result
		},
		Name: "jit-wasm-compiled",
	}, ""
}

func IsFloatExported(prog *IRProgram) bool {
	model := prog.neutralModel()
	return model != nil && corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0)
}

func IrToWasmExported(prog *IRProgram) []byte { return irToWasm(prog) }

func WasmCompileExported(prog *IRProgram) *WasmProgram {
	return wasmCompile(prog)
}

func WasmExecExported(wp *WasmProgram, s []coretypes.Object) coretypes.Object {
	return wasmExec(wp, s)
}

func WasmCompileBytesExported(prog *IRProgram) []byte {
	wp := wasmCompile(prog)
	if wp == nil {
		return nil
	}
	return append([]byte(nil), wp.bytes...)
}

type IRAnalysisExported struct {
	Eligible       bool
	Path           string
	HasCallSlot    bool
	HasSelfCall    bool
	UsesCollection bool
	UsesString     bool
	HasMapOps      bool
	HasAssoc       bool
	HasGenericNth  bool
}

func AnalyzeIRProgramExported(prog *IRProgram) IRAnalysisExported {
	a := AnalyzeIRProgram(prog)
	return IRAnalysisExported{
		Eligible:       irTypedEligible(a),
		Path:           a.SuggestedPath,
		HasCallSlot:    a.HasCallSlot,
		HasSelfCall:    a.HasSelfCall,
		UsesCollection: a.UsesCollection,
		UsesString:     a.UsesString,
		HasMapOps:      a.HasMapOps,
		HasAssoc:       a.HasAssoc,
		HasGenericNth:  a.HasGenericNth,
	}
}

// ---- native_recursive.go ----
// native_recursive.go — Native Go code generation for pure-integer recursive fns.
//
// When a fn body contains only integer arithmetic, comparisons, and self-recursive
// calls (no collections, strings, or other types), we compile to fixed-arity
// native Go functions that run without coretypes.Object boxing, interface dispatch, or
// slice allocation per call.

// nativeIntFn1 through nativeIntFn3 are typed native function signatures.
type nativeIntFn1 func(a int) int
type nativeIntFn2 func(a, b int) int
type nativeIntFn3 func(a, b, c int) int

// nativeRecursiveEntry holds a compiled native fn for a specific arity.
var nativeCanonicalVars = map[*Var]*Fn{}

func init() {
	for _, vr := range GLOBAL_ENV.CoreNamespace.mappings {
		if coreVarToProcName(vr) != "" {
			if fn, ok := vr.Value.(*Fn); ok {
				nativeCanonicalVars[vr] = fn
			}
		}
	}
}

func nativeCoreBindingsUnchanged() bool {
	for vr, value := range nativeCanonicalVars {
		if vr.Value != value {
			return false
		}
	}
	return true
}

type nativeRecursiveEntry struct {
	dependencies    map[*Var]*Fn
	numericFallback func([]coretypes.Object) coretypes.Object
	arity           int
	fn1             nativeIntFn1
	fn2             nativeIntFn2
	fn3             nativeIntFn3
}

var nativeRecursiveCache sync.Map // *Fn → *nativeRecursiveEntry (or nativeRecursiveFailed sentinel)
var nativeRecursiveFailed = &nativeRecursiveEntry{arity: -1}

func tryNativeRecursive(fn *Fn) *nativeRecursiveEntry {
	if cached, ok := nativeRecursiveCache.Load(fn); ok {
		entry := cached.(*nativeRecursiveEntry)
		if entry == nativeRecursiveFailed {
			return nil
		}
		for vr, value := range entry.dependencies {
			if vr.Value != value {
				return nil
			}
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

	deps := map[*Var]*Fn{}
	var inspect func(Expr) bool
	inspect = func(expr Expr) bool {
		switch e := expr.(type) {
		case *CallExpr:
			v, ok := e.callable.(*VarRefExpr)
			if !ok {
				return false
			}
			if v.vr != selfVar {
				canonical, ok := nativeCanonicalVars[v.vr]
				if !ok || v.vr.Value != canonical {
					return false
				}
				deps[v.vr] = canonical
			}
			for _, arg := range e.args {
				if !inspect(arg) {
					return false
				}
			}
		case *IfExpr:
			return inspect(e.cond) && inspect(e.positive) && inspect(e.negative)
		}
		return true
	}
	if !inspect(arity.body[0]) {
		return nil
	}
	entry := &nativeRecursiveEntry{arity: nargs, dependencies: deps}
	entry.numericFallback = func(args []coretypes.Object) coretypes.Object {
		return evalNativeNumeric(arity.body[0], selfVar, paramFrame, args, entry)
	}

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

// Native expressions contain only integer arithmetic, comparisons, branches and
// self calls. Re-evaluating this restricted tree after overflow cannot replay I/O
// or mutations; unlike general IR, it is safe to restart with promoted numbers.
var nativeIntegerOverflow = &struct{}{}

func nativeCheckedAdd(a, b int) int {
	r := a + b
	if (b > 0 && r < a) || (b < 0 && r > a) {
		panic(nativeIntegerOverflow)
	}
	return r
}
func nativeCheckedSub(a, b int) int {
	r := a - b
	if (b < 0 && r < a) || (b > 0 && r > a) {
		panic(nativeIntegerOverflow)
	}
	return r
}
func nativeCheckedMul(a, b int) int {
	r := a * b
	if a != 0 && ((a == -1 && b == coretypes.MinInt) || (b == -1 && a == coretypes.MinInt) || r/a != b) {
		panic(nativeIntegerOverflow)
	}
	return r
}
func evalNativeNumeric(expr Expr, self *Var, frame int, args []coretypes.Object, entry *nativeRecursiveEntry) coretypes.Object {
	switch e := expr.(type) {
	case *LiteralExpr:
		return e.obj
	case *BindingExpr:
		return args[e.binding.index]
	case *IfExpr:
		cond := evalNativeNumeric(e.cond, self, frame, args, entry).(coretypes.Boolean)
		if cond.B {
			return evalNativeNumeric(e.positive, self, frame, args, entry)
		}
		return evalNativeNumeric(e.negative, self, frame, args, entry)
	case *CallExpr:
		values := make([]coretypes.Object, len(e.args))
		for i, arg := range e.args {
			values[i] = evalNativeNumeric(arg, self, frame, args, entry)
		}
		vr := e.callable.(*VarRefExpr).vr
		if vr == self {
			return entry.numericFallback(values)
		}
		switch coreVarToProcName(vr) {
		case "procAdd":
			return procAdd(values)
		case "procSubtract":
			return procSubtract(values)
		case "procMultiply":
			return procMultiply(values)
		case "procInc":
			return procInc(values)
		case "procDec":
			return procDec(values)
		case "procLt":
			return procLt(values)
		case "procLte":
			return procLte(values)
		case "procGt":
			return procGt(values)
		case "procGte":
			return procGte(values)
		case "procEq":
			return procEq(values)
		}
	}
	panic("unsupported expression in native numeric recovery")
}

func callNativeRecursive(entry *nativeRecursiveEntry, args []coretypes.Object) (result coretypes.Object) {
	defer func() {
		if failure := recover(); failure != nil {
			if failure != nativeIntegerOverflow {
				panic(failure)
			}
			result = entry.numericFallback(args)
		}
	}()
	// Leave invalid arities to Fn.Call's language-level error path. Native
	// closures must neither index missing arguments nor ignore extra ones.
	if len(args) != entry.arity {
		return nil
	}
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
		return func(x int) int { return nativeCheckedAdd(a(x), b(x)) }
	case "procSubtract":
		if len(args) == 1 {
			a := compileIntExpr1(args[0], selfVar, pf, entry)
			if a == nil {
				return nil
			}
			return func(x int) int { return nativeCheckedSub(0, a(x)) }
		}
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		b := compileIntExpr1(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x int) int { return nativeCheckedSub(a(x), b(x)) }
	case "procMultiply":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		b := compileIntExpr1(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x int) int { return nativeCheckedMul(a(x), b(x)) }
	case "procInc":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x int) int { return nativeCheckedAdd(a(x), 1) }
	case "procDec":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr1(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x int) int { return nativeCheckedSub(a(x), 1) }
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
		return func(x, y, z int) int { return nativeCheckedAdd(a(x, y, z), b(x, y, z)) }
	case "procSubtract":
		if len(args) == 1 {
			a := compileIntExpr3(args[0], selfVar, pf, entry)
			if a == nil {
				return nil
			}
			return func(x, y, z int) int { return nativeCheckedSub(0, a(x, y, z)) }
		}
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		b := compileIntExpr3(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y, z int) int { return nativeCheckedSub(a(x, y, z), b(x, y, z)) }
	case "procMultiply":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		b := compileIntExpr3(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y, z int) int { return nativeCheckedMul(a(x, y, z), b(x, y, z)) }
	case "procInc":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y, z int) int { return nativeCheckedAdd(a(x, y, z), 1) }
	case "procDec":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr3(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y, z int) int { return nativeCheckedSub(a(x, y, z), 1) }
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
		return func(x, y int) int { return nativeCheckedAdd(a(x, y), b(x, y)) }
	case "procSubtract":
		if len(args) == 1 {
			a := compileIntExpr2(args[0], selfVar, pf, entry)
			if a == nil {
				return nil
			}
			return func(x, y int) int { return nativeCheckedSub(0, a(x, y)) }
		}
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		b := compileIntExpr2(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y int) int { return nativeCheckedSub(a(x, y), b(x, y)) }
	case "procMultiply":
		if len(args) != 2 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		b := compileIntExpr2(args[1], selfVar, pf, entry)
		if a == nil || b == nil {
			return nil
		}
		return func(x, y int) int { return nativeCheckedMul(a(x, y), b(x, y)) }
	case "procInc":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y int) int { return nativeCheckedAdd(a(x, y), 1) }
	case "procDec":
		if len(args) != 1 {
			return nil
		}
		a := compileIntExpr2(args[0], selfVar, pf, entry)
		if a == nil {
			return nil
		}
		return func(x, y int) int { return nativeCheckedSub(a(x, y), 1) }
	}
	return nil
}

// ---- loop_compiler.go ----

// ---- loop_compiler.go ----
// ---------- Compiler ----------

type bindingKey struct {
	frame int
	index int
}

type irCompiler struct {
	code             []byte
	constants        []coretypes.Object
	bindingMap       map[bindingKey]int
	captureKeys      []bindingKey
	captureSlots     []coretypes.Object
	captureSlotIdxs  []int
	numSlots         int
	loopFrame        int
	depth            int
	hasSelf          bool
	selfSlot         int
	selfNArgs        int
	selfVar          *Var // for defn-style var-based self-calls
	recurTargets     []recurTarget
	rejectReason     string
	hasCollectionOps bool
	fnExprs          []*FnExpr
}

type recurTarget struct {
	pc       int // bytecode offset of loop start
	baseSlot int // first slot of this loop's bindings
	nBinds   int // number of loop bindings
}

func irCompile(loop *LoopExpr) *IRProgram {
	prog, _ := irCompileExplain(loop)
	return prog
}

func irCompileExplain(loop *LoopExpr) (*IRProgram, string) {
	c := &irCompiler{
		bindingMap: make(map[bindingKey]int),
		loopFrame:  -1,
	}
	// Pre-scan loop body for collection ops to gate arithmetic helper inlining
	le := (*LetExpr)(loop)
	for _, b := range le.body {
		if exprHasCollectionOp(b) {
			c.hasCollectionOps = true
			break
		}
	}
	c.numSlots = len(loop.names)

	loopLet := (*LetExpr)(loop)
	c.loopFrame = guessLoopFrame(loopLet.body)
	if c.loopFrame < 0 {
		c.loopFrame = 1
	}
	for i := range loop.names {
		c.bindingMap[bindingKey{frame: c.loopFrame, index: i}] = i
	}

	// Push the top-level recur target (PC=0, slots 0..n-1)
	c.recurTargets = []recurTarget{{pc: 0, baseSlot: 0, nBinds: len(loop.names)}}

	for i, expr := range loopLet.body {
		if !c.compileExpr(expr, i == len(loopLet.body)-1) {
			return nil, c.reasonOr("IR compiler rejected loop body")
		}
	}
	if len(c.code) == 0 {
		return nil, "IR compiler emitted no code"
	}
	if c.code[len(c.code)-1] != irReturn && c.code[len(c.code)-1] != irJump {
		c.emit(irReturn)
	}
	// Safety limit: too many captures indicates complex nested scoping
	if len(c.captureKeys) > 12 {
		return nil, fmt.Sprintf("too many captured bindings: %d > 12", len(c.captureKeys))
	}
	// Validate: ensure no slot is assigned twice
	slotUsed := make(map[int]bool, c.numSlots)
	for _, slot := range c.bindingMap {
		if slotUsed[slot] {
			return nil, fmt.Sprintf("IR slot collision detected at slot %d", slot)
		}
		slotUsed[slot] = true
	}
	return (&IRProgram{
		code:        c.code,
		constants:   c.constants,
		numSlots:    c.numSlots,
		captureKeys: c.captureKeys,
		fnExprs:     c.fnExprs,
	}).refreshModel(), ""
}

func (c *irCompiler) reject(format string, args ...interface{}) bool {
	if c.rejectReason == "" {
		c.rejectReason = fmt.Sprintf(format, args...)
	}
	return false
}

func (c *irCompiler) reasonOr(fallback string) string {
	if c.rejectReason != "" {
		return c.rejectReason
	}
	return fallback
}

// guessFnParamFrame scans a fn body for BindingExpr nodes that reference
// indices 0..nparams-1, returning the common frame. Returns -1 if ambiguous.
func guessFnParamFrame(body []Expr, nparams int) int {
	if nparams == 0 {
		return -1
	}
	// Collect all (frame, index) pairs from BindingExprs with index < nparams.
	// The fn param frame is the smallest frame where ALL indices 0..nparams-1 appear.
	frameSeen := map[int]map[int]bool{}
	var scan func(e Expr)
	scan = func(e Expr) {
		switch x := e.(type) {
		case *BindingExpr:
			if x.binding.index < nparams {
				if frameSeen[x.binding.frame] == nil {
					frameSeen[x.binding.frame] = map[int]bool{}
				}
				frameSeen[x.binding.frame][x.binding.index] = true
			}
		case *LoopExpr:
			le := (*LetExpr)(x)
			for _, v := range le.values {
				scan(v)
			}
			for _, b := range le.body {
				scan(b)
			}
		case *LetExpr:
			for _, v := range x.values {
				scan(v)
			}
			for _, b := range x.body {
				scan(b)
			}
		case *IfExpr:
			scan(x.cond)
			scan(x.positive)
			scan(x.negative)
		case *CallExpr:
			scan(x.callable)
			for _, a := range x.args {
				scan(a)
			}
		case *RecurExpr:
			for _, a := range x.args {
				scan(a)
			}
		}
	}
	for _, e := range body {
		scan(e)
	}
	// Find smallest frame with all nparams indices present
	bestFrame := -1
	for f, idxSet := range frameSeen {
		if len(idxSet) >= nparams {
			if bestFrame < 0 || f < bestFrame {
				bestFrame = f
			}
		}
	}
	return bestFrame
}

func guessLoopFrame(body []Expr) int {
	for _, expr := range body {
		if f := findRecurBindingFrame(expr); f >= 0 {
			return f
		}
	}
	for _, expr := range body {
		if f := findBindingFrame(expr); f >= 0 {
			return f
		}
	}
	return -1
}

func findRecurBindingFrame(expr Expr) int {
	switch e := expr.(type) {
	case *RecurExpr:
		for _, arg := range e.args {
			if f := findBindingFrame(arg); f >= 0 {
				return f
			}
		}
	case *IfExpr:
		if f := findRecurBindingFrame(e.positive); f >= 0 {
			return f
		}
		return findRecurBindingFrame(e.negative)
	case *LetExpr:
		for _, b := range e.body {
			if f := findRecurBindingFrame(b); f >= 0 {
				return f
			}
		}
	case *CallExpr:
		for _, arg := range e.args {
			if f := findRecurBindingFrame(arg); f >= 0 {
				return f
			}
		}
	}
	return -1
}

func findBindingFrame(expr Expr) int {
	switch e := expr.(type) {
	case *BindingExpr:
		return e.binding.frame
	case *IfExpr:
		if f := findBindingFrame(e.cond); f >= 0 {
			return f
		}
		if f := findBindingFrame(e.positive); f >= 0 {
			return f
		}
		return findBindingFrame(e.negative)
	case *CallExpr:
		for _, arg := range e.args {
			if f := findBindingFrame(arg); f >= 0 {
				return f
			}
		}
	case *RecurExpr:
		for _, arg := range e.args {
			if f := findBindingFrame(arg); f >= 0 {
				return f
			}
		}
	case *LetExpr:
		for _, v := range e.values {
			if f := findBindingFrame(v); f >= 0 {
				return f
			}
		}
	}
	return -1
}

func (c *irCompiler) emit(op byte) {
	c.code = append(c.code, op)
}

func (c *irCompiler) emitWithOperand(op byte, operand int) {
	c.code = append(c.code, op, byte(operand>>8), byte(operand))
}

func (c *irCompiler) patchJump(pos int, target int) {
	c.code[pos+1] = byte(target >> 8)
	c.code[pos+2] = byte(target)
}

func (c *irCompiler) addConstant(obj coretypes.Object) int {
	for i, existing := range c.constants {
		if existing.Equals(obj) {
			return i
		}
	}
	c.constants = append(c.constants, obj)
	return len(c.constants) - 1
}

func isASCIIBytes(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}

func (c *irCompiler) constantASCIIString(expr Expr) (string, bool) {
	switch e := expr.(type) {
	case *LiteralExpr:
		if s, ok := e.obj.(coretypes.String); ok && isASCIIBytes(s.S) {
			return s.S, true
		}
	case *BindingExpr:
		if lit, ok := e.binding.value.(*LiteralExpr); ok {
			if s, ok := lit.obj.(coretypes.String); ok && isASCIIBytes(s.S) {
				return s.S, true
			}
		}
	}
	return "", false
}

func (c *irCompiler) constantCount(expr Expr) (int, bool) {
	switch e := expr.(type) {
	case *LiteralExpr:
		switch v := e.obj.(type) {
		case coretypes.String:
			return v.Count(), true
		case coretypes.Counted:
			return v.Count(), true
		}
	case *BindingExpr:
		// Only fold captured/outer bindings. Loop-local bindings can change via
		// recur even when their initial value is a literal.
		if e.binding.frame < c.loopFrame {
			if lit, ok := e.binding.value.(*LiteralExpr); ok {
				switch v := lit.obj.(type) {
				case coretypes.String:
					return v.Count(), true
				case coretypes.Counted:
					return v.Count(), true
				}
			}
		}
	}
	return 0, false
}

func (c *irCompiler) compileExpr(expr Expr, isLast bool) bool {
	switch e := expr.(type) {
	case *LiteralExpr:
		idx := c.addConstant(e.obj)
		c.emitWithOperand(irLiteral, idx)
		if isLast {
			c.emit(irReturn)
		}
		return true

	case *VectorExpr:
		// Try constant vector first (all elements are literals)
		allLiteral := true
		for _, elem := range e.v {
			if _, ok := elem.(*LiteralExpr); !ok {
				allLiteral = false
				break
			}
		}
		if allLiteral {
			arr := make([]coretypes.Object, len(e.v))
			for i, elem := range e.v {
				arr[i] = elem.(*LiteralExpr).obj
			}
			idx := c.addConstant(&corecollections.ArrayVector{Arr: arr})
			c.emitWithOperand(irLiteral, idx)
		} else {
			// Compile each element, then emit a vector-build opcode
			for _, elem := range e.v {
				if !c.compileExpr(elem, false) {
					return false
				}
			}
			c.emitWithOperand(irBuildVec, len(e.v))
		}
		if isLast {
			c.emit(irReturn)
		}
		return true

	case *MapExpr:
		allLiteral := true
		for i := range e.keys {
			if _, ok := e.keys[i].(*LiteralExpr); !ok {
				allLiteral = false
				break
			}
			if _, ok := e.values[i].(*LiteralExpr); !ok {
				allLiteral = false
				break
			}
		}
		if !allLiteral {
			return c.reject("unsupported dynamic map literal in IR")
		}
		var obj coretypes.Object
		if int64(len(e.keys)) > corecollections.HASHMAP_THRESHOLD/2 {
			res := corecollections.EmptyHashMap
			for i := range e.keys {
				key := e.keys[i].(*LiteralExpr).obj
				if res.ContainsKey(key) {
					return c.reject("duplicate key in IR map literal: %s", key.ToString(false))
				}
				res = res.Assoc(key, e.values[i].(*LiteralExpr).obj).(*corecollections.HashMap)
			}
			obj = res
		} else {
			res := corecollections.EmptyArrayMap()
			for i := range e.keys {
				key := e.keys[i].(*LiteralExpr).obj
				if !res.Add(key, e.values[i].(*LiteralExpr).obj) {
					return c.reject("duplicate key in IR map literal: %s", key.ToString(false))
				}
			}
			obj = res
		}
		idx := c.addConstant(obj)
		c.emitWithOperand(irLiteral, idx)
		if isLast {
			c.emit(irReturn)
		}
		return true

	case *BindingExpr:
		key := bindingKey{frame: e.binding.frame, index: e.binding.index}
		slot, ok := c.bindingMap[key]
		if !ok {
			if e.binding.frame < c.loopFrame {
				slot = c.numSlots
				c.bindingMap[key] = slot
				c.captureKeys = append(c.captureKeys, key)
				c.numSlots++
			} else {
				return c.reject("binding frame %d index %d is not in loop frame %d and cannot be captured", e.binding.frame, e.binding.index, c.loopFrame)
			}
		}
		c.emitWithOperand(irLoadSlot, slot)
		if isLast {
			c.emit(irReturn)
		}
		return true

	case *IfExpr:
		if !c.compileExpr(e.cond, false) {
			return false
		}
		jumpPos := len(c.code)
		c.emitWithOperand(irJumpIfNot, 0)
		if !c.compileExpr(e.positive, isLast) {
			return false
		}
		if !isLast {
			skipPos := len(c.code)
			c.emitWithOperand(irJump, 0)
			c.patchJump(jumpPos, len(c.code))
			if !c.compileExpr(e.negative, isLast) {
				return false
			}
			c.patchJump(skipPos, len(c.code))
		} else {
			c.patchJump(jumpPos, len(c.code))
			if !c.compileExpr(e.negative, isLast) {
				return false
			}
		}
		return true

	case *CallExpr:
		return c.compileCall(e, isLast)

	case *RecurExpr:
		if len(c.recurTargets) == 0 {
			return c.reject("recur used outside a loop target")
		}
		target := c.recurTargets[len(c.recurTargets)-1]
		for _, arg := range e.args {
			if !c.compileExpr(arg, false) {
				return false
			}
		}
		// Emit: nargs (2) + targetPC (2) [+ baseSlot (2) if nested]
		c.code = append(c.code, irRecur,
			byte(len(e.args)>>8), byte(len(e.args)),
			byte(target.pc>>8), byte(target.pc))
		if target.pc != 0 {
			// Nested loop: also emit baseSlot
			c.code = append(c.code, byte(target.baseSlot>>8), byte(target.baseSlot))
		}
		return true

	case *LetExpr:
		if c.depth > 16 {
			return c.reject("IR nesting depth exceeded for let: %d > 16", c.depth)
		}
		c.depth++
		return c.compileLetBody(e, isLast)

	case *LoopExpr:
		if c.depth > 16 {
			return c.reject("IR nesting depth exceeded for nested loop: %d > 16", c.depth)
		}
		c.depth++
		return c.compileNestedLoop(e, isLast)

	case *TryExpr:
		// Executors do not implement irTryCatch. Reject before execution;
		// emitting a late unsupported opcode would replay preceding effects.
		return c.reject("try/catch requires interpreter execution")

	case *FnExpr:
		// Store FnExpr index for irMakeFn opcode
		if c.fnExprs == nil {
			c.fnExprs = make([]*FnExpr, 0)
		}
		idx := len(c.fnExprs)
		c.fnExprs = append(c.fnExprs, e)
		c.emitWithOperand(irMakeFn, idx)
		if isLast {
			c.emit(irReturn)
		}
		return true

	case *DoExpr:
		for i, bodyExpr := range e.body {
			if !c.compileExpr(bodyExpr, isLast && i == len(e.body)-1) {
				return false
			}
			if i < len(e.body)-1 {
				c.emit(irPop)
			}
		}
		if len(e.body) == 0 {
			c.emitWithOperand(irLiteral, c.addConstant(NIL))
			if isLast {
				c.emit(irReturn)
			}
		}
		return true

	default:
		return c.reject("unsupported IR expression type %T", expr)
	}
}

func (c *irCompiler) compileLetBody(e *LetExpr, isLast bool) bool {
	// Detect let frame using precise binding reference analysis
	letFrame := findLetFrame(e.body, len(e.values), c.bindingMap)
	if letFrame < 0 {
		for _, bodyExpr := range e.body {
			if f := findBindingFrame(bodyExpr); f > c.loopFrame {
				letFrame = f
				break
			}
		}
	}
	if letFrame < 0 {
		letFrame = c.loopFrame + c.depth
	}
	// Save ALL existing bindings for this frame (not just the indices we'll
	// overwrite) so we can restore after the let scope exits. This prevents
	// inner let scopes from corrupting outer scope binding maps when the
	// parser assigns the same frame number to multiple scopes.
	savedBindings := make(map[bindingKey]int)
	for key, slot := range c.bindingMap {
		if key.frame == letFrame {
			savedBindings[key] = slot
		}
	}
	for i, bindExpr := range e.values {
		if !c.compileExpr(bindExpr, false) {
			return false
		}
		// Allocate the let slot after compiling the value expression: the
		// value may capture an outer binding, which grows c.numSlots. Using
		// a stale baseSlot would collide with those capture slots and make
		// otherwise valid loops non-compilable.
		slot := c.numSlots
		c.numSlots++
		c.bindingMap[bindingKey{frame: letFrame, index: i}] = slot
		c.emitWithOperand(irStoreSlot, slot)
	}
	for i, bodyExpr := range e.body {
		if !c.compileExpr(bodyExpr, isLast && i == len(e.body)-1) {
			return false
		}
	}
	// Restore outer scope bindings for this frame.
	// First, delete all current frame bindings, then restore saved ones.
	for key := range c.bindingMap {
		if key.frame == letFrame {
			delete(c.bindingMap, key)
		}
	}
	for key, slot := range savedBindings {
		c.bindingMap[key] = slot
	}
	return true
}

func (c *irCompiler) compileNestedLoop(loop *LoopExpr, isLast bool) bool {
	loopLet := (*LetExpr)(loop)
	baseSlot := -1

	loopFrame := -1
	for _, bodyExpr := range loopLet.body {
		if f := findBindingFrame(bodyExpr); f > c.loopFrame {
			loopFrame = f
			break
		}
	}
	if loopFrame < 0 {
		loopFrame = c.loopFrame + c.depth
	}

	// Save existing bindings for this frame to restore after scope exits.
	savedBindings := make(map[bindingKey]int)
	for key, slot := range c.bindingMap {
		if key.frame == loopFrame {
			savedBindings[key] = slot
		}
	}

	for i, bindExpr := range loopLet.values {
		if !c.compileExpr(bindExpr, false) {
			return false
		}
		// As with let, init expressions may capture outer bindings and grow
		// c.numSlots. Allocate loop slots after each init is compiled so the
		// nested loop's contiguous recur target never collides with captures.
		slot := c.numSlots
		if i == 0 {
			baseSlot = slot
		}
		c.numSlots++
		c.bindingMap[bindingKey{frame: loopFrame, index: i}] = slot
		c.emitWithOperand(irStoreSlot, slot)
	}
	if baseSlot < 0 {
		return false
	}

	loopStartPC := len(c.code)
	c.recurTargets = append(c.recurTargets, recurTarget{
		pc:       loopStartPC,
		baseSlot: baseSlot,
		nBinds:   len(loopLet.names),
	})

	for i, expr := range loopLet.body {
		if !c.compileExpr(expr, isLast && i == len(loopLet.body)-1) {
			c.recurTargets = c.recurTargets[:len(c.recurTargets)-1]
			return false
		}
	}

	c.recurTargets = c.recurTargets[:len(c.recurTargets)-1]
	// Restore outer scope bindings for this frame.
	for key := range c.bindingMap {
		if key.frame == loopFrame {
			delete(c.bindingMap, key)
		}
	}
	for key, slot := range savedBindings {
		c.bindingMap[key] = slot
	}
	return true
}

func exprHasTextLiteralOrStr(expr Expr) bool {
	switch e := expr.(type) {
	case *LiteralExpr:
		switch e.obj.(type) {
		case coretypes.String, coretypes.Char:
			return true
		}
	case *IfExpr:
		return exprHasTextLiteralOrStr(e.cond) || exprHasTextLiteralOrStr(e.positive) || exprHasTextLiteralOrStr(e.negative)
	case *LetExpr:
		for _, v := range e.values {
			if exprHasTextLiteralOrStr(v) {
				return true
			}
		}
		for _, b := range e.body {
			if exprHasTextLiteralOrStr(b) {
				return true
			}
		}
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok && coreVarToProcName(vref.vr) == "procStr" {
			return true
		}
		if exprHasTextLiteralOrStr(e.callable) {
			return true
		}
		for _, a := range e.args {
			if exprHasTextLiteralOrStr(a) {
				return true
			}
		}
	case *RecurExpr:
		for _, a := range e.args {
			if exprHasTextLiteralOrStr(a) {
				return true
			}
		}
	}
	return false
}

func exprHasCollectionOp(expr Expr) bool {
	switch e := expr.(type) {
	case *IfExpr:
		return exprHasCollectionOp(e.cond) || exprHasCollectionOp(e.positive) || exprHasCollectionOp(e.negative)
	case *LetExpr:
		for _, v := range e.values {
			if exprHasCollectionOp(v) {
				return true
			}
		}
		for _, b := range e.body {
			if exprHasCollectionOp(b) {
				return true
			}
		}
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok {
			switch coreVarToProcName(vref.vr) {
			case "procNth", "procGet", "procAssoc", "procConj", "procCount", "procFirst":
				return true
			}
		} else {
			// Calls through local helpers are not considered straight-line.
			return false
		}
		for _, a := range e.args {
			if exprHasCollectionOp(a) {
				return true
			}
		}
	case *RecurExpr:
		for _, a := range e.args {
			if exprHasCollectionOp(a) {
				return true
			}
		}
	}
	return false
}

func exprIsPureArithmetic(expr Expr) bool {
	switch e := expr.(type) {
	case *LiteralExpr:
		switch e.obj.(type) {
		case coretypes.Int, coretypes.Double:
			return true
		default:
			return false
		}
	case *BindingExpr:
		return true
	case *IfExpr:
		return exprIsPureArithmetic(e.cond) && exprIsPureArithmetic(e.positive) && exprIsPureArithmetic(e.negative)
	case *CallExpr:
		if vref, ok := e.callable.(*VarRefExpr); ok {
			switch coreVarToProcName(vref.vr) {
			case "procAdd", "procSubtract", "procMultiply", "procDivide",
				"procInc", "procDec", "procRem", "procQuot",
				"procLt", "procGt", "procLte", "procGte", "procEq",
				"procAbs", "procMax", "procMin":
			default:
				return false
			}
		} else {
			return false
		}
		for _, a := range e.args {
			if !exprIsPureArithmetic(a) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func exprIsStraightLine(expr Expr) bool {
	switch e := expr.(type) {
	case *LoopExpr, *RecurExpr:
		return false
	case *LetExpr:
		for _, v := range e.values {
			if !exprIsStraightLine(v) {
				return false
			}
		}
		for _, b := range e.body {
			if !exprIsStraightLine(b) {
				return false
			}
		}
	case *IfExpr:
		return exprIsStraightLine(e.cond) && exprIsStraightLine(e.positive) && exprIsStraightLine(e.negative)
	case *CallExpr:
		if _, ok := e.callable.(*VarRefExpr); !ok {
			return false
		}
		for _, a := range e.args {
			if !exprIsStraightLine(a) {
				return false
			}
		}
	}
	return true
}

func exprCount(expr Expr) int {
	switch e := expr.(type) {
	case *IfExpr:
		return 1 + exprCount(e.cond) + exprCount(e.positive) + exprCount(e.negative)
	case *LetExpr:
		n := 1
		for _, v := range e.values {
			n += exprCount(v)
		}
		for _, b := range e.body {
			n += exprCount(b)
		}
		return n
	case *CallExpr:
		n := 1 + exprCount(e.callable)
		for _, a := range e.args {
			n += exprCount(a)
		}
		return n
	case *RecurExpr:
		n := 1
		for _, a := range e.args {
			n += exprCount(a)
		}
		return n
	default:
		return 1
	}
}

func (c *irCompiler) compileTryCatch(e *TryExpr, isLast bool) bool {
	// Only support single catch with no finally for now
	if len(e.catches) != 1 || len(e.finallyExpr) > 0 {
		return c.reject("IR try/catch: only single catch without finally supported")
	}
	catch := e.catches[0]

	// Emit irTryCatch with placeholder for catchPC
	catchPCIdx := len(c.code) + 1 // position where catchPC will be
	bindSlot := c.numSlots
	c.numSlots++
	c.code = append(c.code, irTryCatch, 0, 0, byte(bindSlot>>8), byte(bindSlot))

	// Compile try body
	for i, bodyExpr := range e.body {
		if !c.compileExpr(bodyExpr, isLast && i == len(e.body)-1) {
			return false
		}
	}
	if !isLast {
		// Jump over catch body
		jumpIdx := len(c.code) + 1
		c.code = append(c.code, irJump, 0, 0)
		// Patch catchPC to here
		catchPC := len(c.code)
		c.code[catchPCIdx] = byte(catchPC >> 8)
		c.code[catchPCIdx+1] = byte(catchPC)

		// Set up catch binding
		catchFrame := c.loopFrame + c.depth + 1
		c.bindingMap[bindingKey{frame: catchFrame, index: 0}] = bindSlot
		_ = catch.excSymbol

		// Compile catch body
		for i, bodyExpr := range catch.body {
			if !c.compileExpr(bodyExpr, isLast && i == len(catch.body)-1) {
				return false
			}
		}
		// Patch jump target to after catch
		afterCatch := len(c.code)
		c.code[jumpIdx] = byte(afterCatch >> 8)
		c.code[jumpIdx+1] = byte(afterCatch)
	} else {
		// isLast: try body already has irReturn
		// Patch catchPC to here for the catch handler
		catchPC := len(c.code)
		c.code[catchPCIdx] = byte(catchPC >> 8)
		c.code[catchPCIdx+1] = byte(catchPC)

		catchFrame := c.loopFrame + c.depth + 1
		c.bindingMap[bindingKey{frame: catchFrame, index: 0}] = bindSlot

		for i, bodyExpr := range catch.body {
			if !c.compileExpr(bodyExpr, i == len(catch.body)-1) {
				return false
			}
		}
	}
	return true
}

// ---- loop_frame_detect.go ----
// ir_frame_detect.go — precise frame detection for let/loop bindings.
//
// The IR compiler needs to know which parse-time frame each let/loop
// binding belongs to. Instead of guessing via heuristics, this scans
// the body for binding references and deduces the frame from the
// binding indices.

// findLetFrame determines the parse-time frame for a let expression's
// bindings. It scans the body for BindingExpr nodes with indices 0..nBinds-1
// that reference a frame not already known in the compiler's bindingMap.
func findLetFrame(body []Expr, nBinds int, known map[bindingKey]int) int {
	if nBinds == 0 {
		return -1
	}
	// Collect candidate frames: frames where index 0 appears and is NOT in known
	candidates := map[int]int{} // frame -> count of matching indices
	var scan func(e Expr)
	scan = func(e Expr) {
		switch x := e.(type) {
		case *BindingExpr:
			f, idx := x.binding.frame, x.binding.index
			if idx < nBinds {
				if _, alreadyKnown := known[bindingKey{frame: f, index: idx}]; !alreadyKnown {
					candidates[f]++
				}
			}
		case *IfExpr:
			scan(x.cond)
			scan(x.positive)
			scan(x.negative)
		case *CallExpr:
			scan(x.callable)
			for _, a := range x.args {
				scan(a)
			}
		case *RecurExpr:
			for _, a := range x.args {
				scan(a)
			}
		case *LetExpr:
			for _, v := range x.values {
				scan(v)
			}
			for _, b := range x.body {
				scan(b)
			}
		case *LoopExpr:
			le := (*LetExpr)(x)
			for _, v := range le.values {
				scan(v)
			}
			for _, b := range le.body {
				scan(b)
			}
		}
	}
	for _, e := range body {
		scan(e)
	}

	// Pick the candidate frame where count matches nBinds exactly
	// (the let's own frame should have exactly nBinds distinct indices)
	bestFrame := -1
	for f, count := range candidates {
		if count == nBinds {
			if bestFrame < 0 || f < bestFrame {
				bestFrame = f
			}
		}
	}
	// Fallback: pick the smallest frame with any matches
	if bestFrame < 0 {
		for f := range candidates {
			if bestFrame < 0 || f < bestFrame {
				bestFrame = f
			}
		}
	}
	return bestFrame
}

// ---- loop_native_helpers.go ----
// ir_native_helper.go — compile pure arithmetic helpers to Go closures.
//
// When a loop calls a pure arithmetic helper via irCallSlot, this path
// compiles the helper's IR to a native Go function that operates on
// float64 values directly, eliminating WASM/IR dispatch and coretypes.Object boxing.

// nativeF64Fn is a compiled Go closure for a pure arithmetic helper.
type nativeF64Fn func(args []float64) float64

// nativeF64Fn2 is a 2-argument specialization that avoids slice allocation.
type nativeF64Fn2 func(a, b float64) float64

// irCompileNativeHelper attempts to compile an IR program (helper function)
// to a native Go float64 closure.
func irCompileNativeHelper(prog *IRProgram) nativeF64Fn {
	if prog == nil || prog.hasSelf {
		return nil
	}
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	// Only compile pure numeric programs (no collections, strings, calls)
	code := model.Code
	for pc := 0; pc < len(code); {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irDiv, irRem, irInc, irDec,
			irLt, irGte, irGt, irLte, irEq, irIsZero, irReturn, irSqrt:
			// ok
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				return nil
			}
		default:
			return nil
		}
	}

	// Build constants as float64
	consts := make([]float64, len(prog.constants))
	for i, c := range prog.constants {
		switch v := c.(type) {
		case coretypes.Int:
			consts[i] = float64(v.I)
		case coretypes.Double:
			consts[i] = v.D
		default:
			return nil
		}
	}

	numSlots := model.NumSlots
	codeSlice := model.Code

	return func(args []float64) float64 {
		var slotBuf [8]float64
		var slots []float64
		if numSlots <= len(slotBuf) {
			slots = slotBuf[:numSlots]
		} else {
			slots = make([]float64, numSlots)
		}
		copy(slots, args)

		var stack [16]float64
		sp := 0
		pc := 0

		for pc < len(codeSlice) {
			op := codeSlice[pc]
			pc++
			switch op {
			case irLiteral:
				idx := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				stack[sp] = consts[idx]
				sp++
			case irLoadSlot:
				idx := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				stack[sp] = slots[idx]
				sp++
			case irStoreSlot:
				idx := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				sp--
				slots[idx] = stack[sp]
			case irAdd:
				sp--
				stack[sp-1] += stack[sp]
			case irSub:
				sp--
				stack[sp-1] -= stack[sp]
			case irMul:
				sp--
				stack[sp-1] *= stack[sp]
			case irDiv:
				sp--
				stack[sp-1] /= stack[sp]
			case irSqrt:
				stack[sp-1] = math.Sqrt(stack[sp-1])
			case irRem:
				sp--
				b := int(stack[sp])
				if b != 0 {
					stack[sp-1] = float64(int(stack[sp-1]) % b)
				}
			case irInc:
				stack[sp-1]++
			case irDec:
				stack[sp-1]--
			case irLt:
				sp--
				if stack[sp-1] < stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irGte:
				sp--
				if stack[sp-1] >= stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irGt:
				sp--
				if stack[sp-1] > stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irLte:
				sp--
				if stack[sp-1] <= stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irEq:
				sp--
				if stack[sp-1] == stack[sp] {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irIsZero:
				if stack[sp-1] == 0 {
					stack[sp-1] = 1.0
				} else {
					stack[sp-1] = 0.0
				}
			case irJumpIfNot:
				target := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				sp--
				if stack[sp] == 0 {
					pc = target
				}
			case irJump:
				pc = int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
			case irRecur:
				nargs := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				target := int(codeSlice[pc])<<8 | int(codeSlice[pc+1])
				pc += 2
				for i := nargs - 1; i >= 0; i-- {
					sp--
					slots[i] = stack[sp]
				}
				pc = target
			case irReturn:
				sp--
				return stack[sp]
			default:
				return 0
			}
		}
		if sp > 0 {
			return stack[sp-1]
		}
		return 0
	}
}

// ---- loop_wasm_diagnostics.go ----
// ir_diagnostics.go — lightweight IR/WASM compilation explanations.
//
// These helpers are intentionally internal: they give benchmark and regression
// tests a stable way to answer "which execution path did this hot form take?"
// without changing Joker's public language surface. The goal is to make future
// core-runtime speed work measurable instead of guess-driven.

type IRDiagnostic struct {
	Compiled    bool
	Reason      string
	BodyIndex   int
	NumSlots    int
	NumCaptures int
	NumOps      int
	UsesWASM    bool
	WASM        corewasm.Diagnostic
	Analysis    coreir.Analysis
}

func explainIRCompile(loop *LoopExpr) IRDiagnostic {
	if loop == nil {
		return IRDiagnostic{Reason: "nil loop"}
	}
	prog, reason := irCompileExplain(loop)
	if prog == nil {
		if reason == "" {
			reason = "IR compiler rejected loop body (unsupported form or unsafe binding shape)"
		}
		return IRDiagnostic{Reason: reason}
	}
	wasm := explainWASMEligibility(prog)
	analysis := AnalyzeIRProgram(prog)
	model := prog.neutralModel()
	return IRDiagnostic{
		Compiled:    true,
		NumSlots:    model.NumSlots,
		NumCaptures: len(prog.captureKeys),
		NumOps:      coreir.OpCount(model.Code),
		UsesWASM:    wasm.Eligible && !wasm.HasImports,
		WASM:        wasm,
		Analysis:    analysis,
	}
}

func explainWASMEligibility(prog *IRProgram) corewasm.Diagnostic {
	if prog == nil {
		return corewasm.Diagnostic{Reason: "nil IR program"}
	}
	model := prog.neutralModel()
	if model == nil {
		return corewasm.Diagnostic{Reason: "nil IR program model"}
	}
	return corewasm.ExplainEligibility(model.Code, len(model.FloatConsts) > 0)
}

func findFirstLoopExpr(expr Expr) *LoopExpr {
	switch e := expr.(type) {
	case *LoopExpr:
		return e
	case *LetExpr:
		for _, v := range e.values {
			if loop := findFirstLoopExpr(v); loop != nil {
				return loop
			}
		}
		for _, b := range e.body {
			if loop := findFirstLoopExpr(b); loop != nil {
				return loop
			}
		}
	case *IfExpr:
		if loop := findFirstLoopExpr(e.cond); loop != nil {
			return loop
		}
		if loop := findFirstLoopExpr(e.positive); loop != nil {
			return loop
		}
		return findFirstLoopExpr(e.negative)
	case *CallExpr:
		if loop := findFirstLoopExpr(e.callable); loop != nil {
			return loop
		}
		for _, a := range e.args {
			if loop := findFirstLoopExpr(a); loop != nil {
				return loop
			}
		}
	case *RecurExpr:
		for _, a := range e.args {
			if loop := findFirstLoopExpr(a); loop != nil {
				return loop
			}
		}
	}
	return nil
}

func explainFirstLoop(expr Expr) IRDiagnostic {
	loop := findFirstLoopExpr(expr)
	if loop == nil {
		return IRDiagnostic{Reason: "no loop expression found"}
	}
	return explainIRCompile(loop)
}

// ---- inline_rewrites.go ----
func (c *irCompiler) tryInlineCall(fnSlot int, expr *CallExpr, isLast bool) bool {
	_ = fnSlot
	if corert.IRInlineDisabled() {
		return false
	}
	fnExpr := findFnExprForBinding(expr.callable)
	if fnExpr == nil || len(fnExpr.arities) != 1 || fnExpr.variadic != nil {
		return false
	}
	arity := fnExpr.arities[0]
	if !corert.IRInlineForce() {
		inlineOK := false
		for _, b := range arity.body {
			if exprHasTextLiteralOrStr(b) {
				inlineOK = true
				break
			}
			if exprIsStraightLine(b) {
				if exprHasCollectionOp(b) && exprCount(b) <= 16 {
					inlineOK = true
					break
				}
				// Inline pure arithmetic helpers (≤32 exprs) only when the
				// caller loop has no collection ops.
				if exprIsPureArithmetic(b) && exprCount(b) <= 32 && !c.hasCollectionOps {
					inlineOK = true
					break
				}
			}
		}
		if !inlineOK {
			return false
		}
	}
	if len(arity.args) != len(expr.args) || len(arity.body) != 1 {
		return false
	}
	fnFrame := guessLoopFrame(arity.body)
	if fnFrame < 0 {
		return false
	}
	// Use a synthetic frame to avoid collision with the caller's loop frame.
	// The fn's parameters may share the same (frame, index) as the caller's
	// loop bindings. By remapping to a unique frame, inline temps don't
	// overwrite caller slots.
	inlineFrame := fnFrame + 1000
	for _, arg := range expr.args {
		if !c.compileExpr(arg, false) {
			return false
		}
	}
	baseSlot := c.numSlots
	oldBindings := make(map[bindingKey]int, len(arity.args))
	oldPresent := make(map[bindingKey]bool, len(arity.args))
	for i := len(arity.args) - 1; i >= 0; i-- {
		slot := baseSlot + i
		key := bindingKey{frame: inlineFrame, index: i}
		if old, ok := c.bindingMap[key]; ok {
			oldBindings[key] = old
			oldPresent[key] = true
		}
		c.bindingMap[key] = slot
		c.emitWithOperand(irStoreSlot, slot)
	}
	// Also remap the original fnFrame bindings so body references resolve
	origKeys := make([]bindingKey, len(arity.args))
	origOld := make(map[bindingKey]int)
	origPresent := make(map[bindingKey]bool)
	for i := range arity.args {
		origKey := bindingKey{frame: fnFrame, index: i}
		origKeys[i] = origKey
		if old, ok := c.bindingMap[origKey]; ok {
			origOld[origKey] = old
			origPresent[origKey] = true
		}
		c.bindingMap[origKey] = baseSlot + i
	}
	c.numSlots = baseSlot + len(arity.args)
	// The inlined body may contain let/or expansions at frames that
	// collide with the caller's loop bindings. To avoid findLetFrame
	// skipping those frames ("already known"), temporarily clear
	// caller bindings at the inlined body's internal let frames.
	inlineLetFrames := collectLetFrames(arity.body[0], fnFrame)
	savedInlineFrames := make(map[bindingKey]int)
	for k, v := range c.bindingMap {
		for _, lf := range inlineLetFrames {
			if k.frame == lf {
				savedInlineFrames[k] = v
			}
		}
	}
	for k := range savedInlineFrames {
		delete(c.bindingMap, k)
	}
	ok := c.compileExpr(arity.body[0], isLast)
	for k, v := range savedInlineFrames {
		c.bindingMap[k] = v
	}
	for i := range arity.args {
		key := bindingKey{frame: inlineFrame, index: i}
		if oldPresent[key] {
			c.bindingMap[key] = oldBindings[key]
		} else {
			delete(c.bindingMap, key)
		}
		origKey := origKeys[i]
		if origPresent[origKey] {
			c.bindingMap[origKey] = origOld[origKey]
		} else {
			delete(c.bindingMap, origKey)
		}
	}
	return ok
}

// findFnExprForBinding tries to find the FnExpr for a callable binding.
func findFnExprForBinding(callable Expr) *FnExpr {
	bindExpr, ok := callable.(*BindingExpr)
	if !ok {
		return nil
	}
	if bindExpr.binding.value == nil {
		return nil
	}
	if fnExpr, ok := bindExpr.binding.value.(*FnExpr); ok {
		return fnExpr
	}
	return nil
}

func (c *irCompiler) compileCall(expr *CallExpr, isLast bool) bool {
	// Check if callable is a binding (local/captured function)
	if bindExpr, ok := expr.callable.(*BindingExpr); ok {
		// Check for self-recursive call
		if c.hasSelf && bindExpr.binding.frame < c.loopFrame && len(expr.args) == c.selfNArgs {
			for _, arg := range expr.args {
				if !c.compileExpr(arg, false) {
					return false
				}
			}
			c.emitWithOperand(irCallSelf, len(expr.args))
			if isLast {
				c.emit(irReturn)
			}
			return true
		}

		key := bindingKey{frame: bindExpr.binding.frame, index: bindExpr.binding.index}
		slot, ok := c.bindingMap[key]
		if !ok {
			if bindExpr.binding.frame < c.loopFrame {
				slot = c.numSlots
				c.bindingMap[key] = slot
				c.captureKeys = append(c.captureKeys, key)
				c.numSlots++
			} else {
				return c.reject("callable binding frame %d index %d is not capturable from loop frame %d", bindExpr.binding.frame, bindExpr.binding.index, c.loopFrame)
			}
		}

		// Try to inline the function call
		if c.tryInlineCall(slot, expr, isLast) {
			return true
		}

		for _, arg := range expr.args {
			if !c.compileExpr(arg, false) {
				return false
			}
		}
		c.code = append(c.code, irCallSlot,
			byte(slot>>8), byte(slot),
			byte(len(expr.args)>>8), byte(len(expr.args)))
		if isLast {
			c.emit(irReturn)
		}
		return true
	}

	vref, ok := expr.callable.(*VarRefExpr)
	if !ok {
		return c.reject("unsupported callable expression type %T", expr.callable)
	}

	// Check for var-based self-recursive call (defn fib [...] (fib ...))
	if c.hasSelf && c.selfVar != nil && vref.vr == c.selfVar && len(expr.args) == c.selfNArgs {
		for _, arg := range expr.args {
			if !c.compileExpr(arg, false) {
				return false
			}
		}
		c.emitWithOperand(irCallSelf, len(expr.args))
		if isLast {
			c.emit(irReturn)
		}
		return true
	}

	procName := ""
	switch v := vref.vr.Value.(type) {
	case Proc:
		procName = v.Name
	case *Fn:
		procName = coreVarToProcName(vref.vr)
	}
	if procName == "" {
		return c.reject("unsupported callable var %s", vref.vr.name.ToString(false))
	}

	switch procName {
	case "procAdd":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irAdd)
	case "procSubtract":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irSub)
	case "procMultiply":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irMul)
	case "procRem":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irRem)
	case "procDivide":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irDiv)
	case "procInc":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irInc)
	case "procDec":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irDec)
	case "procLt":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irLt)
	case "procGte":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irGte)
	case "procGt":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irGt)
	case "procLte":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irLte)
	case "procEq":
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irEq)
	case "procInt":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg", procName)
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irIntCast)
	case "procSubs":
		if len(expr.args) < 2 || len(expr.args) > 3 {
			return c.reject("%s expects 2-3 args", procName)
		}
		for _, a := range expr.args {
			if !c.compileExpr(a, false) {
				return false
			}
		}
		// Encode arg count in the opcode operand
		c.emitWithOperand(irSubs, len(expr.args))
	case "procIsZero":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irIsZero)
	case "procGet":
		c.hasCollectionOps = true
		if len(expr.args) == 2 {
			if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
				return false
			}
			c.emit(irGet)
		} else if len(expr.args) == 3 {
			if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) || !c.compileExpr(expr.args[2], false) {
				return false
			}
			c.emit(irGet3)
		} else {
			return c.reject("%s expects 2 or 3 args, got %d", procName, len(expr.args))
		}
	case "procAssoc":
		c.hasCollectionOps = true
		if len(expr.args) != 3 {
			return c.reject("%s expects 3 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) || !c.compileExpr(expr.args[2], false) {
			return false
		}
		c.emit(irAssoc)
	case "procNth":
		c.hasCollectionOps = true
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if s, ok := c.constantASCIIString(expr.args[0]); ok {
			if !c.compileExpr(expr.args[1], false) {
				return false
			}
			idx := c.addConstant(coretypes.String{S: s})
			c.emitWithOperand(irNthStringASCII, idx)
		} else {
			if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
				return false
			}
			c.emit(irNth)
		}
	case "procConj":
		c.hasCollectionOps = true
		if len(expr.args) != 2 {
			return c.reject("%s expects 2 args, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irConj)
	case "procSqrt":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irSqrt)
	case "procFirst":
		c.hasCollectionOps = true
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irFirst)
	case "procCursorChar":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irCursorChar)
	case "procCursorNext":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irCursorNext)
	case "procCursorDone":
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irCursorDone)
	case "procStr":
		if len(expr.args) == 1 {
			if !c.compileExpr(expr.args[0], false) {
				return false
			}
			c.emit(irStr1)
		} else if len(expr.args) == 2 {
			if !c.compileExpr(expr.args[0], false) || !c.compileExpr(expr.args[1], false) {
				return false
			}
			c.emit(irStr2)
		} else {
			return c.reject("%s expects 1 or 2 args, got %d", procName, len(expr.args))
		}
	case "procCount":
		c.hasCollectionOps = true
		if len(expr.args) != 1 {
			return c.reject("%s expects 1 arg, got %d", procName, len(expr.args))
		}
		if n, ok := c.constantCount(expr.args[0]); ok {
			idx := c.addConstant(coretypes.Int{I: n})
			c.emitWithOperand(irLiteral, idx)
		} else {
			if !c.compileExpr(expr.args[0], false) {
				return false
			}
			c.emit(irCount)
		}
	case "procBitAnd":
		if len(expr.args) != 2 {
			return c.reject("bit-and expects 2 args")
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		if !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irBitAnd)
	case "procBitOr":
		if len(expr.args) != 2 {
			return c.reject("bit-or expects 2 args")
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		if !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irBitOr)
	case "procBitNot":
		if len(expr.args) != 1 {
			return c.reject("bit-not expects 1 arg")
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irBitNot)
	case "procBitShiftLeft":
		if len(expr.args) != 2 {
			return c.reject("bit-shift-left expects 2 args")
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		if !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irBitShiftLeft)
	case "procBitShiftRight":
		if len(expr.args) != 2 {
			return c.reject("bit-shift-right expects 2 args")
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		if !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irBitShiftRight)
	case "procApply":
		if len(expr.args) != 2 {
			return c.reject("apply expects 2 args (fn + args), got %d", len(expr.args))
		}
		// Compile fn and args-seq onto stack, then irApply
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		if !c.compileExpr(expr.args[1], false) {
			return false
		}
		c.emit(irApply)
	case "procThrow":
		if len(expr.args) != 1 {
			return c.reject("throw expects 1 arg, got %d", len(expr.args))
		}
		if !c.compileExpr(expr.args[0], false) {
			return false
		}
		c.emit(irThrow)
	default:
		return c.reject("unsupported core proc for IR: %s", procName)
	}
	if isLast {
		c.emit(irReturn)
	}
	return true
}

// coreVarToProcName maps well-known core Vars to internal proc names.
func coreVarToProcName(vr *Var) string {
	if vr.ns == nil || vr.ns != GLOBAL_ENV.CoreNamespace {
		return ""
	}
	switch vr.name.ToString(false) {
	case "+":
		return "procAdd"
	case "-":
		return "procSubtract"
	case "*":
		return "procMultiply"
	case "rem":
		return "procRem"
	case "inc":
		return "procInc"
	case "dec":
		return "procDec"
	case "<":
		return "procLt"
	case "<=":
		return "procLte"
	case ">":
		return "procGt"
	case ">=":
		return "procGte"
	case "=":
		return "procEq"
	case "zero?":
		return "procIsZero"
	case "/":
		return "procDivide"
	case "get":
		return "procGet"
	case "assoc":
		return "procAssoc"
	case "conj":
		return "procConj"
	case "sqrt":
		return "procSqrt"
	case "first":
		return "procFirst"
	case "str":
		return "procStr"
	case "count":
		return "procCount"
	case "nth":
		return "procNth"
	case "int":
		return "procInt"
	case "subs":
		return "procSubs"
	default:
		return ""
	}
}

// collectLetFrames finds all frames used by LetExpr nodes inside an expression
// that are deeper than fnFrame (i.e., internal to the inlined fn body).
func collectLetFrames(expr Expr, fnFrame int) []int {
	var frames []int
	seen := map[int]bool{}
	var scan func(e Expr)
	scan = func(e Expr) {
		switch x := e.(type) {
		case *LetExpr:
			// Check what frame this let's bindings use
			for _, b := range x.body {
				scanBindings(b, len(x.values), fnFrame, seen, &frames)
			}
			for _, v := range x.values {
				scan(v)
			}
			for _, b := range x.body {
				scan(b)
			}
		case *IfExpr:
			scan(x.cond)
			scan(x.positive)
			scan(x.negative)
		case *CallExpr:
			scan(x.callable)
			for _, a := range x.args {
				scan(a)
			}
		}
	}
	scan(expr)
	return frames
}

func scanBindings(expr Expr, nBinds int, fnFrame int, seen map[int]bool, frames *[]int) {
	switch x := expr.(type) {
	case *BindingExpr:
		f := x.binding.frame
		if f > fnFrame && x.binding.index < nBinds && !seen[f] {
			seen[f] = true
			*frames = append(*frames, f)
		}
	case *IfExpr:
		scanBindings(x.cond, nBinds, fnFrame, seen, frames)
		scanBindings(x.positive, nBinds, fnFrame, seen, frames)
		scanBindings(x.negative, nBinds, fnFrame, seen, frames)
	case *CallExpr:
		scanBindings(x.callable, nBinds, fnFrame, seen, frames)
		for _, a := range x.args {
			scanBindings(a, nBinds, fnFrame, seen, frames)
		}
	case *LetExpr:
		for _, v := range x.values {
			scanBindings(v, nBinds, fnFrame, seen, frames)
		}
		for _, b := range x.body {
			scanBindings(b, nBinds, fnFrame, seen, frames)
		}
	}
}

// ---- wasm_compile.go ----

// ---- wasm_compile.go ----
// wasm_codegen.go — translates IR bytecode to WASM function body.

var wasmFnCache sync.Map // map[*FnArityExpr]*WasmProgram

var wasmFnFail = &WasmProgram{}

// wasmGetFn retrieves or compiles a WASM program for a Fn.
func wasmGetFn(fn *Fn) *WasmProgram {
	if len(fn.fnExpr.arities) != 1 {
		return nil
	}
	arity := &fn.fnExpr.arities[0]

	if v, ok := wasmFnCache.Load(arity); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFnFail {
			return nil
		}
		return wp
	}

	// First compile to IR
	irProg := irCompileFn(fn)
	if irProg == nil {
		wasmFnCache.Store(arity, wasmFnFail)
		return nil
	}

	// Then try WASM
	wp := wasmCompile(irProg)
	if wp == nil {
		wasmFnCache.Store(arity, wasmFnFail)
		return nil
	}

	wasmFnCache.Store(arity, wp)
	return wp
}

func irToWasm(prog *IRProgram) []byte {
	model := prog.neutralModel()
	if model == nil || !corewasm.Eligible(model.Code) {
		return nil
	}
	useFloat := corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0)
	body := compileWasmBody(prog, useFloat)
	if body == nil {
		return nil
	}
	m := corewasm.NewModule()
	valType := corewasm.ValTypeI64
	if useFloat {
		valType = corewasm.ValTypeF64
	}
	m.AddTypeSectionTyped(model.NumSlots, valType)
	if prog.hasSelf {
		m.AddFuncSectionRecursive()
	} else {
		m.AddFuncSection()
	}
	m.AddExportSection()
	m.AddCodeSection(body)
	return m.Bytes()
}

// compileWasmBody generates WASM instructions.
//
// Layout:
//
//	block $exit (result i64)     ;; depth from inside if: +2
//	  loop $loop (void)          ;; depth from inside if: +1
//	    ;; body
//	    ;; irReturn → br $exit (depth = nesting + 1)
//	    ;; irRecur  → set locals, br $loop (depth = nesting)
//	  end
//	  i64.const 0  ;; unreachable
//	end
//
// For if/else: both branches end with `br` (stack-polymorphic),
// so `if void` works and no values need to flow through the if block.
func compileWasmBody(prog *IRProgram, useFloat bool) []byte {
	return compileWasmBodyWithHelper(prog, useFloat, -1, -1)
}

func compileWasmBodyWithHelper(prog *IRProgram, useFloat bool, helperSlot int, helperFuncIdx int) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	return compileWasmBodyWithHelperParams(prog, useFloat, helperSlot, helperFuncIdx, model.NumSlots)
}

// Emit checked i64 arithmetic. Overflow traps before a wrapped value can affect
// a comparison or loop state. Only import-free programs may recover in Go.
func wasmCheckedArithmetic(out []byte, op byte, temp int) []byte {
	local := func(code byte, idx int) { out = append(out, code); out = corewasm.AppendULEB(out, idx) }
	local(0x21, temp+1) // b
	local(0x21, temp)   // a
	local(0x20, temp)
	local(0x20, temp+1)
	out = append(out, op)
	local(0x21, temp+2)
	if op == 0x7e { // mul: a != 0 && product/a != b
		local(0x20, temp)
		out = append(out, 0x50, 0x45, 0x04, 0x40)
		local(0x20, temp+2)
		local(0x20, temp)
		out = append(out, 0x7f)
		local(0x20, temp+1)
		out = append(out, 0x52, 0x04, 0x40, 0x00, 0x0b, 0x0b)
	} else {
		local(0x20, temp)
		if op == 0x7c {
			local(0x20, temp+2)
		} else {
			local(0x20, temp+1)
		}
		out = append(out, 0x85) // xor
		if op == 0x7c {
			local(0x20, temp+1)
		} else {
			local(0x20, temp)
		}
		local(0x20, temp+2)
		out = append(out, 0x85, 0x83, 0x42, 0x00, 0x53, 0x04, 0x40, 0x00, 0x0b)
	}
	local(0x20, temp+2)
	return out
}

func compileWasmBodyWithHelperParams(prog *IRProgram, useFloat bool, helperSlot int, helperFuncIdx int, numParams int) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	var o []byte
	valType := corewasm.ValTypeI64
	if useFloat {
		valType = corewasm.ValTypeF64
	}
	extraLocals := model.NumSlots - numParams
	if !useFloat {
		extraLocals += 3
	}
	if extraLocals > 0 {
		o = append(o, 0x01) // 1 local decl group
		o = corewasm.AppendULEB(o, extraLocals)
		o = append(o, valType)
	} else {
		o = append(o, 0x00) // 0 local decls
	}

	resType := valType
	if useFloat {
		resType = corewasm.ValTypeF64
	}
	o = append(o, 0x02, resType) // block $exit -> result type

	code := model.Code

	// Find the loop start: scan for the first Recur opcode's target
	loopStartPC := 0
	{
		scan := 0
		for scan < len(code) {
			op := code[scan]
			scan++
			switch op {
			case irLiteral, irLoadSlot, irStoreSlot, irJumpIfNot, irJump, 29: // 2-byte operand ops
				scan += 2
			case irCallSlot:
				scan += 4
			case irCallSelf:
				scan += 2
			case irRecur:
				tgt := int(code[scan+2])<<8 | int(code[scan+3])
				if tgt != 0 {
					loopStartPC = tgt
				}
				scan = len(code) // done
			default:
				// single-byte ops
			}
		}
	}

	pc := 0
	depth := 0 // extra nesting from if blocks
	// Track where each if-block's else-branch ends (jump target of irJump)
	type ifEnd struct {
		endPC int
	}
	var ifEnds []ifEnd
	falseBranches := map[int]int{}
	loopEmitted := false

	for pc < len(code) {
		// Emit loop instruction at the loop start PC
		if !loopEmitted && pc >= loopStartPC {
			o = append(o, 0x03, 0x40) // loop $loop -> void
			loopEmitted = true
		}

		// Close any if-blocks that end at this PC
		for len(ifEnds) > 0 && ifEnds[len(ifEnds)-1].endPC == pc {
			o = append(o, 0x0b) // end if
			depth--
			ifEnds = ifEnds[:len(ifEnds)-1]
		}

		op := code[pc]
		pc++

		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			c := prog.constants[idx]
			if useFloat {
				var fv float64
				switch v := c.(type) {
				case coretypes.Int:
					fv = float64(v.I)
				case coretypes.Double:
					fv = v.D
				default:
					return nil
				}
				o = append(o, 0x44) // f64.const
				o = corewasm.AppendF64(o, fv)
			} else {
				v, ok := c.(coretypes.Int)
				if !ok {
					return nil
				}
				o = append(o, 0x42) // i64.const
				o = corewasm.AppendSLEB(o, int64(v.I))
			}

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x20)
			o = corewasm.AppendULEB(o, idx)

		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x21)
			o = corewasm.AppendULEB(o, idx)

		case irAdd:
			if useFloat {
				o = append(o, 0xa0)
			} else {
				o = wasmCheckedArithmetic(o, 0x7c, model.NumSlots)
			}
		case irSub:
			if useFloat {
				o = append(o, 0xa1)
			} else {
				o = wasmCheckedArithmetic(o, 0x7d, model.NumSlots)
			}
		case irMul:
			if useFloat {
				o = append(o, 0xa2)
			} else {
				o = wasmCheckedArithmetic(o, 0x7e, model.NumSlots)
			}
		case irDiv:
			if useFloat {
				o = append(o, 0xa3)
			} else {
				return nil
			}
		case irSqrt:
			if useFloat {
				o = append(o, 0x9f)
			} else {
				return nil
			}
		case irRem:
			if useFloat {
				return nil
			}
			o = append(o, 0x81)
		case irInc:
			if useFloat {
				o = append(o, 0x44)
				o = corewasm.AppendF64(o, 1.0)
				o = append(o, 0xa0)
			} else {
				o = append(o, 0x42, 0x01)
				o = wasmCheckedArithmetic(o, 0x7c, model.NumSlots)
			}
		case irDec:
			if useFloat {
				o = append(o, 0x44)
				o = corewasm.AppendF64(o, 1.0)
				o = append(o, 0xa1)
			} else {
				o = append(o, 0x42, 0x01)
				o = wasmCheckedArithmetic(o, 0x7d, model.NumSlots)
			}
		case irLt:
			if useFloat {
				o = append(o, 0x63) // f64.lt
			} else {
				o = append(o, 0x53, 0xad) // i64.lt_s, i64.extend_i32_s
			}
		case irGte:
			if useFloat {
				o = append(o, 0x65) // f64.ge
			} else {
				o = append(o, 0x56, 0xad) // i64.ge_s, i64.extend_i32_s
			}
		case irGt:
			if useFloat {
				o = append(o, 0x64) // f64.gt
			} else {
				o = append(o, 0x55, 0xad) // i64.gt_s, i64.extend_i32_s
			}
		case irLte:
			if useFloat {
				o = append(o, 0x66) // f64.le
			} else {
				o = append(o, 0x57, 0xad) // i64.le_s, i64.extend_i32_s
			}
		case irEq:
			if useFloat {
				o = append(o, 0x61)
			} else {
				o = append(o, 0x51, 0xad)
			}
		case irIsZero:
			if useFloat {
				o = append(o, 0x44)
				o = corewasm.AppendF64(o, 0.0)
				o = append(o, 0x61)
			} else {
				o = append(o, 0x50, 0xad)
			}

		case irJumpIfNot:
			jumpTarget := int(code[pc])<<8 | int(code[pc+1])
			falseBranches[depth+1] = jumpTarget
			pc += 2
			if !useFloat {
				o = append(o, 0xa7) // i32.wrap_i64
			}
			// Determine if this if-block produces a value:
			// Look for a Jump (else) whose target has StoreSlot next,
			// meaning both branches leave a value on stack.
			isValueIf := false
			// Scan true branch for Jump opcode
		scanTrueBranch:
			for scan := pc; scan < jumpTarget && scan < len(code); {
				scanOp := code[scan]
				if scanOp == irJump {
					jmpTgt := int(code[scan+1])<<8 | int(code[scan+2])
					if jmpTgt < len(code) && code[jmpTgt] == irStoreSlot {
						isValueIf = true
					}
					break
				}
				// advance scan past operands
				switch scanOp {
				case irLiteral, irLoadSlot, irStoreSlot, irJumpIfNot, irJump, 29: // NthStringASCII
					scan += 3
				case irCallSlot:
					scan += 5
				case irRecur:
					break scanTrueBranch
				case irCallSelf:
					scan += 3
				default:
					scan++
				}
			}
			if isValueIf {
				o = append(o, 0x04, valType) // if (result valType)
			} else {
				o = append(o, 0x04, 0x40) // if void
			}
			depth++

		case irJump:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x05) // else
			// If jump target is before end of code, this if-block is a value
			// expression (not tail control flow). Record where to close it.
			if target < len(code) {
				ifEnds = append(ifEnds, ifEnd{endPC: target})
			}

		case irReturn:
			// Value on stack → br to $exit
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth+1)
			// If we're inside an if and no explicit else follows,
			// emit else so the false branch code has somewhere to go.
			if depth > 0 && pc < len(code) && code[pc] != irJump {
				o = append(o, 0x05)
			}

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			tgt := int(code[pc+2])<<8 | int(code[pc+3])
			pc += 4
			if tgt != 0 {
				baseSlot := int(code[pc])<<8 | int(code[pc+1])
				pc += 2
				for i := nargs - 1; i >= 0; i-- {
					o = append(o, 0x21)
					o = corewasm.AppendULEB(o, baseSlot+i)
				}
			} else {
				for i := nargs - 1; i >= 0; i-- {
					o = append(o, 0x21)
					o = corewasm.AppendULEB(o, i)
				}
			}
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth)
			if pc < len(code) && depth > 0 && falseBranches[depth] == pc {
				// Preserve the false branch containing the loop exit.
				o = append(o, 0x05) // else
			} else {
				pc = len(code)
			}

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			_ = nargs // args already on WASM stack
			if slotIdx != helperSlot || helperFuncIdx < 0 {
				return nil
			}
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, helperFuncIdx)

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			_ = nargs                     // args already on WASM stack
			o = append(o, 0x10)           // call
			o = corewasm.AppendULEB(o, 0) // function index 0 (self)

		default:
			return nil
		}
	}

	// Close any open if blocks
	for depth > 0 {
		o = append(o, 0x0b)
		depth--
	}

	o = append(o, 0x0b) // end loop
	if useFloat {
		o = append(o, 0x44) // f64.const 0.0
		o = corewasm.AppendF64(o, 0.0)
	} else {
		o = append(o, 0x42, 0x00) // i64.const 0
	}
	o = append(o, 0x0b) // end block
	o = append(o, 0x0b) // end func
	return o
}

// ---- wasm_compile_host.go ----
// wasm_codegen_host.go — WASM codegen with host function imports.
//
// Extends the base codegen to emit modules that import the "joker"
// host module functions for collection operations. Programs with
// collection IR opcodes (irGet, irGet3, irAssoc, irNth, irConj, etc.)
// use this path instead of the pure-numeric codegen.

// standardHostImports lists the host functions in fixed order.
// Their indices in the WASM module are 0..len-1.
var standardHostImports = corewasm.StandardHostImports

// irToWasmWithImports compiles an IR program that uses collection ops
// to a WASM module with host function imports.
func irToWasmWithImports(prog *IRProgram) []byte {
	model := prog.neutralModel()
	if model == nil || !corewasm.EligibleWithImports(model.Code) {
		return nil
	}

	body := compileWasmBodyWithImports(prog)
	if body == nil {
		return nil
	}

	m := corewasm.NewModule()

	// Type section: one type per import + one for the main fn
	// All use i64 params and i64 result
	numTypes := len(standardHostImports) + 1
	var typeBody []byte
	typeBody = corewasm.AppendULEB(typeBody, numTypes)
	// Import function types (index 0..6)
	for _, imp := range standardHostImports {
		typeBody = append(typeBody, 0x60) // functype
		typeBody = corewasm.AppendULEB(typeBody, imp.NumParams)
		for j := 0; j < imp.NumParams; j++ {
			typeBody = append(typeBody, corewasm.ValTypeI64)
		}
		typeBody = append(typeBody, 0x01, corewasm.ValTypeI64)
	}
	// Main function type (index 7)
	typeBody = append(typeBody, 0x60)
	typeBody = corewasm.AppendULEB(typeBody, model.NumSlots)
	for i := 0; i < model.NumSlots; i++ {
		typeBody = append(typeBody, corewasm.ValTypeI64)
	}
	typeBody = append(typeBody, 0x01, corewasm.ValTypeI64)
	m.AddSection(0x01, typeBody)

	// Import section
	var importBody []byte
	importBody = corewasm.AppendULEB(importBody, len(standardHostImports))
	for i, imp := range standardHostImports {
		modName := []byte(wasmHostModuleName)
		importBody = corewasm.AppendULEB(importBody, len(modName))
		importBody = append(importBody, modName...)
		importBody = corewasm.AppendULEB(importBody, len(imp.Name))
		importBody = append(importBody, []byte(imp.Name)...)
		importBody = append(importBody, 0x00)           // import kind: func
		importBody = corewasm.AppendULEB(importBody, i) // type index
	}
	m.AddSection(0x02, importBody)

	// Function section: 1 function with type index = len(imports)
	mainTypeIdx := len(standardHostImports)
	var funcBody []byte
	funcBody = append(funcBody, 0x01)
	funcBody = corewasm.AppendULEB(funcBody, mainTypeIdx)
	m.AddSection(0x03, funcBody)

	// Export section: export the main function
	mainFuncIdx := len(standardHostImports) // imports are 0..6, main is 7
	name := []byte("exec")
	var exportBody []byte
	exportBody = append(exportBody, 0x01)
	exportBody = corewasm.AppendULEB(exportBody, len(name))
	exportBody = append(exportBody, name...)
	exportBody = append(exportBody, 0x00) // func export
	exportBody = corewasm.AppendULEB(exportBody, mainFuncIdx)
	m.AddSection(0x07, exportBody)

	// Code section
	m.AddCodeSection(body)

	return m.Bytes()
}

// compileWasmBodyWithImports generates function body with host call instructions.
func compileWasmBodyWithImports(prog *IRProgram) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	var o []byte
	o = append(o, 0x00) // 0 local decls

	o = append(o, 0x02, corewasm.ValTypeI64) // block $exit -> i64
	o = append(o, 0x03, 0x40)                // loop $loop -> void

	mainFuncIdx := len(standardHostImports)
	code := model.Code
	pc := 0
	depth := 0

	for pc < len(code) {
		op := code[pc]
		pc++

		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			c := prog.constants[idx]
			switch v := c.(type) {
			case coretypes.Int:
				o = append(o, 0x42)
				o = corewasm.AppendSLEB(o, int64(v.I))
			default:
				// Non-Int constant: use a pre-computed handle.
				// The handle value is: (1<<62) | constant_index
				// wasmExec will pre-populate the object table with these.
				handle := (int64(1) << 62) | int64(idx)
				o = append(o, 0x42)
				o = corewasm.AppendSLEB(o, handle)
			}

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x20)
			o = corewasm.AppendULEB(o, idx)

		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x21)
			o = corewasm.AppendULEB(o, idx)

		case irAdd:
			o = append(o, 0x7c)
		case irSub:
			o = append(o, 0x7d)
		case irMul:
			o = append(o, 0x7e)
		case irRem:
			o = append(o, 0x81)
		case irInc:
			o = append(o, 0x42, 0x01, 0x7c)
		case irDec:
			o = append(o, 0x42, 0x01, 0x7d)
		case irLt:
			o = append(o, 0x53, 0xad) // i64.lt_s, extend
		case irGte:
			o = append(o, 0x56, 0xad) // i64.ge_s, extend
		case irGt:
			o = append(o, 0x55, 0xad) // i64.gt_s, extend
		case irLte:
			o = append(o, 0x57, 0xad) // i64.le_s, extend
		case irEq:
			o = append(o, 0x51, 0xad)
		case irIsZero:
			o = append(o, 0x50, 0xad)

		// coretypes.Collection operations → call imported host functions
		case irGet:
			o = append(o, 0x10)           // call
			o = corewasm.AppendULEB(o, 0) // import index 0 = get
		case irGet3:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 1) // import index 1 = get3
		case irAssoc:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 2) // import index 2 = assoc
		case irNth:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 3) // import index 3 = nth
		case irConj:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 4) // import index 4 = conj
		case irCount:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 5) // import index 5 = count
		case irFirst:
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, 6) // import index 6 = first

		case irJumpIfNot:
			pc += 2
			o = append(o, 0xa7)       // i32.wrap_i64
			o = append(o, 0x04, 0x40) // if void
			depth++

		case irJump:
			pc += 2
			o = append(o, 0x05) // else

		case irReturn:
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth+1)
			if depth > 0 && pc < len(code) && code[pc] != irJump {
				o = append(o, 0x05)
			}

		case irCallSelf:
			pc += 2                                 // skip nargs
			o = append(o, 0x10)                     // call
			o = corewasm.AppendULEB(o, mainFuncIdx) // self = main function index

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			for i := nargs - 1; i >= 0; i-- {
				o = append(o, 0x21)
				o = corewasm.AppendULEB(o, i)
			}
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth)
			pc = len(code) // dead code after recur

		default:
			return nil
		}
	}

	for depth > 0 {
		o = append(o, 0x0b)
		depth--
	}
	o = append(o, 0x0b)       // end loop
	o = append(o, 0x42, 0x00) // i64.const 0
	o = append(o, 0x0b)       // end block
	o = append(o, 0x0b)       // end func
	return o
}

// ---- wasm_helper_backend.go ----
// wasm_multifn.go — experimental one-helper multi-function WASM modules.
//
// This removes the host boundary for hot loops that call a single captured
// helper function. The caller remains the exported exec function; the helper is
// emitted as a second internal WASM function and irCallSlot becomes a direct
// WASM call. This is intentionally not wired into the default eval path yet.

type wasmMultiKey struct {
	caller *IRProgram
	helper *FnArityExpr
}

var wasmMultiFnCache sync.Map    // map[wasmMultiKey]*WasmProgram
var wasmMultiFnProgFail sync.Map // map[*IRProgram]bool for no-helper/auto-rejected callers

func wasmGetCachedWithOneHelper(prog *IRProgram, slots []coretypes.Object) *WasmProgram {
	if !corert.WasmMultiFnEnabled() {
		return nil
	}
	if _, failed := wasmMultiFnProgFail.Load(prog); failed {
		return nil
	}
	helperSlot, helperFn, helperProg, helperParams, ok := findSingleWasmHelper(prog, slots)
	if !ok {
		wasmMultiFnProgFail.Store(prog, true)
		return nil
	}
	key := wasmMultiKey{caller: prog, helper: &helperFn.fnExpr.arities[0]}
	if v, ok := wasmMultiFnCache.Load(key); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFail {
			return nil
		}
		return wp
	}
	wp := wasmCompileWithOneHelper(prog, helperSlot, helperProg, helperParams)
	if wp == nil {
		wasmMultiFnCache.Store(key, wasmFail)
		return nil
	}
	wasmMultiFnCache.Store(key, wp)
	return wp
}

func findSingleWasmHelper(prog *IRProgram, slots []coretypes.Object) (int, *Fn, *IRProgram, int, bool) {
	model := prog.neutralModel()
	if model == nil {
		return 0, nil, nil, 0, false
	}
	code := model.Code
	pc := 0
	helperSlot := -1
	helperNArgs := -1
	helperCalls := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot, irJumpIfNot, irJump, irCallSelf, irBuildVec, irNthStringASCII:
			pc += 2
		case irCallSlot:
			slot := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			helperCalls++
			if helperSlot < 0 {
				helperSlot = slot
				helperNArgs = nargs
			} else if helperSlot != slot || helperNArgs != nargs {
				return 0, nil, nil, 0, false
			}
		case irRecur:
			pc += 4
			tgt := int(code[pc-2])<<8 | int(code[pc-1])
			if tgt != 0 {
				pc += 2
			}
		}
	}
	if helperSlot < 0 || helperSlot >= len(slots) {
		return 0, nil, nil, 0, false
	}
	helperFn, ok := slots[helperSlot].(*Fn)
	if !ok || len(helperFn.fnExpr.arities) != 1 || len(helperFn.fnExpr.arities[0].args) != helperNArgs {
		return 0, nil, nil, 0, false
	}
	helperProg := irCompileFn(helperFn)
	if helperProg == nil || helperProg.hasSelf {
		return 0, nil, nil, 0, false
	}
	helperModel := helperProg.neutralModel()
	if helperModel == nil || !corewasm.Eligible(helperModel.Code) {
		return 0, nil, nil, 0, false
	}
	if !corewasm.EligibleWithHelper(model.Code, helperSlot) {
		return 0, nil, nil, 0, false
	}
	// Multi-function WASM: enable for both integer and float helpers.
	// Originally gated because float helpers were believed to regress,
	// but 5x median probes show no regression vs auto (within noise).
	if !corert.WasmMultiFnForce() && helperCalls == 0 {
		return 0, nil, nil, 0, false
	}
	return helperSlot, helperFn, helperProg, helperNArgs, true
}

func wasmCompileWithOneHelper(prog *IRProgram, helperSlot int, helperProg *IRProgram, helperParams int) *WasmProgram {
	model := prog.neutralModel()
	if helperProg == nil {
		return nil
	}
	helperModel := helperProg.neutralModel()
	if model == nil || helperModel == nil {
		return nil
	}
	useFloat := corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0) || corewasm.UsesFloat(helperModel.Code, len(helperModel.FloatConsts) > 0)
	callerBody := compileWasmBodyWithHelper(prog, useFloat, helperSlot, 1)
	if callerBody == nil {
		return nil
	}
	helperBody := compileWasmBodyWithHelperParams(helperProg, useFloat, -1, -1, helperParams)
	if helperBody == nil {
		return nil
	}
	valType := corewasm.ValTypeI64
	if useFloat {
		valType = corewasm.ValTypeF64
	}
	bin := corewasm.TwoFuncExecModule(model.NumSlots, helperParams, valType, callerBody, helperBody)

	rt := getWasmRT()
	ctx := context.Background()
	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return nil
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(corewasm.NextWasmModuleName()))
	if err != nil {
		return nil
	}
	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		return nil
	}
	return &WasmProgram{mod: mod, execFn: execFn, useFloat: useFloat, hasImports: false, constants: prog.constants}
}

// ---- wasm_host_funcs.go ----
// wasm_host.go — Host function imports for WASM modules.
//
// Provides Joker collection operations (get, assoc, nth, conj, first, count)
// as imported host functions that WASM-compiled loops can call.
//
// Objects are passed as opaque handles (uint64 indices into a per-execution
// object table). Numeric values (Int, Double) are passed directly as i64/f64.
//
// The object table is thread-local to each wasmExec call, stored in a
// context value so host functions can access it.

// wasmHostModuleName is the import module name for Joker host functions.
const wasmHostModuleName = corewasm.HostModuleName

var wasmHostRegistered sync.Once

// registerWasmHost registers the "joker" host module with collection operations.
func registerWasmHost(rt wazero.Runtime) {
	wasmHostRegistered.Do(func() {
		ctx := context.Background()
		builder := rt.NewHostModuleBuilder(wasmHostModuleName)

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64, key uint64) uint64 {
			return corewasm.HostGet(corewasm.GetObjectTable(ctx), collHandle, key, 0)
		}).Export("get")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64, key uint64, def uint64) uint64 {
			return corewasm.HostGet(corewasm.GetObjectTable(ctx), collHandle, key, def)
		}).Export("get3")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64, key uint64, val uint64) uint64 {
			return corewasm.HostAssoc(corewasm.GetObjectTable(ctx), collHandle, key, val)
		}).Export("assoc")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64, idx uint64) uint64 {
			return corewasm.HostNth(corewasm.GetObjectTable(ctx), collHandle, idx, 0, func(coll coretypes.Object, i int) (coretypes.Object, bool) {
				if v, ok := coll.(*corecollections.ArrayVector); ok && i >= 0 && i < len(v.Arr) {
					return v.Arr[i], true
				}
				return nil, false
			})
		}).Export("nth")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64, val uint64) uint64 {
			return corewasm.HostConj(corewasm.GetObjectTable(ctx), collHandle, val)
		}).Export("conj")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64) uint64 {
			return corewasm.HostFirst(corewasm.GetObjectTable(ctx), collHandle, 0, func(coll coretypes.Object) (coretypes.Object, bool) {
				if v, ok := coll.(*corecollections.ArrayVector); ok && len(v.Arr) > 0 {
					return v.Arr[0], true
				}
				return nil, false
			})
		}).Export("first")

		builder.NewFunctionBuilder().WithFunc(func(ctx context.Context, collHandle uint64) uint64 {
			return corewasm.HostCount(corewasm.GetObjectTable(ctx), collHandle)
		}).Export("count")

		builder.Instantiate(ctx)
	})
}

// Ensure api import is used
var _ api.Module

// ---- wasm_mem_nth_backend.go ----
// wasm_mem_nth.go — WASM f64 codegen with linear memory for vector nth.
//
// For loops that use f64 arithmetic + vector nth + optional helper calls,
// vector elements are copied into WASM linear memory before execution.
// The nth opcode becomes an f64.load from computed memory address.
// This eliminates all Go↔WASM boundary crossings for nth.

var wasmMemNthCache sync.Map

type wasmMemNthKey struct {
	prog   *IRProgram
	helper *IRProgram
}

// wasmMemNthStaticEligible is a fast static check (no slot inspection).
func wasmMemNthStaticEligible(prog *IRProgram) bool {
	if !corert.WasmMemNthEnabled() {
		return false
	}
	model := prog.neutralModel()
	if model == nil {
		return false
	}
	code := model.Code
	pc := 0
	hasNth := false
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irDiv, irRem, irInc, irDec,
			irLt, irGte, irGt, irLte, irEq, irIsZero, irReturn, irSqrt:
			// ok
		case irNth:
			hasNth = true
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				return false
			}
		default:
			return false
		}
	}
	return hasNth
}

// Requires: f64 arithmetic, irNth on captured vectors, optional irCallSlot.
func wasmMemNthEligible(prog *IRProgram, slots []coretypes.Object) bool {
	if prog == nil {
		return false
	}
	model := prog.neutralModel()
	if model == nil || len(slots) < model.NumSlots {
		return false
	}
	// Check if any slot is a Double (indicates float loop)
	hasFloat := false
	for _, s := range slots {
		if _, ok := s.(coretypes.Double); ok {
			hasFloat = true
			break
		}
	}
	if !hasFloat {
		hasFloat = corewasm.UsesFloat(model.Code, len(model.FloatConsts) > 0)
	}
	if !hasFloat {
		return false
	}
	code := model.Code
	pc := 0
	hasNth := false
	nthSlots := make(map[int]bool) // which slots are used as nth collection args
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irAdd, irSub, irMul, irDiv, irRem, irInc, irDec,
			irLt, irGte, irGt, irLte, irEq, irIsZero, irReturn, irSqrt:
			// ok
		case irNth:
			hasNth = true
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				return false
			}
		default:
			return false
		}
	}
	if !hasNth {
		return false
	}
	// Find which slots are loaded before nth and verify they're vectors
	pc = 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLoadSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Check if next non-load op is nth
			if pc < len(code) {
				nextOp := code[pc]
				if nextOp == irLoadSlot {
					// Pattern: load coll, load idx, nth
					nextSlot := int(code[pc+1])<<8 | int(code[pc+2])
					if pc+3 < len(code) && code[pc+3] == irNth {
						_ = nextSlot
						nthSlots[slotIdx] = true
					}
				}
			}
		case irLiteral, irStoreSlot:
			pc += 2
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				pc += 2
			}
		default:
			// single byte
		}
	}
	// Verify that nth collection slots hold ArrayVectors
	for slot := range nthSlots {
		if slot >= len(slots) {
			return false
		}
		if _, ok := slots[slot].(*corecollections.ArrayVector); !ok {
			return false
		}
	}
	return true
}

type wasmMemNthCached struct {
	wp         *WasmProgram
	vecSlotIdx []int     // initSlots indices that hold vectors
	memOffsets []int     // byte offset for each vecSlotIdx
	lastVecPtr []uintptr // last-written vector pointer per slot
	paramsBuf  []uint64  // reusable params buffer
	buf8       [8]byte   // reusable byte buffer for f64 writes
}

// wasmMemNthCompileAndExec compiles and executes the loop with linear memory nth.
func wasmMemNthCompileAndExec(prog *IRProgram, slots []coretypes.Object) coretypes.Object {
	if !wasmMemNthEligible(prog, slots) {
		return nil
	}
	helperSlot, helperProg := findHelperForMemNth(prog, slots)

	key := wasmMemNthKey{prog: prog, helper: helperProg}
	var c *wasmMemNthCached
	if v, ok := wasmMemNthCache.Load(key); ok {
		if v == nil {
			return nil // cached failure
		}
		c = v.(*wasmMemNthCached)
	} else {
		wp := buildMemNthModule(prog, helperSlot, helperProg)
		if wp == nil {
			wasmMemNthCache.Store(key, nil)
			return nil
		}
		// Identify vector slots
		vecSlots := findVecSlots(prog, slots)
		var vecIdx []int
		var memOff []int
		offset := 0
		for _, vs := range vecSlots {
			vecIdx = append(vecIdx, vs.slot)
			memOff = append(memOff, offset)
			offset += len(vs.vec.Arr) * 8
		}
		model := prog.neutralModel()
		if model == nil {
			wasmMemNthCache.Store(key, nil)
			return nil
		}
		c = &wasmMemNthCached{
			wp:         wp,
			vecSlotIdx: vecIdx,
			memOffsets: memOff,
			lastVecPtr: make([]uintptr, len(vecIdx)),
			paramsBuf:  make([]uint64, model.NumSlots),
		}
		wasmMemNthCache.Store(key, c)
	}

	// Write vector data to memory — skip if same vector pointer
	mem := c.wp.mod.ExportedMemory("memory")
	if mem == nil {
		return nil
	}
	for vi, slotIdx := range c.vecSlotIdx {
		vec := slots[slotIdx].(*corecollections.ArrayVector)
		vecPtr := reflect.ValueOf(vec).Pointer()
		if vecPtr != c.lastVecPtr[vi] {
			base := c.memOffsets[vi]
			for i, obj := range vec.Arr {
				var fv float64
				switch v := obj.(type) {
				case coretypes.Double:
					fv = v.D
				case coretypes.Int:
					fv = float64(v.I)
				default:
					return nil
				}
				binary.LittleEndian.PutUint64(c.buf8[:], math.Float64bits(fv))
				if !mem.Write(uint32(base+i*8), c.buf8[:]) {
					return nil
				}
			}
			c.lastVecPtr[vi] = vecPtr
		}
	}

	// Build params — reuse buffer
	for i, s := range slots {
		switch v := s.(type) {
		case coretypes.Int:
			c.paramsBuf[i] = math.Float64bits(float64(v.I))
		case coretypes.Double:
			c.paramsBuf[i] = math.Float64bits(v.D)
		default:
			// coretypes.Vector slot: pass memory byte offset
			for vi, si := range c.vecSlotIdx {
				if si == i {
					c.paramsBuf[i] = math.Float64bits(float64(c.memOffsets[vi]))
					break
				}
			}
		}
	}

	ctx := context.Background()
	if err := c.wp.execFn.CallWithStack(ctx, c.paramsBuf); err != nil {
		return nil
	}
	return coretypes.Double{D: math.Float64frombits(c.paramsBuf[0])}
}

type vecSlotInfo struct {
	slot int
	vec  *corecollections.ArrayVector
}

func findVecSlots(prog *IRProgram, slots []coretypes.Object) []vecSlotInfo {
	// Find slots loaded before irNth
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	code := model.Code
	var result []vecSlotInfo
	seen := make(map[int]bool)
	pc := 0
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLoadSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if pc+3 < len(code) && code[pc] == irLoadSlot && code[pc+3] == irNth {
				if !seen[slotIdx] {
					if v, ok := slots[slotIdx].(*corecollections.ArrayVector); ok {
						result = append(result, vecSlotInfo{slot: slotIdx, vec: v})
						seen[slotIdx] = true
					}
				}
			}
		case irLiteral, irStoreSlot:
			pc += 2
		case irCallSlot:
			pc += 4
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				pc += 2
			}
		default:
		}
	}
	return result
}

func findHelperForMemNth(prog *IRProgram, slots []coretypes.Object) (int, *IRProgram) {
	model := prog.neutralModel()
	if model == nil {
		return -1, nil
	}
	code := model.Code
	pc := 0
	helperSlot := -1
	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irCallSlot:
			s := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			if helperSlot < 0 {
				helperSlot = s
			} else if helperSlot != s {
				return -1, nil
			}
		case irLiteral, irLoadSlot, irStoreSlot:
			pc += 2
		case irJumpIfNot, irJump:
			pc += 2
		case irRecur:
			pc += 4
			if tgt := int(code[pc-2])<<8 | int(code[pc-1]); tgt != 0 {
				pc += 2
			}
		default:
		}
	}
	if helperSlot < 0 || helperSlot >= len(slots) {
		return -1, nil
	}
	fn, ok := slots[helperSlot].(*Fn)
	if !ok {
		return -1, nil
	}
	hp := irCompileFn(fn)
	hm := hp.neutralModel()
	if hp == nil || hm == nil || !corewasm.Eligible(hm.Code) {
		return -1, nil
	}
	return helperSlot, hp
}

func buildMemNthModule(prog *IRProgram, helperSlot int, helperProg *IRProgram) *WasmProgram {
	rt := getWasmRT()
	if rt == nil {
		return nil
	}
	helperFuncIdx := -1
	helperParams := 0
	if helperProg != nil {
		helperFuncIdx = 1
		helperModel := helperProg.neutralModel()
		if helperModel == nil {
			return nil
		}
		helperParams = helperModel.NumSlots
	}
	model := prog.neutralModel()
	if model == nil {
		return nil
	}

	callerBody := buildMemNthBody(prog, helperSlot, helperFuncIdx, model.NumSlots)
	if callerBody == nil {
		return nil
	}
	var helperBody []byte
	if helperProg != nil {
		helperBody = compileWasmBodyWithHelperParams(helperProg, true, -1, -1, helperParams)
		if helperBody == nil {
			return nil
		}
	}

	bin := corewasm.MemoryExportModule(model.NumSlots, helperParams, callerBody, helperBody)
	ctx := context.Background()
	compiled, err := rt.CompileModule(ctx, bin)
	if err != nil {
		return nil
	}
	mod, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(corewasm.NextWasmModuleName()))
	if err != nil {
		return nil
	}
	execFn := mod.ExportedFunction("exec")
	if execFn == nil {
		return nil
	}
	return &WasmProgram{mod: mod, execFn: execFn, useFloat: true, hasImports: false, constants: prog.constants}
}

func buildMemNthBody(prog *IRProgram, helperSlot, helperFuncIdx, numParams int) []byte {
	model := prog.neutralModel()
	if model == nil {
		return nil
	}
	var o []byte
	extra := model.NumSlots - numParams
	// Local decls: extra f64 locals + 1 i32 temp for nth address computation
	if extra > 0 {
		o = append(o, 0x02) // 2 groups
		o = corewasm.AppendULEB(o, extra)
		o = append(o, 0x7c) // f64
		o = append(o, 0x01) // 1 i32
		o = append(o, 0x7f) // i32
	} else {
		o = append(o, 0x01) // 1 group
		o = append(o, 0x01) // 1 i32
		o = append(o, 0x7f)
	}
	i32Temp := model.NumSlots // local index of i32 temp
	o = append(o, 0x02, 0x7c) // block $exit → f64
	o = append(o, 0x03, 0x40) // loop $loop → void

	code := model.Code
	pc := 0
	depth := 0

	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			c := prog.constants[idx]
			var fv float64
			switch v := c.(type) {
			case coretypes.Int:
				fv = float64(v.I)
			case coretypes.Double:
				fv = v.D
			default:
				return nil
			}
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, fv)
		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x20)
			o = corewasm.AppendULEB(o, idx)
		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			o = append(o, 0x21)
			o = corewasm.AppendULEB(o, idx)
		case irAdd:
			o = append(o, 0xa0)
		case irSub:
			o = append(o, 0xa1)
		case irMul:
			o = append(o, 0xa2)
		case irDiv:
			o = append(o, 0xa3)
		case irSqrt:
			o = append(o, 0x9f)
		case irInc:
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, 1.0)
			o = append(o, 0xa0)
		case irDec:
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, 1.0)
			o = append(o, 0xa1)
		case irLt:
			o = append(o, 0x63) // f64.lt → i32
			o = append(o, 0xb7) // f64.convert_i32_s → f64
		case irGte:
			o = append(o, 0x65) // f64.ge → i32
			o = append(o, 0xb7)
		case irGt:
			o = append(o, 0x64) // f64.gt → i32
			o = append(o, 0xb7)
		case irLte:
			o = append(o, 0x66) // f64.le → i32
			o = append(o, 0xb7)
		case irEq:
			o = append(o, 0x61) // f64.eq → i32
			o = append(o, 0xb7)
		case irIsZero:
			o = append(o, 0x44)
			o = corewasm.AppendF64(o, 0.0)
			o = append(o, 0x61)
			o = append(o, 0xb7)

		case irNth:
			// Stack: [base_offset_f64, idx_f64]
			// Compute address: i32(base) + i32(idx) * 8
			o = append(o, 0xaa) // i32.trunc_f64_s (idx → i32)
			o = append(o, 0x21) // local.set i32_temp
			o = corewasm.AppendULEB(o, i32Temp)
			o = append(o, 0xaa) // i32.trunc_f64_s (base → i32)
			o = append(o, 0x20) // local.get i32_temp
			o = corewasm.AppendULEB(o, i32Temp)
			o = append(o, 0x41, 0x08)       // i32.const 8
			o = append(o, 0x6c)             // i32.mul
			o = append(o, 0x6a)             // i32.add
			o = append(o, 0x2b, 0x03, 0x00) // f64.load align=3 offset=0

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			_ = nargs
			if slotIdx != helperSlot || helperFuncIdx < 0 {
				return nil
			}
			o = append(o, 0x10)
			o = corewasm.AppendULEB(o, helperFuncIdx)
		case irJumpIfNot:
			pc += 2
			// Comparison results are f64 (0.0 or 1.0), convert to i32 for if
			o = append(o, 0xaa) // i32.trunc_f64_s
			o = append(o, 0x04, 0x40)
			depth++
		case irJump:
			pc += 2
			o = append(o, 0x05)
		case irReturn:
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth+1)
			if depth > 0 && pc < len(code) && code[pc] != irJump {
				o = append(o, 0x05)
			}
		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 4
			for i := nargs - 1; i >= 0; i-- {
				o = append(o, 0x21)
				o = corewasm.AppendULEB(o, i)
			}
			o = append(o, 0x0c)
			o = corewasm.AppendULEB(o, depth)
			pc = len(code)
		default:
			return nil
		}
	}
	for depth > 0 {
		o = append(o, 0x0b)
		depth--
	}
	o = append(o, 0x0b)
	o = append(o, 0x44)
	o = corewasm.AppendF64(o, 0.0)
	o = append(o, 0x0b)
	o = append(o, 0x0b)
	return o
}

// ---- boxed_exec.go ----

// ---- boxed_exec.go ----
// ---------- Interpreter ----------

func irExec(prog *IRProgram, initSlots []coretypes.Object) coretypes.Object {
	defer traceIRProgramCall(prog, len(initSlots))()
	irProfileExecStart()
	defer irProfileMaybeWrite()
	var slots []coretypes.Object
	if runtimeExec.ProgramNumSlots(prog) <= 16 {
		var buf [16]coretypes.Object
		slots = buf[:runtimeExec.ProgramNumSlots(prog)]
	} else {
		slots = make([]coretypes.Object, runtimeExec.ProgramNumSlots(prog))
	}
	copy(slots, initSlots)
	// Pre-fill captured closure values into their assigned slots
	if !runtimeExec.ApplyProgramCaptureSlots(prog, slots) {
		return nil
	}

	// Escape analysis: convert safe local values to transient builders.
	// Only run if there are actually mutable candidate slots.
	if runtimeExec.HasMutableSlotCandidate(slots) {
		escapeInfo := runtimeExec.ProgramEscapeInfo(prog)
		if escapeInfo == nil {
			return nil
		}
		for i, s := range slots {
			slots[i] = runtimeExec.MutableSlotObject(s, escapeInfo, i)
		}
	}

	var stack []coretypes.Object
	var stackBuf [16]coretypes.Object
	stack = stackBuf[:0]
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	// Frame stack for irCallSelf — avoids recursive irExec calls
	var frameStack *coreirx.FrameStack[coretypes.Object]
	defer func() { coreirx.ReleaseFrameStack(frameStack) }()
	var selfTraceStack []func()

	var irProfPrev byte
	var irProfHasPrev bool
	irProfStarted := irProfileStart()
	defer func() { irProfileFinish(irProfPrev, irProfHasPrev, irProfStarted) }()
loop:
	for pc < len(code) {
		op := code[pc]
		irProfStarted = irProfileOp(irProfPrev, op, irProfHasPrev, irProfStarted)
		irProfPrev, irProfHasPrev = op, true
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			constant, ok := runtimeExec.ProgramConstant(prog, idx)
			if !ok {
				return nil
			}
			stack = append(stack, constant)

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stack = append(stack, slots[idx])

		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			slots[idx] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]

		case irAdd:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					r := av.I + bv.I
					if bv.I > 0 && r < av.I || bv.I < 0 && r > av.I {
						stack = append(stack, procAdd([]coretypes.Object{a, b}))
					} else {
						stack = append(stack, coretypes.Int{I: r})
					}
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: float64(av.I) + bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Double{D: av.D + float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: av.D + bv.D})
					continue
				}
			}
			stack = append(stack, procAdd([]coretypes.Object{a, b}))

		case irSub:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					r := av.I - bv.I
					if bv.I < 0 && r < av.I || bv.I > 0 && r > av.I {
						stack = append(stack, procSubtract([]coretypes.Object{a, b}))
					} else {
						stack = append(stack, coretypes.Int{I: r})
					}
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: float64(av.I) - bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Double{D: av.D - float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: av.D - bv.D})
					continue
				}
			}
			stack = append(stack, procSubtract([]coretypes.Object{a, b}))

		case irMul:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					r := av.I * bv.I
					if av.I != 0 && ((av.I == -1 && bv.I == coretypes.MinInt) || (bv.I == -1 && av.I == coretypes.MinInt) || r/av.I != bv.I) {
						stack = append(stack, procMultiply([]coretypes.Object{a, b}))
					} else {
						stack = append(stack, coretypes.Int{I: r})
					}
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: float64(av.I) * bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Double{D: av.D * float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Double{D: av.D * bv.D})
					continue
				}
			}
			stack = append(stack, procMultiply([]coretypes.Object{a, b}))

		case irRem:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-1]
			stack[len(stack)-1] = procRem([]coretypes.Object{a, b})

		case irDiv:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-1]
			stack[len(stack)-1] = procDivide([]coretypes.Object{a, b})

		case irInc:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Int:
				if av.I == coretypes.MaxInt {
					stack = append(stack, procInc([]coretypes.Object{a}))
				} else {
					stack = append(stack, coretypes.Int{I: av.I + 1})
				}
			case coretypes.Double:
				stack = append(stack, coretypes.Double{D: av.D + 1})
			default:
				stack = append(stack, procInc([]coretypes.Object{a}))
			}

		case irDec:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Int:
				if av.I == coretypes.MinInt {
					stack = append(stack, procDec([]coretypes.Object{a}))
				} else {
					stack = append(stack, coretypes.Int{I: av.I - 1})
				}
			case coretypes.Double:
				stack = append(stack, coretypes.Double{D: av.D - 1})
			default:
				stack = append(stack, procDec([]coretypes.Object{a}))
			}

		case irLt:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I < bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) < bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D < float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D < bv.D})
					continue
				}
			}
			stack = append(stack, procLt([]coretypes.Object{a, b}))

		case irGte:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I >= bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) >= bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D >= float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D >= bv.D})
					continue
				}
			}
			stack = append(stack, procGte([]coretypes.Object{a, b}))

		case irGt:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I > bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) > bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D > float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D > bv.D})
					continue
				}
			}
			stack = append(stack, procGt([]coretypes.Object{a, b}))

		case irLte:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I <= bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) <= bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D <= float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D <= bv.D})
					continue
				}
			}
			stack = append(stack, procLte([]coretypes.Object{a, b}))

		case irCursorChar:
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.CursorChar(cur)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irCursorNext:
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.CursorNext(cur)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irCursorDone:
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.CursorDone(cur)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irApply:
			argsSeq := stack[len(stack)-1]
			fnObj := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			args, ok := runtimeExec.CallArgs(argsSeq)
			if !ok {
				return nil
			}
			result, ok := runtimeExec.CallObject(fnObj, args)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irThrow:
			v := stack[len(stack)-1]
			runtimeExec.Throw(v)

		case irTryCatch:
			pc += 4
			return nil

		case irPop:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

		case irMakeFn:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnExpr, ok := runtimeExec.ProgramFnExpr(prog, idx)
			if !ok {
				return nil
			}
			fn := runtimeExec.MakeFn(fnExpr, slots)
			stack = append(stack, fn)

		case irBitAnd:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I & b.I})
		case irBitOr:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I | b.I})
		case irBitNot:
			a := stack[len(stack)-1].(coretypes.Int)
			stack = stack[:len(stack)-1]
			stack = append(stack, coretypes.Int{I: ^a.I})
		case irBitShiftLeft:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I << uint(b.I)})
		case irBitShiftRight:
			b, a := stack[len(stack)-1].(coretypes.Int), stack[len(stack)-2].(coretypes.Int)
			stack = stack[:len(stack)-2]
			stack = append(stack, coretypes.Int{I: a.I >> uint(b.I)})

		case irCase:
			// Jump table: dispatch by integer value
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nCases := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			var testVal int
			switch v := slots[slotIdx].(type) {
			case coretypes.Int:
				testVal = v.I
			default:
				// Skip table, jump to default
				pc += nCases * 4
				pc = int(code[pc])<<8 | int(code[pc+1])
				continue
			}
			matched := false
			for i := 0; i < nCases; i++ {
				caseVal := int(int16(code[pc])<<8 | int16(code[pc+1]))
				target := int(code[pc+2])<<8 | int(code[pc+3])
				pc += 4
				if testVal == caseVal {
					pc = target
					matched = true
					break
				}
			}
			if !matched {
				pc = int(code[pc])<<8 | int(code[pc+1])
			}

		case irEq:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			switch av := a.(type) {
			case coretypes.Int:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.I == bv.I})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: float64(av.I) == bv.D})
					continue
				}
			case coretypes.Double:
				switch bv := b.(type) {
				case coretypes.Int:
					stack = append(stack, coretypes.Boolean{B: av.D == float64(bv.I)})
					continue
				case coretypes.Double:
					stack = append(stack, coretypes.Boolean{B: av.D == bv.D})
					continue
				}
			case coretypes.Char:
				if bv, ok := b.(coretypes.Char); ok {
					stack = append(stack, coretypes.Boolean{B: av.Ch == bv.Ch})
					continue
				}
			case coretypes.String:
				if bv, ok := b.(coretypes.String); ok {
					stack = append(stack, coretypes.Boolean{B: av.S == bv.S})
					continue
				}
			}
			stack = append(stack, coretypes.Boolean{B: runtimeExec.Equal(a, b)})

		case irIsZero:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Int:
				stack = append(stack, coretypes.Boolean{B: av.I == 0})
			case coretypes.Double:
				stack = append(stack, coretypes.Boolean{B: av.D == 0})
			default:
				stack = append(stack, procIsZero([]coretypes.Object{a}))
			}

		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			val := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch v := val.(type) {
			case Nil:
				pc = target
			case coretypes.Boolean:
				if !v.B {
					pc = target
				}
			}

		case irJump:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc = target

		case irRecur:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			targetPC := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// For nested loops, the recur target baseSlot might not be 0
			// We need to figure out which slots to write to.
			// Convention: recur writes to slots starting from the baseSlot
			// of the loop that recur targets. For the top-level loop, baseSlot=0.
			// For nested loops, we determine baseSlot from the target PC.
			// Simple approach: if targetPC==0, write to slots 0..n-1 (backward compat).
			// Otherwise, we need the baseSlot encoded somewhere.
			// For now, recur always writes to the slots at the end of stack in order.
			if targetPC == 0 {
				// Top-level loop recur
				for i := n - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			} else {
				// Nested loop recur — find the base slot from the compile info
				// We store base slot in the bytecode too: nargs(2) + targetPC(2) + baseSlot(2)
				// ... but we didn't emit baseSlot yet. Let's add it.
				// For now, infer: the slots for this loop start at (numSlots - n) or
				// we need to extend the encoding.
				// Quick fix: also encode baseSlot
				baseSlot := int(code[pc])<<8 | int(code[pc+1])
				pc += 2
				for i := n - 1; i >= 0; i-- {
					slots[baseSlot+i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			}
			pc = targetPC
			stack = stack[:0]
			goto loop

		case irReturn:
			if len(stack) == 0 {
				if frameStack != nil && frameStack.Depth() > 0 {
					if len(selfTraceStack) > 0 {
						exit := selfTraceStack[len(selfTraceStack)-1]
						selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
						exit()
					}
					var sl int
					pc, sl = frameStack.Pop(slots)
					stack = stack[:sl]
					stack = append(stack, NIL)
					continue
				}
				return NIL
			}
			result := stack[len(stack)-1]
			if frameStack != nil && frameStack.Depth() > 0 {
				result = runtimeExec.PersistentResult(result)
				if len(selfTraceStack) > 0 {
					exit := selfTraceStack[len(selfTraceStack)-1]
					selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
					exit()
				}
				var sl int
				pc, sl = frameStack.Pop(slots)
				stack = stack[:sl]
				stack = append(stack, result)
				continue
			}
			return runtimeExec.PersistentResult(result)
		case irGet:
			key := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if _, ok := coll.(coretypes.Gettable); !ok {
				return nil
			}
			stack = append(stack, runtimeExec.Get(coll, key, NIL))

		case irGet3:
			def := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			stack = append(stack, runtimeExec.Get(coll, key, def))

		case irAssoc:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			result, ok := runtimeExec.Assoc(coll, key, val)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irNth:
			idxObj := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			idx, iok := idxObj.(coretypes.Int)
			if !iok {
				return nil
			}
			result, ok := runtimeExec.Nth(coll, idx.I)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irConj:
			val := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			result, ok := runtimeExec.Conj(coll, val)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irSqrt:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch av := a.(type) {
			case coretypes.Double:
				stack = append(stack, coretypes.Double{D: math.Sqrt(av.D)})
			case coretypes.Int:
				stack = append(stack, coretypes.Double{D: math.Sqrt(float64(av.I))})
			default:
				return nil
			}

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnObj := slots[slotIdx]
			// Fast path: native f64 closure (fn-level or loop-level)
			if fnProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if nativeHelper, ok := runtimeExec.NativeHelper(fnProg); ok {
					var f64buf [4]float64
					var f64args []float64
					if nargs <= len(f64buf) {
						f64args = f64buf[:nargs]
					} else {
						f64args = make([]float64, nargs)
					}
					for i := nargs - 1; i >= 0; i-- {
						switch v := stack[len(stack)-1].(type) {
						case coretypes.Double:
							f64args[i] = v.D
						case coretypes.Int:
							f64args[i] = float64(v.I)
						default:
							f64args[i] = 0
						}
						stack = stack[:len(stack)-1]
					}
					stack = append(stack, coretypes.Double{D: nativeHelper(coreirx.Float64(f64args))})
					continue
				}
			}
			// Slow path
			var args []coretypes.Object
			var argsBuf [4]coretypes.Object
			if nargs > 0 {
				if nargs <= len(argsBuf) {
					args = argsBuf[:nargs]
				} else {
					args = make([]coretypes.Object, nargs)
				}
				for i := nargs - 1; i >= 0; i-- {
					args[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			}
			// Try WASM fn dispatch first, then IR, then tree-walker
			if result, ok := runtimeExec.FnWasmExec(fnObj, args); ok {
				stack = append(stack, result)
				continue
			}
			if baseProg, ok := runtimeExec.FnProgram(fnObj); ok {
				// Try IR — typed executor first, skip if previously failed
				if fnProg := runtimeExec.DispatchArityProgram(baseProg, nargs); runtimeExec.CanExecuteIR(fnProg) {
					callArgs, ok := runtimeExec.FnCallSlots(fnObj, fnProg, args)
					if !ok {
						return nil
					}
					if runtimeExec.CanExecuteTypedIR(fnProg) {
						if result := irExecTyped(fnProg, callArgs); result != nil {
							stack = append(stack, result)
							continue
						}
						runtimeExec.MarkTypedExecutionFailed(fnProg)
					}
					if result := irExec(fnProg, callArgs); result != nil {
						stack = append(stack, result)
						continue
					}
				}
			}
			// Fallback to normal Fn.Call
			result, ok := runtimeExec.CallObjectWithSyntheticCallExpr(fnObj, args)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Use frame stack for bounded recursion, fall back to
			// recursive irExec for deep/exponential recursion.
			if frameStack == nil {
				frameStack = coreirx.NewFrameStack[coretypes.Object](runtimeExec.ProgramNumSlots(prog))
			}
			if frameStack.Depth() < 512 {
				selfTraceStack = append(selfTraceStack, traceIRProgramCall(prog, nargs))
				frameStack.Push(pc, slots, len(stack)-nargs)
				for i := nargs - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
				// Only clear slots beyond nargs if there are captures
				if runtimeExec.ProgramHasCaptureSlots(prog) {
					for i := nargs; i < len(slots); i++ {
						slots[i] = nil
					}
					if !runtimeExec.ApplyProgramCaptureSlots(prog, slots) {
						return nil
					}
				}
				pc = 0
			} else {
				// Deep recursion: fall back to recursive call
				var args []coretypes.Object
				var argsBuf [4]coretypes.Object
				if nargs <= len(argsBuf) {
					args = argsBuf[:nargs]
				} else {
					args = make([]coretypes.Object, nargs)
				}
				for i := nargs - 1; i >= 0; i-- {
					args[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
				result := irExec(prog, args)
				if result == nil {
					return nil
				}
				stack = append(stack, result)
			}

		case irFirst:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.First(a)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			arr := make([]coretypes.Object, n)
			for i := n - 1; i >= 0; i-- {
				arr[i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, runtimeExec.BuildVector(arr))

		case irStr1:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			stack = append(stack, runtimeExec.Str1(a))

		case irNthStringASCII:
			idxConst := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			idxObj := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			idx, ok := idxObj.(coretypes.Int)
			if !ok {
				return nil
			}
			result, ok := runtimeExec.NthASCIIStringConst(prog, idxConst, idx.I)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irStr2:
			b := stack[len(stack)-1]
			a := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			stack = append(stack, runtimeExec.Str2(a, b))

		case irCount:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			count, ok := runtimeExec.Count(a)
			if !ok {
				return nil
			}
			stack = append(stack, coretypes.Int{I: count})

		case irToTransient:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.ToTransient(a)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irAssocBang:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			result, ok := runtimeExec.AssocBang(coll, key, val)
			if !ok {
				return nil
			}
			stack = append(stack, result)

		case irToPersistent:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			result, ok := runtimeExec.ToPersistent(a)
			if !ok {
				return nil
			}
			stack = append(stack, result)
		case irIntCast:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch v := a.(type) {
			case coretypes.Char:
				stack = append(stack, coretypes.Int{I: int(v.Ch)})
			case coretypes.Int:
				stack = append(stack, v)
			case coretypes.Double:
				stack = append(stack, coretypes.Int{I: int(v.D)})
			default:
				return nil
			}

		case irSubs:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if nargs == 3 {
				end := stack[len(stack)-1]
				start := stack[len(stack)-2]
				sObj := stack[len(stack)-3]
				stack = stack[:len(stack)-3]
				s := sObj.(coretypes.String).S
				si := start.(coretypes.Int).I
				ei := end.(coretypes.Int).I
				if coretypes.StringIsASCII(s) {
					stack = append(stack, coretypes.String{S: s[si:ei]})
				} else {
					runes := []rune(s)
					stack = append(stack, coretypes.String{S: string(runes[si:ei])})
				}
			} else {
				start := stack[len(stack)-1]
				sObj := stack[len(stack)-2]
				stack = stack[:len(stack)-2]
				s := sObj.(coretypes.String).S
				si := start.(coretypes.Int).I
				if coretypes.StringIsASCII(s) {
					stack = append(stack, coretypes.String{S: s[si:]})
				} else {
					runes := []rune(s)
					stack = append(stack, coretypes.String{S: string(runes[si:])})
				}
			}

		case irFallback:
			return nil

		default:
			return nil
		}
	}
	if len(stack) > 0 {
		return stack[len(stack)-1]
	}
	return NIL
}

// ---- typed_exec.go ----
func irExecTyped(prog *IRProgram, initSlots []coretypes.Object) coretypes.Object {
	defer traceIRProgramCall(prog, len(initSlots))()
	irProfileExecStart()
	defer irProfileMaybeWrite()
	analysis := runtimeExec.ProgramAnalysis(prog)
	if !irTypedEligible(analysis) {
		return nil
	}
	var slotBuf [16]irValue
	var slots []irValue
	numSlots := runtimeExec.ProgramNumSlots(prog)
	if numSlots <= len(slotBuf) {
		slots = slotBuf[:numSlots]
	} else {
		slots = make([]irValue, numSlots)
	}
	for i := 0; i < len(initSlots) && i < len(slots); i++ {
		v := objectToIRValue(initSlots[i])
		if v.tag == irValString && i < len(analysis.StringAppendSlots) && (analysis.StringAppendSlots[i] || analysis.StringPrependSlots[i]) {
			buf := make([]byte, len(v.str()), len(v.str())+16)
			copy(buf, v.str())
			v = irMakeStringBuilder(buf, v.i, v.boolean())
		}
		slots[i] = v
	}
	// Pre-fill captured closure values into their assigned slots
	if !runtimeExec.ApplyProgramTypedCaptureSlots(prog, slots) {
		return nil
	}

	var stackBuf [32]irValue
	stack := stackBuf[:0]
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	// Frame stack for irCallSelf — avoids recursive irExecTyped calls
	var typedFrameStack *coreirx.FrameStack[irValue]
	defer func() { coreirx.ReleaseFrameStack(typedFrameStack) }()
	var selfTraceStack []func()
	var irProfPrev byte
	var irProfHasPrev bool
	irProfStarted := irProfileStart()
	defer func() { irProfileFinish(irProfPrev, irProfHasPrev, irProfStarted) }()

	for pc < len(code) {
		op := code[pc]
		irProfStarted = irProfileOp(irProfPrev, op, irProfHasPrev, irProfStarted)
		irProfPrev, irProfHasPrev = op, true
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			constant, ok := runtimeExec.ProgramConstant(prog, idx)
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(constant))
		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if idx < 0 || idx >= len(slots) {
				return nil
			}
			stack = append(stack, slots[idx])
		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if idx < 0 || idx >= len(slots) || len(stack) == 0 {
				return nil
			}
			slots[idx] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case irAdd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				r := a.i + b.i
				if b.i > 0 && r < a.i || b.i < 0 && r > a.i {
					stack = append(stack, objectToIRValue(procAdd([]coretypes.Object{a.object(), b.object()})))
					continue
				} else {
					stack = append(stack, irValue{tag: irValInt, i: r})
				}
			} else {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					stack = append(stack, objectToIRValue(procAdd([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					stack = append(stack, objectToIRValue(procAdd([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				stack = append(stack, irValue{tag: irValDouble, f: af + bf})
			}
		case irSub:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				r := a.i - b.i
				if b.i < 0 && r < a.i || b.i > 0 && r > a.i {
					stack = append(stack, objectToIRValue(procSubtract([]coretypes.Object{a.object(), b.object()})))
					continue
				} else {
					stack = append(stack, irValue{tag: irValInt, i: r})
				}
			} else if a.tag == irValDouble || b.tag == irValDouble {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					stack = append(stack, objectToIRValue(procSubtract([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					stack = append(stack, objectToIRValue(procSubtract([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				stack = append(stack, irValue{tag: irValDouble, f: af - bf})
			} else {
				stack = append(stack, objectToIRValue(procSubtract([]coretypes.Object{a.object(), b.object()})))
				continue
			}
		case irMul:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				r := a.i * b.i
				if a.i != 0 && ((a.i == -1 && b.i == coretypes.MinInt) || (b.i == -1 && a.i == coretypes.MinInt) || r/a.i != b.i) {
					stack = append(stack, objectToIRValue(procMultiply([]coretypes.Object{a.object(), b.object()})))
					continue
				} else {
					stack = append(stack, irValue{tag: irValInt, i: r})
				}
			} else if a.tag == irValDouble || b.tag == irValDouble {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					stack = append(stack, objectToIRValue(procMultiply([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					stack = append(stack, objectToIRValue(procMultiply([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				stack = append(stack, irValue{tag: irValDouble, f: af * bf})
			} else {
				stack = append(stack, objectToIRValue(procMultiply([]coretypes.Object{a.object(), b.object()})))
				continue
			}
		case irDiv:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-1]
			if a.tag == irValDouble && b.tag == irValDouble {
				stack[len(stack)-1] = irValue{tag: irValDouble, f: a.f / b.f}
			} else {
				stack[len(stack)-1] = objectToIRValue(procDivide([]coretypes.Object{a.object(), b.object()}))
			}

		case irRem:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-1]
			stack[len(stack)-1] = objectToIRValue(procRem([]coretypes.Object{a.object(), b.object()}))

		case irInc:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValInt || a.i == coretypes.MaxInt {
				stack = append(stack, objectToIRValue(procInc([]coretypes.Object{a.object()})))
				continue
			}
			stack = append(stack, irValue{tag: irValInt, i: a.i + 1})
		case irDec:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValInt || a.i == coretypes.MinInt {
				stack = append(stack, objectToIRValue(procDec([]coretypes.Object{a.object()})))
				continue
			}
			stack = append(stack, irValue{tag: irValInt, i: a.i - 1})
		case irLt:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i < b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f < b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f < float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) < b.f))
			} else {
				stack = append(stack, objectToIRValue(procLt([]coretypes.Object{a.object(), b.object()})))
			}
		case irGte:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i >= b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f >= b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f >= float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) >= b.f))
			} else {
				stack = append(stack, objectToIRValue(procGte([]coretypes.Object{a.object(), b.object()})))
			}
		case irGt:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i > b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f > b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f > float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) > b.f))
			} else {
				stack = append(stack, objectToIRValue(procGt([]coretypes.Object{a.object(), b.object()})))
			}
		case irLte:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i <= b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f <= b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f <= float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) <= b.f))
			} else {
				stack = append(stack, objectToIRValue(procLte([]coretypes.Object{a.object(), b.object()})))
			}

		case irCursorChar:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			result, ok := runtimeExec.CursorChar(v.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irCursorNext:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			result, ok := runtimeExec.CursorNext(v.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irCursorDone:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag != irValCursor {
				return nil
			}
			result, ok := runtimeExec.CursorDone(v.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irApply:
			argsVal := stack[len(stack)-1]
			fnVal := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			fnObj := fnVal.object()
			argsObj := argsVal.object()
			args, ok := runtimeExec.CallArgs(argsObj)
			if !ok {
				return nil
			}
			result, ok := runtimeExec.CallObject(fnObj, args)
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irThrow:
			v := stack[len(stack)-1]
			runtimeExec.Throw(v.object())

		case irTryCatch:
			pc += 4
			return nil

		case irPop:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

		case irMakeFn:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnExpr, ok := runtimeExec.ProgramFnExpr(prog, idx)
			if !ok {
				return nil
			}
			capturedSlots := make([]coretypes.Object, len(slots))
			for i, v := range slots {
				capturedSlots[i] = v.object()
			}
			fn := runtimeExec.MakeFn(fnExpr, capturedSlots)
			stack = append(stack, objectToIRValue(fn))

		case irBitAnd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i & b.i})
			} else {
				return nil
			}
		case irBitOr:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i | b.i})
			} else {
				return nil
			}
		case irBitNot:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: ^a.i})
			} else {
				return nil
			}
		case irBitShiftLeft:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i << uint(b.i)})
			} else {
				return nil
			}
		case irBitShiftRight:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i >> uint(b.i)})
			} else {
				return nil
			}

		case irCase:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nCases := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			v := slots[slotIdx]
			if v.tag != irValInt {
				pc += nCases * 4
				pc = int(code[pc])<<8 | int(code[pc+1])
				continue
			}
			testVal := v.i
			matched := false
			for i := 0; i < nCases; i++ {
				caseVal := int(int16(code[pc])<<8 | int16(code[pc+1]))
				target := int(code[pc+2])<<8 | int(code[pc+3])
				pc += 4
				if testVal == caseVal {
					pc = target
					matched = true
					break
				}
			}
			if !matched {
				pc = int(code[pc])<<8 | int(code[pc+1])
			}

		case irIsZero:
			v := stack[len(stack)-1]
			if v.tag == irValInt {
				stack[len(stack)-1] = irMakeBool(v.i == 0)
			} else if v.tag == irValDouble {
				stack[len(stack)-1] = irMakeBool(v.f == 0)
			} else {
				stack[len(stack)-1] = objectToIRValue(procIsZero([]coretypes.Object{v.object()}))
			}
		case irEq:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			v, ok := irValueEq(a, b)
			if !ok {
				return nil
			}
			stack = append(stack, v)
		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			cond := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !cond.truthy() {
				pc = target
			}
		case irJump:
			pc = int(code[pc])<<8 | int(code[pc+1])
		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			baseSlot := 0
			if target != 0 {
				baseSlot = int(code[pc])<<8 | int(code[pc+1])
				pc += 2
			}
			for i := nargs - 1; i >= 0; i-- {
				slots[baseSlot+i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			pc = target
			stack = stack[:0]
		case irReturn:
			if len(stack) == 0 {
				if typedFrameStack != nil && typedFrameStack.Depth() > 0 {
					if len(selfTraceStack) > 0 {
						exit := selfTraceStack[len(selfTraceStack)-1]
						selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
						exit()
					}
					var sl int
					pc, sl = typedFrameStack.Pop(slots)
					stack = stack[:sl]
					stack = append(stack, irValue{tag: irValNil})
					continue
				}
				return NIL
			}
			result := stack[len(stack)-1]
			if typedFrameStack != nil && typedFrameStack.Depth() > 0 {
				if len(selfTraceStack) > 0 {
					exit := selfTraceStack[len(selfTraceStack)-1]
					selfTraceStack = selfTraceStack[:len(selfTraceStack)-1]
					exit()
				}
				var sl int
				pc, sl = typedFrameStack.Pop(slots)
				stack = stack[:sl]
				stack = append(stack, result)
				continue
			}
			return result.object()
		case irGet:
			key := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if coll.tag != irValStringIntMap {
				return nil
			}
			k, ok := irValueStringKey(key)
			if !ok {
				return nil
			}
			if v, ok := coll.stringIntMap()[k]; ok {
				stack = append(stack, irValue{tag: irValInt, i: v})
			} else {
				stack = append(stack, irValue{tag: irValNil})
			}
		case irGet3:
			def := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if coll.tag != irValStringIntMap || def.tag != irValInt {
				return nil
			}
			k, ok := irValueStringKey(key)
			if !ok {
				return nil
			}
			if v, ok := coll.stringIntMap()[k]; ok {
				stack = append(stack, irValue{tag: irValInt, i: v})
			} else {
				stack = append(stack, def)
			}
		case irAssoc:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			coll := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if coll.tag == irValStringIntMap && val.tag == irValInt {
				k, ok := irValueStringKey(key)
				if !ok {
					return nil
				}
				if coll.stringIntMap() == nil {
					coll.setStringIntMap(make(map[string]int))
				}
				coll.stringIntMap()[k] = val.i
				stack = append(stack, coll)
			} else if coll.tag == irValIntVector && key.tag == irValInt && val.tag == irValInt {
				if key.i < 0 || key.i > len(coll.intVec()) {
					return nil
				}
				iv := coll.intVec()
				if key.i == len(iv) {
					iv = append(iv, val.i)
				} else {
					iv[key.i] = val.i
				}
				coll.setIntVec(iv)
				stack = append(stack, coll)
			} else {
				// General assoc path for coretypes.Object types (vector of doubles, etc.)
				result, ok := runtimeExec.Assoc(coll.object(), key.object(), val.object())
				if !ok {
					return nil
				}
				stack = append(stack, objectToIRValue(result))
			}
		case irNth:
			idx := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if idx.tag != irValInt || idx.i < 0 {
				return nil
			}
			if coll.tag == irValString {
				if coll.boolean() {
					if idx.i >= len(coll.str()) {
						return nil
					}
					stack = append(stack, irMakeChar(rune(coll.str()[idx.i])))
				} else {
					n := 0
					found := false
					for _, r := range coll.str() {
						if n == idx.i {
							stack = append(stack, irMakeChar(r))
							found = true
							break
						}
						n++
					}
					if !found {
						return nil
					}
				}
			} else if coll.tag == irValIntVector {
				if idx.i >= len(coll.intVec()) {
					return nil
				}
				stack = append(stack, irValue{tag: irValInt, i: coll.intVec()[idx.i]})
			} else if coll.tag == irValObject {
				obj, ok := runtimeExec.Nth(coll.obj(), idx.i)
				if !ok {
					return nil
				}
				stack = append(stack, objectToIRValue(obj))
			} else {
				return nil
			}
		case irNthStringASCII:
			idxConst := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			idx := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if idx.tag != irValInt {
				return nil
			}
			constant, ok := runtimeExec.ProgramConstant(prog, idxConst)
			if !ok {
				return nil
			}
			s := constant.(coretypes.String).S
			if idx.i < 0 || idx.i >= len(s) {
				return nil
			}
			stack = append(stack, irMakeChar(rune(s[idx.i])))
		case irStr1:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValString || a.tag == irValStringBuilder {
				stack = append(stack, a)
			} else {
				stack = append(stack, stringToIRValue(irValueToString(a)))
			}
		case irStr2:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValStringBuilder {
				bs := irValueToString(b)
				abuf := append(a.bytes(), bs...)
				ascii := a.isASCII()
				if ascii {
					for i := 0; i < len(bs); i++ {
						if bs[i] >= utf8.RuneSelf {
							ascii = false
							break
						}
					}
				}
				rc := a.i
				if ascii {
					rc += len(bs)
				} else {
					rc = irStringRuneCount(string(abuf))
				}
				stack = append(stack, irMakeStringBuilder(abuf, rc, ascii))
			} else if b.tag == irValStringBuilder {
				prefix := irValueToString(a)
				if prefix != "" {
					bbuf := b.bytes()
					newBuf := make([]byte, len(prefix)+len(bbuf))
					copy(newBuf, prefix)
					copy(newBuf[len(prefix):], bbuf)
					ascii := b.isASCII()
					if ascii {
						for i := 0; i < len(prefix); i++ {
							if prefix[i] >= utf8.RuneSelf {
								ascii = false
								break
							}
						}
					}
					rc := b.i
					if ascii {
						rc += len(prefix)
					} else {
						rc = irStringRuneCount(string(newBuf))
					}
					b = irMakeStringBuilder(newBuf, rc, ascii)
				}
				stack = append(stack, b)
			} else {
				stack = append(stack, stringToIRValue(irValueToString(a)+irValueToString(b)))
			}
		case irCount:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValString || a.tag == irValStringBuilder {
				stack = append(stack, irValue{tag: irValInt, i: a.i})
			} else if a.tag == irValStringIntMap {
				stack = append(stack, irValue{tag: irValInt, i: len(a.stringIntMap())})
			} else if a.tag == irValIntVector {
				stack = append(stack, irValue{tag: irValInt, i: len(a.intVec())})
			} else if a.tag == irValObject {
				count, ok := runtimeExec.Count(a.obj())
				if !ok {
					return nil
				}
				stack = append(stack, irValue{tag: irValInt, i: count})
			} else {
				return nil
			}
		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			// Load fn from typed slots (supports captures beyond initSlots)
			var fnObj coretypes.Object
			if slotIdx < len(initSlots) {
				fnObj = initSlots[slotIdx]
			} else {
				fnObj = slots[slotIdx].object()
			}
			// Fast path: native f64 closure (zero boxing)
			if fnProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if nativeHelper, ok := runtimeExec.NativeHelper(fnProg); ok {
					// Call native helper with stack-allocated args for common arities.
					var f64buf [4]float64
					var f64args []float64
					if nargs <= len(f64buf) {
						f64args = f64buf[:nargs]
					} else {
						f64args = make([]float64, nargs)
					}
					for i := nargs - 1; i >= 0; i-- {
						v := stack[len(stack)-1]
						stack = stack[:len(stack)-1]
						if v.tag == irValDouble {
							f64args[i] = v.f
						} else if v.tag == irValInt {
							f64args[i] = float64(v.i)
						}
					}
					r := nativeHelper(coreirx.Float64(f64args))
					stack = append(stack, irValue{tag: irValDouble, f: r})
					continue
				}
			}
			// Pop args as irValues (no boxing)
			var typedArgBuf [4]irValue
			var typedArgs []irValue
			if nargs <= 4 {
				typedArgs = typedArgBuf[:nargs]
			} else {
				typedArgs = make([]irValue, nargs)
			}
			for i := nargs - 1; i >= 0; i-- {
				typedArgs[i] = stack[len(stack)-1]
				stack = stack[:len(stack)-1]
			}
			var result coretypes.Object
			if baseProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if baseProg != nil && runtimeExec.HasNativeHelper(baseProg) {
					// Already handled above
				} else if fnProg := runtimeExec.DispatchArityProgram(baseProg, nargs); runtimeExec.CanExecuteIR(fnProg) {
					routedToIR := false
					if runtimeExec.CanExecuteTypedIR(fnProg) {
						// FAST PATH: typed sub-call without coretypes.Object boxing
						// Only for pure numeric programs (no collections/strings)
						subAnalysis := runtimeExec.ProgramAnalysis(fnProg)
						if irTypedEligible(subAnalysis) && !subAnalysis.UsesCollection && !subAnalysis.UsesString && !subAnalysis.HasCallSlot {
							var subBuf [16]irValue
							var subSlots []irValue
							numSlots := runtimeExec.ProgramNumSlots(fnProg)
							if numSlots < nargs {
								return nil
							}
							if numSlots <= 16 {
								subSlots = subBuf[:numSlots]
							} else {
								subSlots = make([]irValue, numSlots)
							}
							copy(subSlots[:nargs], typedArgs)
							// Resolve captures
							if !runtimeExec.InstallFnTypedEnvCaptures(fnObj, fnProg, subSlots) {
								return nil
							}
							// Execute inline with typed slots
							subResult := irExecTypedInline(fnProg, subSlots)
							if subResult.tag != 0 || subResult.i != 0 || subResult.f != 0 {
								stack = append(stack, subResult)
								continue
							}
							runtimeExec.MarkTypedExecutionFailed(fnProg)
						}
					}
					// Fallback: box args
					var argsBuf [4]coretypes.Object
					args := runtimeExec.ObjectsFromTypedValues(typedArgs, argsBuf[:])
					callArgs, ok := runtimeExec.FnCallSlots(fnObj, fnProg, args)
					if !ok {
						return nil
					}
					if r := irExec(fnProg, callArgs); r != nil {
						result = r
						routedToIR = true
					} else {
						runtimeExec.MarkBoxedExecutionFailed(fnProg)
					}
					if !routedToIR && result == nil {
						return nil
					}
				}
				if result == nil {
					var args2 [4]coretypes.Object
					a := runtimeExec.ObjectsFromTypedValues(typedArgs, args2[:])
					var ok bool
					result, ok = runtimeExec.CallObject(fnObj, a)
					if !ok {
						return nil
					}
				}
			} else {
				var args3 [4]coretypes.Object
				a := runtimeExec.ObjectsFromTypedValues(typedArgs, args3[:])
				var ok bool
				result, ok = runtimeExec.CallObject(fnObj, a)
				if !ok {
					return nil
				}
			}
			stack = append(stack, objectToIRValue(result))

		case irSqrt:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValDouble {
				stack = append(stack, irValue{tag: irValDouble, f: math.Sqrt(a.f)})
			} else if a.tag == irValInt {
				stack = append(stack, irValue{tag: irValDouble, f: math.Sqrt(float64(a.i))})
			} else {
				return nil
			}

		case irConj:
			val := stack[len(stack)-1]
			coll := stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if coll.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.Conj(coll.obj(), val.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irCallSelf:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if typedFrameStack == nil {
				typedFrameStack = coreirx.NewFrameStack[irValue](runtimeExec.ProgramNumSlots(prog))
			}
			if typedFrameStack.Depth() < 256 {
				// Save current state and restart
				selfTraceStack = append(selfTraceStack, traceIRProgramCall(prog, nargs))
				typedFrameStack.Push(pc, slots, len(stack)-nargs)
				// Pop args directly into slots (no intermediate copy)
				for i := nargs - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
				// Clear only non-capture working slots
				if !runtimeExec.ClearTypedNonCaptureSlots(prog, slots, nargs) {
					return nil
				}
				pc = 0
			} else {
				// Deep recursion: box args and fall back
				args := make([]coretypes.Object, nargs)
				for i := nargs - 1; i >= 0; i-- {
					args[i] = stack[len(stack)-1].object()
					stack = stack[:len(stack)-1]
				}
				result := irExecTyped(prog, args)
				if result == nil {
					result = irExec(prog, args)
				}
				if result == nil {
					return nil
				}
				stack = append(stack, objectToIRValue(result))
			}

		case irFirst:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.First(a.obj())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irBuildVec:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			arr := make([]coretypes.Object, n)
			for i := n - 1; i >= 0; i-- {
				arr[i] = stack[len(stack)-1].object()
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, irMakeObject(runtimeExec.BuildVector(arr)))

		case irToTransient:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.ToTransient(a.obj())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irAssocBang:
			val := stack[len(stack)-1]
			key := stack[len(stack)-2]
			tv := stack[len(stack)-3]
			stack = stack[:len(stack)-3]
			if tv.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.AssocBang(tv.obj(), key.object(), val.object())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irToPersistent:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValObject {
				return nil
			}
			result, ok := runtimeExec.ToPersistent(a.obj())
			if !ok {
				return nil
			}
			stack = append(stack, objectToIRValue(result))

		case irIntCast:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			switch a.tag {
			case irValChar:
				stack = append(stack, irValue{tag: irValInt, i: int(a.char())})
			case irValInt:
				stack = append(stack, a)
			case irValDouble:
				stack = append(stack, irValue{tag: irValInt, i: int(a.f)})
			default:
				return nil
			}

		case irSubs:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if nargs == 3 {
				ei := stack[len(stack)-1]
				si := stack[len(stack)-2]
				sv := stack[len(stack)-3]
				stack = stack[:len(stack)-3]
				if sv.tag == irValString && si.tag == irValInt {
					s := sv.str()
					start := si.i
					end := ei.i
					if sv.isASCII() {
						stack = append(stack, irMakeString(s[start:end], end-start, true))
					} else {
						runes := []rune(s)
						stack = append(stack, stringToIRValue(string(runes[start:end])))
					}
				} else {
					return nil
				}
			} else {
				si := stack[len(stack)-1]
				sv := stack[len(stack)-2]
				stack = stack[:len(stack)-2]
				if sv.tag == irValString && si.tag == irValInt {
					s := sv.str()
					start := si.i
					if sv.isASCII() {
						stack = append(stack, irMakeString(s[start:], len(s)-start, true))
					} else {
						runes := []rune(s)
						stack = append(stack, stringToIRValue(string(runes[start:])))
					}
				} else {
					return nil
				}
			}

		default:
			return nil
		}
	}
	if len(stack) == 0 {
		return NIL
	}
	return stack[len(stack)-1].object()
}

// irExecTypedIV runs the typed executor and returns the result as irValue
// directly, avoiding the coretypes.Object boxing/unboxing at callSlot boundaries.
// Returns (result, true) on success, (zero, false) on failure.

// ---- typed_exec_inline.go ----
func irExecTypedIV(prog *IRProgram, initSlots []coretypes.Object) (irValue, bool) {
	result := irExecTyped(prog, initSlots)
	if result == nil {
		return irValue{}, false
	}
	return objectToIRValue(result), true
}

// irExecTypedInline executes a typed IR program with pre-filled irValue slots.
// Returns the result as irValue directly (no coretypes.Object boxing).
// Returns zero irValue on failure.
func irExecTypedInline(prog *IRProgram, slots []irValue) irValue {
	var stackBuf [32]irValue
	stack := stackBuf[:0]
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	for pc < len(code) {
		op := code[pc]
		pc++
		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			constant, ok := runtimeExec.ProgramConstant(prog, idx)
			if !ok {
				return irValue{}
			}
			stack = append(stack, objectToIRValue(constant))
		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stack = append(stack, slots[idx])
		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			slots[idx] = stack[len(stack)-1]
			stack = stack[:len(stack)-1]
		case irAdd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				r := a.i + b.i
				if b.i > 0 && r < a.i || b.i < 0 && r > a.i {
					stack = append(stack, objectToIRValue(procAdd([]coretypes.Object{a.object(), b.object()})))
					continue
				} else {
					stack = append(stack, irValue{tag: irValInt, i: r})
				}
			} else {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					stack = append(stack, objectToIRValue(procAdd([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					stack = append(stack, objectToIRValue(procAdd([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				stack = append(stack, irValue{tag: irValDouble, f: af + bf})
			}
		case irSub:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				r := a.i - b.i
				if b.i < 0 && r < a.i || b.i > 0 && r > a.i {
					stack = append(stack, objectToIRValue(procSubtract([]coretypes.Object{a.object(), b.object()})))
					continue
				} else {
					stack = append(stack, irValue{tag: irValInt, i: r})
				}
			} else if a.tag == irValDouble || b.tag == irValDouble {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					stack = append(stack, objectToIRValue(procSubtract([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					stack = append(stack, objectToIRValue(procSubtract([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				stack = append(stack, irValue{tag: irValDouble, f: af - bf})
			} else {
				stack = append(stack, objectToIRValue(procSubtract([]coretypes.Object{a.object(), b.object()})))
				continue
			}
		case irMul:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				r := a.i * b.i
				if a.i != 0 && ((a.i == -1 && b.i == coretypes.MinInt) || (b.i == -1 && a.i == coretypes.MinInt) || r/a.i != b.i) {
					stack = append(stack, objectToIRValue(procMultiply([]coretypes.Object{a.object(), b.object()})))
					continue
				} else {
					stack = append(stack, irValue{tag: irValInt, i: r})
				}
			} else if a.tag == irValDouble || b.tag == irValDouble {
				af, bf := 0.0, 0.0
				if a.tag == irValDouble {
					af = a.f
				} else if a.tag == irValInt {
					af = float64(a.i)
				} else {
					stack = append(stack, objectToIRValue(procMultiply([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				if b.tag == irValDouble {
					bf = b.f
				} else if b.tag == irValInt {
					bf = float64(b.i)
				} else {
					stack = append(stack, objectToIRValue(procMultiply([]coretypes.Object{a.object(), b.object()})))
					continue
				}
				stack = append(stack, irValue{tag: irValDouble, f: af * bf})
			} else {
				stack = append(stack, objectToIRValue(procMultiply([]coretypes.Object{a.object(), b.object()})))
				continue
			}
		case irRem:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-1]
			stack[len(stack)-1] = objectToIRValue(procRem([]coretypes.Object{a.object(), b.object()}))
		case irDiv:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-1]
			if a.tag == irValDouble && b.tag == irValDouble {
				stack[len(stack)-1] = irValue{tag: irValDouble, f: a.f / b.f}
			} else {
				stack[len(stack)-1] = objectToIRValue(procDivide([]coretypes.Object{a.object(), b.object()}))
			}

		case irLt:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.i < b.i))
			} else if a.tag == irValDouble && b.tag == irValDouble {
				stack = append(stack, irMakeBool(a.f < b.f))
			} else if a.tag == irValDouble && b.tag == irValInt {
				stack = append(stack, irMakeBool(a.f < float64(b.i)))
			} else if a.tag == irValInt && b.tag == irValDouble {
				stack = append(stack, irMakeBool(float64(a.i) < b.f))
			} else {
				stack = append(stack, objectToIRValue(procLt([]coretypes.Object{a.object(), b.object()})))
			}
		case irEq:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			v, ok := irValueEq(a, b)
			if !ok {
				return irValue{}
			}
			stack = append(stack, v)
		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			cond := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if !cond.truthy() {
				pc = target
			}
		case irJump:
			pc = int(code[pc])<<8 | int(code[pc+1])
		case irReturn:
			if len(stack) == 0 {
				return irValue{}
			}
			return stack[len(stack)-1]
		case irRecur:
			n := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			targetPC := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if targetPC == 0 {
				for i := n - 1; i >= 0; i-- {
					slots[i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			} else {
				baseSlot := int(code[pc])<<8 | int(code[pc+1])
				pc += 2
				for i := n - 1; i >= 0; i-- {
					slots[baseSlot+i] = stack[len(stack)-1]
					stack = stack[:len(stack)-1]
				}
			}
			pc = targetPC
			stack = stack[:0]
		case irInc:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValInt || a.i == coretypes.MaxInt {
				stack = append(stack, objectToIRValue(procInc([]coretypes.Object{a.object()})))
				continue
			}
			stack = append(stack, irValue{tag: irValInt, i: a.i + 1})
		case irDec:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag != irValInt || a.i == coretypes.MinInt {
				stack = append(stack, objectToIRValue(procDec([]coretypes.Object{a.object()})))
				continue
			}
			stack = append(stack, irValue{tag: irValInt, i: a.i - 1})
		case irIsZero:
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if v.tag == irValInt {
				stack = append(stack, irMakeBool(v.i == 0))
			} else {
				stack = append(stack, objectToIRValue(procIsZero([]coretypes.Object{v.object()})))
				continue
			}
		case irBitAnd:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i & b.i})
			} else {
				return irValue{}
			}
		case irBitOr:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i | b.i})
			} else {
				return irValue{}
			}
		case irBitNot:
			a := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if a.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: ^a.i})
			} else {
				return irValue{}
			}
		case irBitShiftLeft:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i << uint(b.i)})
			} else {
				return irValue{}
			}
		case irBitShiftRight:
			b, a := stack[len(stack)-1], stack[len(stack)-2]
			stack = stack[:len(stack)-2]
			if a.tag == irValInt && b.tag == irValInt {
				stack = append(stack, irValue{tag: irValInt, i: a.i >> uint(b.i)})
			} else {
				return irValue{}
			}
		default:
			return irValue{} // unsupported opcode — bail
		}
	}
	return irValue{}
}

// ---- typed_exec_nanbox.go ----
// ir_exec_typed_nb.go — NaN-boxed typed IR executor.
//
// Uses []uint64 stack (8 bytes per entry) instead of []irValue (32 bytes).
// Numeric operations are pure bit manipulation — zero allocation, zero copy.
// coretypes.Object operations convert at the boundary via the local nb* helpers.
//
// This is the typed executor's hot path for numeric loops.
// Falls back to nil (letting irExecTyped handle it) for unsupported patterns.

func irExecTypedNB(prog *IRProgram, initSlots []coretypes.Object) coretypes.Object {
	analysis := AnalyzeIRProgram(prog)
	// Only handle numeric-dominant programs without complex collection ops
	if !irTypedEligible(analysis) {
		return nil
	}
	// Only handle pure numeric programs — no collections, no self-calls,
	// no strings, no fn calls (which allocate []coretypes.Object args).
	if analysis.HasSelfCall || analysis.UsesString || analysis.UsesTransient ||
		analysis.UsesCollection || analysis.HasCallSlot {
		return nil
	}

	numSlots := runtimeExec.ProgramNumSlots(prog)
	var slotBuf [16]uint64
	var slots []uint64
	if numSlots <= len(slotBuf) {
		slots = slotBuf[:numSlots]
	} else {
		slots = make([]uint64, numSlots)
	}

	// coretypes.Object side-table for non-numeric values
	var objTable []coretypes.Object

	// Convert init slots
	for i := 0; i < numSlots && i < len(initSlots); i++ {
		slots[i] = coreirx.NBFromObject(initSlots[i], &objTable, corert.IsNil)
	}
	// Pre-fill captures
	captureIdxs, captureSlots := runtimeExec.ProgramCaptureSlots(prog)
	for i, obj := range captureSlots {
		if i >= len(captureIdxs) || captureIdxs[i] < 0 || captureIdxs[i] >= len(slots) {
			return nil
		}
		slots[captureIdxs[i]] = coreirx.NBFromObject(obj, &objTable, corert.IsNil)
	}

	// Pre-convert constants
	constants := runtimeExec.ProgramConstants(prog)
	consts := make([]uint64, len(constants))
	for i, c := range constants {
		consts[i] = coreirx.NBFromObject(c, &objTable, corert.IsNil)
	}

	var stackBuf [32]uint64
	sp := 0
	code := runtimeExec.ProgramCode(prog)
	pc := 0

	for pc < len(code) {
		op := code[pc]
		pc++

		switch op {
		case irLiteral:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stackBuf[sp] = consts[idx]
			sp++

		case irLoadSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			stackBuf[sp] = slots[idx]
			sp++

		case irStoreSlot:
			idx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			sp--
			slots[idx] = stackBuf[sp]

		case irAdd:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.NBFromObject(coretypes.MakeInt(coreirx.ToInt(a)+coreirx.ToInt(b)), &objTable, corert.IsNil)
			} else if coreirx.IsObj(a) || coreirx.IsObj(b) {
				stackBuf[sp-1] = coreirx.NBFromObject(procAdd([]coretypes.Object{coreirx.NBToObject(a, objTable, NIL), coreirx.NBToObject(b, objTable, NIL)}), &objTable, corert.IsNil)
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(a) + coreirx.ToFloat(b))
			}

		case irSub:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.NBFromObject(coretypes.MakeInt(coreirx.ToInt(a)-coreirx.ToInt(b)), &objTable, corert.IsNil)
			} else if coreirx.IsObj(a) || coreirx.IsObj(b) {
				stackBuf[sp-1] = coreirx.NBFromObject(procSubtract([]coretypes.Object{coreirx.NBToObject(a, objTable, NIL), coreirx.NBToObject(b, objTable, NIL)}), &objTable, corert.IsNil)
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(a) - coreirx.ToFloat(b))
			}

		case irMul:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.NBFromObject(coretypes.MakeInt(coreirx.ToInt(a)*coreirx.ToInt(b)), &objTable, corert.IsNil)
			} else if coreirx.IsObj(a) || coreirx.IsObj(b) {
				stackBuf[sp-1] = coreirx.NBFromObject(procMultiply([]coretypes.Object{coreirx.NBToObject(a, objTable, NIL), coreirx.NBToObject(b, objTable, NIL)}), &objTable, corert.IsNil)
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(a) * coreirx.ToFloat(b))
			}

		case irDiv:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsDouble(a) && coreirx.IsDouble(b) {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToDouble(a) / coreirx.ToDouble(b))
			} else {
				stackBuf[sp-1] = coreirx.NBFromObject(procDivide([]coretypes.Object{coreirx.NBToObject(a, objTable, NIL), coreirx.NBToObject(b, objTable, NIL)}), &objTable, corert.IsNil)
			}

		case irRem:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			stackBuf[sp-1] = coreirx.NBFromObject(procRem([]coretypes.Object{coreirx.NBToObject(a, objTable, NIL), coreirx.NBToObject(b, objTable, NIL)}), &objTable, corert.IsNil)

		case irSqrt:
			stackBuf[sp-1] = coreirx.BoxDouble(math.Sqrt(coreirx.ToFloat(stackBuf[sp-1])))

		case irInc:
			v := stackBuf[sp-1]
			if coreirx.IsInt(v) {
				stackBuf[sp-1] = coreirx.NBFromObject(coretypes.MakeInt(coreirx.ToInt(v)+1), &objTable, corert.IsNil)
			} else if coreirx.IsObj(v) {
				stackBuf[sp-1] = coreirx.NBFromObject(procInc([]coretypes.Object{coreirx.NBToObject(v, objTable, NIL)}), &objTable, corert.IsNil)
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(v) + 1)
			}

		case irDec:
			v := stackBuf[sp-1]
			if coreirx.IsInt(v) {
				stackBuf[sp-1] = coreirx.NBFromObject(coretypes.MakeInt(coreirx.ToInt(v)-1), &objTable, corert.IsNil)
			} else if coreirx.IsObj(v) {
				stackBuf[sp-1] = coreirx.NBFromObject(procDec([]coretypes.Object{coreirx.NBToObject(v, objTable, NIL)}), &objTable, corert.IsNil)
			} else {
				stackBuf[sp-1] = coreirx.BoxDouble(coreirx.ToFloat(v) - 1)
			}

		case irLt:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) < coreirx.ToInt(b))
			} else if coreirx.IsObj(a) || coreirx.IsObj(b) {
				stackBuf[sp-1] = coreirx.NBFromObject(procLt([]coretypes.Object{coreirx.NBToObject(a, objTable, NIL), coreirx.NBToObject(b, objTable, NIL)}), &objTable, corert.IsNil)
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) < coreirx.ToFloat(b))
			}

		case irGte:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) >= coreirx.ToInt(b))
			} else if coreirx.IsObj(a) || coreirx.IsObj(b) {
				stackBuf[sp-1] = coreirx.NBFromObject(procGte([]coretypes.Object{coreirx.NBToObject(a, objTable, NIL), coreirx.NBToObject(b, objTable, NIL)}), &objTable, corert.IsNil)
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) >= coreirx.ToFloat(b))
			}

		case irGt:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) > coreirx.ToInt(b))
			} else if coreirx.IsObj(a) || coreirx.IsObj(b) {
				stackBuf[sp-1] = coreirx.NBFromObject(procGt([]coretypes.Object{coreirx.NBToObject(a, objTable, NIL), coreirx.NBToObject(b, objTable, NIL)}), &objTable, corert.IsNil)
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) > coreirx.ToFloat(b))
			}

		case irLte:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(a) <= coreirx.ToInt(b))
			} else if coreirx.IsObj(a) || coreirx.IsObj(b) {
				stackBuf[sp-1] = coreirx.NBFromObject(procLte([]coretypes.Object{coreirx.NBToObject(a, objTable, NIL), coreirx.NBToObject(b, objTable, NIL)}), &objTable, corert.IsNil)
			} else {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) <= coreirx.ToFloat(b))
			}

		case irEq:
			sp--
			a, b := stackBuf[sp-1], stackBuf[sp]
			if a == b && !(coreirx.IsDouble(a) && math.IsNaN(coreirx.ToDouble(a))) {
				stackBuf[sp-1] = coreirx.BoxBool(true)
			} else if coreirx.IsInt(a) && coreirx.IsInt(b) {
				stackBuf[sp-1] = coreirx.BoxBool(false)
			} else if (coreirx.IsDouble(a) || coreirx.IsInt(a)) && (coreirx.IsDouble(b) || coreirx.IsInt(b)) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToFloat(a) == coreirx.ToFloat(b))
			} else {
				oa := coreirx.NBToObject(a, objTable, NIL)
				ob := coreirx.NBToObject(b, objTable, NIL)
				stackBuf[sp-1] = coreirx.BoxBool(runtimeExec.Equal(oa, ob))
			}

		case irIsZero:
			v := stackBuf[sp-1]
			if coreirx.IsInt(v) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToInt(v) == 0)
			} else if coreirx.IsDouble(v) {
				stackBuf[sp-1] = coreirx.BoxBool(coreirx.ToDouble(v) == 0)
			} else {
				stackBuf[sp-1] = coreirx.NBFromObject(procIsZero([]coretypes.Object{coreirx.NBToObject(v, objTable, NIL)}), &objTable, corert.IsNil)
			}

		case irJumpIfNot:
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			sp--
			if !coreirx.Truthy(stackBuf[sp]) {
				pc = target
			}

		case irJump:
			pc = int(code[pc])<<8 | int(code[pc+1])

		case irReturn:
			if sp == 0 {
				return NIL
			}
			sp--
			return coreirx.NBToObject(stackBuf[sp], objTable, NIL)

		case irRecur:
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			target := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			if target != 0 {
				baseSlot := int(code[pc])<<8 | int(code[pc+1])
				pc += 2
				for i := nargs - 1; i >= 0; i-- {
					sp--
					slots[baseSlot+i] = stackBuf[sp]
				}
			} else {
				for i := nargs - 1; i >= 0; i-- {
					sp--
					slots[i] = stackBuf[sp]
				}
			}
			sp = 0
			pc = target

		// coretypes.Collection ops: convert at boundary
		case irNth:
			sp -= 2
			coll := coreirx.NBToObject(stackBuf[sp], objTable, NIL)
			idxV := stackBuf[sp+1]
			var idx int
			if coreirx.IsInt(idxV) {
				idx = coreirx.ToInt(idxV)
			} else {
				idx = int(coreirx.ToFloat(idxV))
			}
			obj, ok := runtimeExec.Nth(coll, idx)
			if !ok {
				return nil
			}
			stackBuf[sp] = coreirx.NBFromObject(obj, &objTable, corert.IsNil)
			sp++

		case irCallSlot:
			slotIdx := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			nargs := int(code[pc])<<8 | int(code[pc+1])
			pc += 2
			fnObj := coreirx.NBToObject(slots[slotIdx], objTable, NIL)
			// coretypes.Native f64 fast path
			if fnProg, ok := runtimeExec.FnProgram(fnObj); ok {
				if nativeHelper, ok := runtimeExec.NativeHelper(fnProg); ok {
					var f64buf [4]float64
					var f64args []float64
					if nargs <= len(f64buf) {
						f64args = f64buf[:nargs]
					} else {
						f64args = make([]float64, nargs)
					}
					for i := nargs - 1; i >= 0; i-- {
						sp--
						f64args[i] = coreirx.ToFloat(stackBuf[sp])
					}
					stackBuf[sp] = coreirx.BoxDouble(nativeHelper(coreirx.Float64(f64args)))
					sp++
					continue
				}
			}
			// corecollections.Box args and call
			args := make([]coretypes.Object, nargs)
			for i := nargs - 1; i >= 0; i-- {
				sp--
				args[i] = coreirx.NBToObject(stackBuf[sp], objTable, NIL)
			}
			var result coretypes.Object
			if fnProg, ok := runtimeExec.CompileFnProgram(fnObj); ok {
				result = irExecTyped(fnProg, args)
				if result == nil {
					result = irExec(fnProg, args)
				}
				if result == nil {
					var ok bool
					result, ok = runtimeExec.CallObject(fnObj, args)
					if !ok {
						return nil
					}
				}
			} else {
				var ok bool
				result, ok = runtimeExec.CallObject(fnObj, args)
				if !ok {
					return nil
				}
			}
			stackBuf[sp] = coreirx.NBFromObject(result, &objTable, corert.IsNil)
			sp++

		case irConj:
			sp -= 2
			coll := coreirx.NBToObject(stackBuf[sp], objTable, NIL)
			val := coreirx.NBToObject(stackBuf[sp+1], objTable, NIL)
			result, ok := runtimeExec.Conj(coll, val)
			if !ok {
				return nil
			}
			stackBuf[sp] = coreirx.NBFromObject(result, &objTable, corert.IsNil)
			sp++

		case irCount:
			sp--
			v := stackBuf[sp]
			if !coreirx.IsObj(v) {
				return nil
			}
			count, ok := runtimeExec.Count(coreirx.NBToObject(v, objTable, NIL))
			if !ok {
				return nil
			}
			stackBuf[sp] = coreirx.BoxInt(count)
			sp++

		default:
			return nil // unsupported — fall back to irExecTyped
		}
	}

	if sp > 0 {
		return coreirx.NBToObject(stackBuf[sp-1], objTable, NIL)
	}
	return NIL
}

// ---- typed_value_accessors.go ----
// ir_value_accessors.go — typed accessors for irValue's unsafe.Pointer field.
//
// irValue stores extended data (strings, collections, objects) behind an
// unsafe.Pointer to keep the struct at 32 bytes for the numeric hot path.
// These accessors provide type-safe reads/writes.

// --- String ---

func irMakeString(s string, runeCount int, ascii bool) irValue {
	v := irValue{tag: irValString, i: runeCount, p: unsafe.Pointer(&s)}
	if ascii {
		v.f = 1
	}
	return v
}

func (v irValue) str() string {
	if v.p == nil {
		return ""
	}
	return *(*string)(v.p)
}

func (v irValue) isASCII() bool { return v.f != 0 }

// --- StringBuilder ([]byte) ---

func irMakeStringBuilder(buf []byte, runeCount int, ascii bool) irValue {
	v := irValue{tag: irValStringBuilder, i: runeCount, p: unsafe.Pointer(&buf)}
	if ascii {
		v.f = 1
	}
	return v
}

func (v irValue) bytes() []byte {
	if v.p == nil {
		return nil
	}
	return *(*[]byte)(v.p)
}

func (v *irValue) setBytes(buf []byte) {
	v.p = unsafe.Pointer(&buf)
}

func (v *irValue) setASCII(ascii bool) {
	if ascii {
		v.f = 1
	} else {
		v.f = 0
	}
}

// --- Bool ---

func irMakeBool(b bool) irValue {
	v := irValue{tag: irValBool}
	if b {
		v.i = 1
	}
	return v
}

func (v irValue) boolean() bool { return v.i != 0 }

// --- Char ---

func irMakeChar(r rune) irValue {
	return irValue{tag: irValChar, i: int(r)}
}

func (v irValue) char() rune { return rune(v.i) }

// --- StringIntMap ---

func irMakeStringIntMap(m map[string]int) irValue {
	return irValue{tag: irValStringIntMap, p: unsafe.Pointer(&m)}
}

func (v irValue) stringIntMap() map[string]int {
	if v.p == nil {
		return nil
	}
	return *(*map[string]int)(v.p)
}

func (v *irValue) setStringIntMap(m map[string]int) {
	v.p = unsafe.Pointer(&m)
}

// --- IntVector ---

func irMakeIntVector(iv []int) irValue {
	return irValue{tag: irValIntVector, p: unsafe.Pointer(&iv)}
}

func (v irValue) intVec() []int {
	if v.p == nil {
		return nil
	}
	return *(*[]int)(v.p)
}

func (v *irValue) setIntVec(iv []int) {
	v.p = unsafe.Pointer(&iv)
}

// --- coretypes.Object ---

func irMakeObject(obj coretypes.Object) irValue {
	// For common concrete pointer types, store directly to avoid
	// allocating an coretypes.Object interface box. Use i field as sub-tag.
	switch v := obj.(type) {
	case *corecollections.ArrayVector:
		return irValue{tag: irValObject, i: 1, p: unsafe.Pointer(v)}
	case *coretypes.TransientVector:
		return irValue{tag: irValObject, i: 2, p: unsafe.Pointer(v)}
	case *Fn:
		return irValue{tag: irValObject, i: 3, p: unsafe.Pointer(v)}
	default:
		p := new(coretypes.Object)
		*p = obj
		return irValue{tag: irValObject, i: 0, p: unsafe.Pointer(p)}
	}
}

func (v irValue) obj() coretypes.Object {
	if v.p == nil {
		return NIL
	}
	switch v.i {
	case 1:
		return (*corecollections.ArrayVector)(v.p)
	case 2:
		return (*coretypes.TransientVector)(v.p)
	case 3:
		return (*Fn)(v.p)
	default:
		return *(*coretypes.Object)(v.p)
	}
}

// ---- typed_values.go ----
// ir_typed.go — experimental typed IR executor (v2).
//
// This is the first incremental step away from the boxed []coretypes.Object stack used by
// irExec. It is intentionally small and gated: primitive/string-only loops can
// be executed with tagged values, while unsupported opcodes return nil and let
// the normal IR/tree path handle them.

type irValueTag byte

const (
	irValObject irValueTag = iota
	irValInt
	irValDouble
	irValBool
	irValChar
	irValString
	irValStringBuilder
	irValStringIntMap
	irValIntVector
	irValNil
	irValKeyword
	irValCursor // StringCursor pointer in obj field
)

// irValue is the tagged value for the typed IR executor.
// Layout: 32 bytes for the compact numeric path.
// String/collection data is stored behind an unsafe.Pointer to avoid
// bloating the struct for the common numeric case.
type irValue struct {
	tag irValueTag
	i   int            // int value, bool (0/1), rune, rune count for strings
	f   float64        // double value, or ASCII flag (nonzero = ASCII) for strings
	p   unsafe.Pointer // -> string | []byte | map[string]int | []int | coretypes.Object
}

func irTypedEligible(a coreirx.Analysis) bool {
	if a.NumOps == 0 || a.UsesTransient {
		return false
	}
	// Call-slot loops: allow if numeric-only or numeric+generic-nth
	if a.HasCallSlot {
		return !a.UsesString && !a.HasMapOps && (!a.UsesCollection || a.HasGenericNth)
	}
	// coretypes.Collection programs with nth but NO assoc (read-only vector access)
	if a.UsesCollection && a.HasGenericNth && !a.HasMapOps && !a.UsesString && !a.HasAssoc {
		return true
	}
	// coretypes.Collection programs with assoc: prefer boxed executor (has transient support)
	if a.UsesCollection && a.HasGenericNth && a.HasAssoc && !a.HasMapOps && !a.UsesString {
		return false
	}
	if a.UsesCollection && (a.HasMapOps || !a.HasGenericNth) {
		if corert.IRTypedMapEnabled() && a.HasMapOps && a.UsesString {
			return true
		}
		// Self-recursive tree builders/walkers (binary-trees pattern)
		if a.HasSelfCall && !a.HasMapOps && !a.UsesString {
			return true
		}
		return corert.IRTypedVecEnabled() && a.UsesCollection && !a.UsesString && !a.HasMapOps
	}
	// Accept: pure numeric loops (no strings, no collections, no call-slots)
	if !a.UsesString && !a.UsesCollection && !a.HasCallSlot {
		return true
	}
	return a.UsesString || a.SuggestedPath == "typed-ir-string-candidate" || a.SuggestedPath == "typed-ir-generic-string-nth-candidate"
}

func stringToIRValue(s string) irValue {
	ascii := true
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			ascii = false
			return irMakeString(s, utf8.RuneCountInString(s), false)
		}
	}
	return irMakeString(s, len(s), ascii)
}

func objectToIRValue(obj coretypes.Object) irValue {
	switch v := obj.(type) {
	case coretypes.Int:
		return irValue{tag: irValInt, i: v.I}
	case coretypes.Double:
		return irValue{tag: irValDouble, f: v.D}
	case coretypes.Boolean:
		return irMakeBool(v.B)
	case coretypes.Char:
		return irMakeChar(v.Ch)
	case coretypes.String:
		return stringToIRValue(v.S)
	case *corecollections.ArrayVector:
		if corert.IRTypedVecEnabled() {
			iv := make([]int, len(v.Arr))
			for i, item := range v.Arr {
				x, ok := item.(coretypes.Int)
				if !ok {
					return irMakeObject(obj)
				}
				iv[i] = x.I
			}
			return irMakeIntVector(iv)
		}
	case *corecollections.ArrayMap:
		if v.Count() == 0 {
			return irMakeStringIntMap(make(map[string]int))
		}
	case *corecollections.HashMap:
		if v.Count() == 0 {
			return irMakeStringIntMap(make(map[string]int))
		}
	case Nil:
		return irValue{tag: irValNil}
	case coretypes.Keyword:
		return irValue{tag: irValKeyword, p: unsafe.Pointer(v.NameKey())}
	case *coretypes.StringCursor:
		return irValue{tag: irValCursor, p: unsafe.Pointer(v)}
	default:
		return irMakeObject(obj)
	}
	return irMakeObject(obj)
}

func (v irValue) object() coretypes.Object {
	switch v.tag {
	case irValInt:
		return coretypes.Int{I: v.i}
	case irValDouble:
		return coretypes.Double{D: v.f}
	case irValBool:
		return coretypes.Boolean{B: v.boolean()}
	case irValChar:
		return coretypes.Char{Ch: v.char()}
	case irValString:
		return coretypes.String{S: v.str()}
	case irValStringBuilder:
		return coretypes.String{S: string(v.bytes())}
	case irValStringIntMap:
		res := corecollections.EmptyArrayMap()
		for k, v := range v.stringIntMap() {
			res.Add(coretypes.String{S: k}, coretypes.Int{I: v})
		}
		return res
	case irValIntVector:
		arr := make([]coretypes.Object, len(v.intVec()))
		for i, x := range v.intVec() {
			arr[i] = coretypes.Int{I: x}
		}
		return runtimeExec.BuildVector(arr)
	case irValNil:
		return NIL
	case irValKeyword:
		return keywordObjectFromName((*string)(v.p))
	case irValCursor:
		return (*coretypes.StringCursor)(v.p)
	default:
		if v.obj() == nil {
			return NIL
		}
		return v.obj()
	}
}

func (v irValue) truthy() bool {
	switch v.tag {
	case irValBool:
		return v.boolean()
	case irValNil:
		return false
	default:
		return true
	}
}

func irValueToString(v irValue) string {
	switch v.tag {
	case irValString:
		return v.str()
	case irValStringBuilder:
		return string(v.bytes())
	case irValChar:
		return corestr.CharToStringFast(v.char())
	case irValNil:
		return ""
	case irValInt:
		return strconv.Itoa(v.i)
	case irValDouble:
		return strconv.FormatFloat(v.f, 'g', -1, 64)
	case irValBool:
		if v.boolean() {
			return "true"
		}
		return "false"
	default:
		return v.object().ToString(false)
	}
}

func irValueStringKey(v irValue) (string, bool) {
	switch v.tag {
	case irValString:
		return v.str(), true
	case irValStringBuilder:
		return string(v.bytes()), true
	case irValChar:
		return corestr.CharToStringFast(v.char()), true
	default:
		return "", false
	}
}

func irStringRuneCount(s string) int {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return utf8.RuneCountInString(s)
		}
	}
	return len(s)
}

func irValueEq(a, b irValue) (irValue, bool) {
	if a.tag == b.tag {
		switch a.tag {
		case irValInt:
			return irMakeBool(a.i == b.i), true
		case irValDouble:
			return irMakeBool(a.f == b.f), true
		case irValBool:
			return irMakeBool(a.boolean() == b.boolean()), true
		case irValChar:
			return irMakeBool(a.char() == b.char()), true
		case irValString:
			return irMakeBool(a.str() == b.str()), true
		case irValStringBuilder:
			return irMakeBool(string(a.bytes()) == string(b.bytes())), true
		case irValNil:
			return irMakeBool(true), true
		case irValKeyword:
			// Keywords are interned — pointer equality on name
			return irMakeBool(a.p == b.p), true
		}
	}
	if a.tag == irValInt && b.tag == irValDouble {
		return irMakeBool(float64(a.i) == b.f), true
	}
	if a.tag == irValDouble && b.tag == irValInt {
		return irMakeBool(a.f == float64(b.i)), true
	}
	return irMakeBool(runtimeExec.Equal(a.object(), b.object())), true
}

// keywordObjectCache caches Keyword Objects by name pointer to avoid
// repeated heap allocation when converting irValKeyword → coretypes.Object.
var keywordObjectCache sync.Map // *string → coretypes.Object (Keyword)

func keywordObjectFromName(name *string) coretypes.Object {
	if v, ok := keywordObjectCache.Load(name); ok {
		return v.(coretypes.Object)
	}
	kw := coretypes.MakeKeywordFromKeys(nil, name)
	// Store as coretypes.Object interface to avoid re-boxing
	var obj coretypes.Object = kw
	keywordObjectCache.Store(name, obj)
	return obj
}

// ---- wasm_exec_runtime.go ----
// wasm_runtime.go — wazero-based WASM execution engine.
// Compiles WASM modules and caches them. Handles coretypes.Object ↔ WASM i64 conversion.

// WasmProgram is a compiled, ready-to-execute WASM module.
type WasmProgram struct {
	recovery   *IRProgram
	mod        api.Module
	execFn     api.Function
	useFloat   bool
	hasImports bool
	constants  []coretypes.Object // pre-stored constants for handle references
	bytes      []byte             // raw wasm module for export/debugging
}

var (
	wasmRT     wazero.Runtime
	wasmRTOnce sync.Once
	wasmCache  sync.Map // map[*IRProgram]*WasmProgram
	wasmFail   = &WasmProgram{}
)

func getWasmRT() wazero.Runtime {
	wasmRTOnce.Do(func() {
		cache := wazero.NewCompilationCache()
		wasmRT = wazero.NewRuntimeWithConfig(context.Background(),
			wazero.NewRuntimeConfig().WithCompilationCache(cache))
		// Register host functions for collection operations
		registerWasmHost(wasmRT)
	})
	return wasmRT
}

// wasmGetCached retrieves or compiles a WASM program for an IR program.
func wasmGetCached(prog *IRProgram) *WasmProgram {
	if v, ok := wasmCache.Load(prog); ok {
		wp := v.(*WasmProgram)
		if wp == wasmFail {
			return nil
		}
		return wp
	}
	wp := wasmCompile(prog)
	if wp == nil {
		wasmCache.Store(prog, wasmFail)
		return nil
	}
	wasmCache.Store(prog, wp)
	return wp
}

func wasmExec(wp *WasmProgram, slots []coretypes.Object) coretypes.Object {
	// This backend assigns one numeric type to all slots. Even small integer
	// inputs can produce inexact integer intermediates in a float module.
	// Decline such calls before execution until per-value typing is available;
	// pure modules can run the same IR without replaying any effects.
	if wp.useFloat {
		for _, slot := range slots {
			if _, ok := slot.(coretypes.Int); ok {
				if wp.recovery != nil && !wp.hasImports {
					return irExec(wp.recovery, slots)
				}
				return nil
			}
		}
	}
	if !wp.hasImports {
		numericOnly := true
		for _, s := range slots {
			switch s.(type) {
			case coretypes.Int, coretypes.Double:
			default:
				numericOnly = false
			}
		}
		if numericOnly {
			var stackBuf [16]uint64
			var stack []uint64
			if len(slots) <= len(stackBuf) {
				stack = stackBuf[:len(slots)]
			} else {
				stack = make([]uint64, len(slots))
			}
			for i, s := range slots {
				switch v := s.(type) {
				case coretypes.Int:
					if wp.useFloat {
						stack[i] = math.Float64bits(float64(v.I))
					} else {
						stack[i] = uint64(v.I)
					}
				case coretypes.Double:
					if wp.useFloat {
						stack[i] = math.Float64bits(v.D)
					} else {
						return nil
					}
				default:
					return nil
				}
			}
			if err := wp.execFn.CallWithStack(context.Background(), stack); err != nil {
				if wp.recovery != nil && !wp.hasImports && (strings.Contains(err.Error(), "unreachable") || strings.Contains(err.Error(), "integer overflow")) {
					return irExec(wp.recovery, slots)
				}
				return nil
			}
			r := stack[0]
			if wp.useFloat {
				return coretypes.Double{D: math.Float64frombits(r)}
			}
			return corewasm.RawIntObject(r)
		}
	}

	// Create object table for this execution
	table := corewasm.NewObjectTable(NIL)

	// Pre-populate with IR program constants (for handle references)
	if wp.hasImports && len(wp.constants) > 0 {
		for _, c := range wp.constants {
			table.Store(c)
		}
	}

	var stackBuf [16]uint64
	var stack []uint64
	if len(slots) <= len(stackBuf) {
		stack = stackBuf[:len(slots)]
	} else {
		stack = make([]uint64, len(slots))
	}
	for i, s := range slots {
		switch v := s.(type) {
		case coretypes.Int:
			if wp.useFloat {
				stack[i] = math.Float64bits(float64(v.I))
			} else {
				stack[i] = uint64(v.I)
			}
		case coretypes.Double:
			if wp.useFloat {
				stack[i] = math.Float64bits(v.D)
			} else {
				return nil
			}
		default:
			stack[i] = table.Store(s)
		}
	}

	ctx := corewasm.WithObjectTable(context.Background(), table)
	if err := wp.execFn.CallWithStack(ctx, stack); err != nil {
		return nil
	}

	r := stack[0]
	if wp.useFloat {
		return coretypes.Double{D: math.Float64frombits(r)}
	}
	// Check if result is a handle
	if corewasm.IsHandle(r) {
		return table.Load(r)
	}
	return corewasm.RawIntObject(r)
}

// Ensure math import is used
var _ = math.Float64bits

// ---- runtime_execution_contract.go ----

// RuntimeExecutionAdapter is the narrow root-owned runtime surface that future
// extracted IR executors should target instead of reaching through all of core.
// It is intentionally small and grows only when contract tests justify a new
// operation.
type RuntimeExecutionAdapter struct{}

var runtimeExec RuntimeExecutionAdapter

func (RuntimeExecutionAdapter) Errorf(format string, args ...any) coretypes.Error {
	return RT.NewError(fmt.Sprintf(format, args...))
}

func (r RuntimeExecutionAdapter) Throw(obj coretypes.Object) {
	panic(r.Errorf("%s", obj.ToString(false)))
}

func (RuntimeExecutionAdapter) Equal(a coretypes.Object, b coretypes.Object) bool {
	return a.Equals(b)
}

func (RuntimeExecutionAdapter) ApplyCaptureSlots(slots []coretypes.Object, idxs []int, values []coretypes.Object) bool {
	if len(idxs) != len(values) {
		return false
	}
	for i, obj := range values {
		idx := idxs[i]
		if idx < 0 || idx >= len(slots) {
			return false
		}
		slots[idx] = obj
	}
	return true
}

func (RuntimeExecutionAdapter) ApplyTypedCaptureSlots(slots []irValue, idxs []int, values []coretypes.Object) bool {
	if len(idxs) != len(values) {
		return false
	}
	for i, obj := range values {
		idx := idxs[i]
		if idx < 0 || idx >= len(slots) {
			return false
		}
		slots[idx] = objectToIRValue(obj)
	}
	return true
}

func (r RuntimeExecutionAdapter) PrepareCallSlots(prog *IRProgram, args []coretypes.Object, env *LocalEnv) []coretypes.Object {
	if prog == nil || len(prog.captureKeys) == 0 {
		return args
	}
	full := make([]coretypes.Object, prog.numSlots)
	copy(full, args)
	r.InstallEnvCaptures(prog, full, env)
	return full
}

func (RuntimeExecutionAdapter) InstallEnvCaptures(prog *IRProgram, slots []coretypes.Object, env *LocalEnv) {
	if prog == nil {
		return
	}
	for ci, ck := range prog.captureKeys {
		if ci >= len(prog.captureSlotIdxs) {
			return
		}
		idx := prog.captureSlotIdxs[ci]
		if idx < 0 || idx >= len(slots) {
			continue
		}
		for e := env; e != nil; e = e.parent {
			if ck.index < len(e.bindings) {
				slots[idx] = e.bindings[ck.index]
				break
			}
		}
	}
}

func (RuntimeExecutionAdapter) InstallTypedEnvCaptures(prog *IRProgram, slots []irValue, env *LocalEnv) {
	if prog == nil {
		return
	}
	for ci, ck := range prog.captureKeys {
		if ci >= len(prog.captureSlotIdxs) {
			return
		}
		idx := prog.captureSlotIdxs[ci]
		if idx < 0 || idx >= len(slots) {
			continue
		}
		for e := env; e != nil; e = e.parent {
			if ck.index < len(e.bindings) {
				slots[idx] = objectToIRValue(e.bindings[ck.index])
				break
			}
		}
	}
}

func (RuntimeExecutionAdapter) MakeFn(fnExpr *FnExpr, slots []coretypes.Object) coretypes.Object {
	fnEnv := &LocalEnv{bindings: make([]coretypes.Object, len(slots))}
	copy(fnEnv.bindings, slots)
	return &Fn{fnExpr: fnExpr, env: fnEnv}
}

func (RuntimeExecutionAdapter) CallArgs(argsSeq coretypes.Object) ([]coretypes.Object, bool) {
	seqable, ok := argsSeq.(coretypes.Seqable)
	if !ok {
		return nil, false
	}
	seq := seqable.Seq()
	if seq == nil {
		return nil, true
	}
	return corecollections.ToSlice(seq), true
}

func (RuntimeExecutionAdapter) CallObject(fnObj coretypes.Object, args []coretypes.Object) (coretypes.Object, bool) {
	callable, ok := fnObj.(coretypes.Callable)
	if !ok {
		return nil, false
	}
	defer func() {
		if failure := recover(); failure != nil {
			if err, ok := failure.(coretypes.Error); ok {
				if _, marked := err.(*irCallbackError); marked {
					panic(failure)
				}
				panic(&irCallbackError{irLanguageError: err})
			}
			panic(failure)
		}
	}()
	currentGRT().CallableEpoch++
	return callable.Call(args), true
}

func (adapter RuntimeExecutionAdapter) CallObjectWithSyntheticCallExpr(fnObj coretypes.Object, args []coretypes.Object) (coretypes.Object, bool) {
	grt := currentGRT()
	prevExpr := grt.CurrentExpr
	grt.CurrentExpr = &CallExpr{}
	defer func() { grt.CurrentExpr = prevExpr }()
	return adapter.CallObject(fnObj, args)
}

func (RuntimeExecutionAdapter) HasMutableSlotCandidate(slots []coretypes.Object) bool {
	for _, s := range slots {
		switch s.(type) {
		case *corecollections.ArrayVector, *corecollections.ArrayMap, *corecollections.HashMap, coretypes.String:
			return true
		}
	}
	return false
}

func (RuntimeExecutionAdapter) MutableSlotObject(obj coretypes.Object, escapeInfo *EscapeInfo, slot int) coretypes.Object {
	if escapeInfo == nil || slot < 0 || slot >= len(escapeInfo.SafeMutableSlots) || !escapeInfo.SafeMutableSlots[slot] {
		return obj
	}
	switch v := obj.(type) {
	case *corecollections.ArrayVector:
		return coretypes.ToTransient(v.Arr)
	case *corecollections.ArrayMap:
		return coretypes.MapToTransient(v)
	case *corecollections.HashMap:
		return coretypes.MapToTransient(v)
	case coretypes.String:
		if !corert.IRStringBuilderDisabled() && slot < len(escapeInfo.StringPrependSlots) {
			builder := slot < len(escapeInfo.StringBuilderSlots) && escapeInfo.StringBuilderSlots[slot]
			prepend := escapeInfo.StringPrependSlots[slot]
			if (corert.IRStringBuilderForce() && (builder || prepend)) || (!corert.IRStringBuilderForce() && prepend) {
				return coretypes.NewTransientString(v)
			}
		}
	}
	return obj
}

func (RuntimeExecutionAdapter) PersistentResult(result coretypes.Object) coretypes.Object {
	switch v := result.(type) {
	case *coretypes.TransientVector:
		return v.ToPersistent()
	case *coretypes.TransientMap:
		return v.ToPersistent()
	case *coretypes.TransientString:
		return v.ToPersistent()
	default:
		return result
	}
}

func (RuntimeExecutionAdapter) Get(coll coretypes.Object, key coretypes.Object, def coretypes.Object) coretypes.Object {
	if g, ok := coll.(coretypes.Gettable); ok {
		if ok, v := g.Get(key); ok {
			return v
		}
	}
	return def
}

func (RuntimeExecutionAdapter) Assoc(coll coretypes.Object, key coretypes.Object, val coretypes.Object) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *coretypes.TransientVector:
		return c.AssocInPlace(key, val), true
	case *coretypes.TransientMap:
		return c.AssocInPlace(key, val), true
	case coretypes.Associative:
		return c.Assoc(key, val), true
	default:
		return nil, false
	}
}

// stringNthFast returns the i-th rune of s with an ASCII-prefix fast path.
//
// Joker's string indexing is by rune index. For ASCII prefixes, byte and rune
// offsets are identical, which covers the common CLBG/gi text-processing hot
// path without changing Unicode semantics. If a non-ASCII byte appears before
// the requested index, this falls back to the Unicode-correct range walk.
func stringNthFast(s string, i int) coretypes.Object {
	if i < 0 {
		panic(RT.NewError(fmt.Sprintf("Negative index: %d", i)))
	}
	if r, length, ok := corestr.NthRune(s, i); ok {
		return coretypes.Char{Ch: r}
	} else {
		panic(RT.NewError(fmt.Sprintf("Index %d exceeds string's length %d", i, length)))
	}
}

func (RuntimeExecutionAdapter) Nth(coll coretypes.Object, idx int) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *corecollections.ArrayVector:
		if idx >= 0 && idx < len(c.Arr) {
			return c.Arr[idx], true
		}
	case *coretypes.TransientVector:
		if idx >= 0 && idx < len(c.Arr) {
			return c.Arr[idx], true
		}
	case coretypes.String:
		return stringNthFast(c.S, idx), true
	case coretypes.Indexed:
		return c.Nth(idx), true
	}
	return nil, false
}

func (RuntimeExecutionAdapter) Conj(coll coretypes.Object, val coretypes.Object) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *coretypes.TransientVector:
		return c.ConjInPlace(val), true
	case coretypes.Conjable:
		return c.Conj(val), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) First(coll coretypes.Object) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *corecollections.ArrayVector:
		if len(c.Arr) > 0 {
			return c.Arr[0], true
		}
		return NIL, true
	case *coretypes.TransientVector:
		if len(c.Arr) > 0 {
			return c.Arr[0], true
		}
		return NIL, true
	case coretypes.Seqable:
		s := c.Seq()
		if s == nil || s.IsEmpty() {
			return NIL, true
		}
		return s.First(), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) BuildVector(items []coretypes.Object) coretypes.Object {
	arr := make([]coretypes.Object, len(items))
	copy(arr, items)
	return &corecollections.ArrayVector{Arr: arr}
}

func (RuntimeExecutionAdapter) ToTransient(coll coretypes.Object) (coretypes.Object, bool) {
	if av, ok := coll.(*corecollections.ArrayVector); ok {
		return coretypes.ToTransient(av.Arr), true
	}
	return nil, false
}

func (RuntimeExecutionAdapter) AssocBang(coll coretypes.Object, key coretypes.Object, val coretypes.Object) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *coretypes.TransientVector:
		return c.AssocInPlace(key, val), true
	case *coretypes.TransientMap:
		return c.AssocInPlace(key, val), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) ToPersistent(coll coretypes.Object) (coretypes.Object, bool) {
	switch c := coll.(type) {
	case *coretypes.TransientVector:
		return c.ToPersistent(), true
	case *coretypes.TransientMap:
		return c.ToPersistent(), true
	default:
		return nil, false
	}
}

func (RuntimeExecutionAdapter) Str1(obj coretypes.Object) coretypes.Object {
	switch v := obj.(type) {
	case Nil:
		return coretypes.String{S: ""}
	case coretypes.String:
		return v
	case coretypes.Char:
		return coretypes.CharToStringObjectFast(v.Ch)
	default:
		return coretypes.String{S: obj.ToString(false)}
	}
}

func (RuntimeExecutionAdapter) Str2(a coretypes.Object, b coretypes.Object) coretypes.Object {
	switch av := a.(type) {
	case *coretypes.TransientString:
		switch bv := b.(type) {
		case coretypes.Char:
			return av.AppendChar(bv.Ch)
		case coretypes.String:
			return av.AppendString(bv.S)
		default:
			return av.AppendString(b.ToString(false))
		}
	case coretypes.String:
		switch bv := b.(type) {
		case coretypes.Char:
			return coretypes.String{S: av.S + corestr.CharToStringFast(bv.Ch)}
		case coretypes.String:
			return coretypes.String{S: av.S + bv.S}
		case *coretypes.TransientString:
			return bv.PrependString(av.S)
		default:
			return coretypes.String{S: av.S + b.ToString(false)}
		}
	case coretypes.Char:
		if bv, ok := b.(*coretypes.TransientString); ok {
			return bv.PrependChar(av.Ch)
		}
		return coretypes.String{S: corestr.CharToStringFast(av.Ch) + b.ToString(false)}
	default:
		return coretypes.String{S: a.ToString(false) + b.ToString(false)}
	}
}

func (RuntimeExecutionAdapter) Count(obj coretypes.Object) (int, bool) {
	switch v := obj.(type) {
	case *coretypes.TransientString:
		return v.Count(), true
	case coretypes.Counted:
		return v.Count(), true
	default:
		return 0, false
	}
}

func (adapter RuntimeExecutionAdapter) NthASCIIStringConst(prog *IRProgram, constIdx int, idx int) (coretypes.Object, bool) {
	constant, ok := adapter.ProgramConstant(prog, constIdx)
	if !ok {
		return nil, false
	}
	s, ok := constant.(coretypes.String)
	if !ok || idx < 0 || idx >= len(s.S) {
		return nil, false
	}
	return coretypes.Char{Ch: rune(s.S[idx])}, true
}

func (RuntimeExecutionAdapter) CursorChar(obj coretypes.Object) (coretypes.Object, bool) {
	cur, ok := obj.(*coretypes.StringCursor)
	if !ok {
		return nil, false
	}
	if r := cur.Char(); r >= 0 {
		return coretypes.Char{Ch: r}, true
	}
	return NIL, true
}

func (RuntimeExecutionAdapter) CursorNext(obj coretypes.Object) (coretypes.Object, bool) {
	cur, ok := obj.(*coretypes.StringCursor)
	if !ok {
		return nil, false
	}
	return cur.Next(), true
}

func (RuntimeExecutionAdapter) CursorDone(obj coretypes.Object) (coretypes.Object, bool) {
	cur, ok := obj.(*coretypes.StringCursor)
	if !ok {
		return nil, false
	}
	return coretypes.Boolean{B: cur.Done()}, true
}

func (RuntimeExecutionAdapter) MarkTypedExecutionFailed(prog *IRProgram) {
	if prog != nil {
		prog.typedFailed.Store(true)
	}
}

func (RuntimeExecutionAdapter) MarkBoxedExecutionFailed(prog *IRProgram) {
	if prog != nil {
		prog.execFailed.Store(true)
	}
}

func (RuntimeExecutionAdapter) ProgramNumSlots(prog *IRProgram) int {
	if prog == nil {
		return 0
	}
	return prog.numSlots
}

func (RuntimeExecutionAdapter) ProgramCode(prog *IRProgram) []byte {
	if prog == nil {
		return nil
	}
	return prog.code
}

func (RuntimeExecutionAdapter) ProgramModel(prog *IRProgram) *coreir.Program {
	if prog == nil {
		return nil
	}
	return prog.neutralModel()
}

func (RuntimeExecutionAdapter) ProgramConstant(prog *IRProgram, idx int) (coretypes.Object, bool) {
	if prog == nil || idx < 0 || idx >= len(prog.constants) {
		return nil, false
	}
	return prog.constants[idx], true
}

func (RuntimeExecutionAdapter) ProgramConstants(prog *IRProgram) []coretypes.Object {
	if prog == nil {
		return nil
	}
	return prog.constants
}

func (RuntimeExecutionAdapter) ProgramFnExpr(prog *IRProgram, idx int) (*FnExpr, bool) {
	if prog == nil || idx < 0 || idx >= len(prog.fnExprs) {
		return nil, false
	}
	return prog.fnExprs[idx], true
}

func (RuntimeExecutionAdapter) FnProgram(fnObj coretypes.Object) (*IRProgram, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	prog := irGetFnProg(fn)
	return prog, prog != nil
}

func (RuntimeExecutionAdapter) CompileFnProgram(fnObj coretypes.Object) (*IRProgram, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	prog := irCompileFn(fn)
	return prog, prog != nil
}

func (RuntimeExecutionAdapter) FnWasmExec(fnObj coretypes.Object, args []coretypes.Object) (coretypes.Object, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	wp := wasmGetFn(fn)
	if wp == nil {
		return nil, false
	}
	result := wasmExec(wp, args)
	return result, result != nil
}

func (adapter RuntimeExecutionAdapter) FnCallSlots(fnObj coretypes.Object, prog *IRProgram, args []coretypes.Object) ([]coretypes.Object, bool) {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return nil, false
	}
	return adapter.PrepareCallSlots(prog, args, fn.env), true
}

func (adapter RuntimeExecutionAdapter) InstallFnTypedEnvCaptures(fnObj coretypes.Object, prog *IRProgram, slots []irValue) bool {
	fn, ok := fnObj.(*Fn)
	if !ok {
		return false
	}
	adapter.InstallTypedEnvCaptures(prog, slots, fn.env)
	return true
}

func (RuntimeExecutionAdapter) ObjectsFromTypedValues(values []irValue, buf []coretypes.Object) []coretypes.Object {
	var out []coretypes.Object
	if len(values) <= len(buf) {
		out = buf[:len(values)]
	} else {
		out = make([]coretypes.Object, len(values))
	}
	for i, v := range values {
		out[i] = v.object()
	}
	return out
}

func (adapter RuntimeExecutionAdapter) DispatchArityProgram(prog *IRProgram, nargs int) *IRProgram {
	if prog == nil {
		return nil
	}
	if prog.arityPrograms == nil {
		if prog.variadicMinArgs > 0 && nargs < prog.variadicMinArgs {
			return nil
		}
		return prog
	}
	if sub, ok := prog.arityPrograms[nargs]; ok {
		return sub
	}
	if prog.variadicProg != nil && nargs >= prog.variadicMinArgs {
		return prog.variadicProg
	}
	return nil
}

func (RuntimeExecutionAdapter) ProgramHasCaptureSlots(prog *IRProgram) bool {
	return prog != nil && len(prog.captureSlots) > 0
}

func (RuntimeExecutionAdapter) ProgramEscapeInfo(prog *IRProgram) *EscapeInfo {
	if prog == nil {
		return nil
	}
	prog.stateMu.Lock()
	defer prog.stateMu.Unlock()
	if prog.escapeInfo == nil {
		prog.escapeInfo = analyzeEscapes(prog)
	}
	return prog.escapeInfo
}

func (RuntimeExecutionAdapter) ProgramAnalysis(prog *IRProgram) coreir.Analysis {
	return AnalyzeIRProgram(prog)
}

func (adapter RuntimeExecutionAdapter) ApplyProgramCaptureSlots(prog *IRProgram, slots []coretypes.Object) bool {
	if prog == nil {
		return false
	}
	return adapter.ApplyCaptureSlots(slots, prog.captureSlotIdxs, prog.captureSlots)
}

func (adapter RuntimeExecutionAdapter) ApplyProgramTypedCaptureSlots(prog *IRProgram, slots []irValue) bool {
	if prog == nil {
		return false
	}
	return adapter.ApplyTypedCaptureSlots(slots, prog.captureSlotIdxs, prog.captureSlots)
}

func (adapter RuntimeExecutionAdapter) ClearTypedNonCaptureSlots(prog *IRProgram, slots []irValue, keepArgs int) bool {
	if keepArgs < 0 || keepArgs > len(slots) {
		return false
	}
	if prog != nil && prog.captureSlotSet != nil {
		if len(prog.captureSlotSet) < len(slots) {
			return false
		}
		for i := keepArgs; i < len(slots); i++ {
			if !prog.captureSlotSet[i] {
				slots[i] = irValue{}
			}
		}
		return true
	}
	for i := keepArgs; i < len(slots); i++ {
		slots[i] = irValue{}
	}
	if prog == nil || len(prog.captureSlots) == 0 {
		return true
	}
	return adapter.ApplyProgramTypedCaptureSlots(prog, slots)
}

func (RuntimeExecutionAdapter) ProgramCaptureSlots(prog *IRProgram) ([]int, []coretypes.Object) {
	if prog == nil {
		return nil, nil
	}
	return prog.captureSlotIdxs, prog.captureSlots
}

func (RuntimeExecutionAdapter) CanExecuteIR(prog *IRProgram) bool {
	return prog != nil && !prog.execFailed.Load()
}

func (RuntimeExecutionAdapter) CanExecuteTypedIR(prog *IRProgram) bool {
	return prog != nil && !prog.typedFailed.Load() && !prog.execFailed.Load()
}

func (RuntimeExecutionAdapter) HasNativeHelper(prog *IRProgram) bool {
	if prog == nil {
		return false
	}
	prog.stateMu.RLock()
	defer prog.stateMu.RUnlock()
	return prog.nativeHelper != nil
}

func (RuntimeExecutionAdapter) NativeHelper(prog *IRProgram) (nativeF64Fn, bool) {
	if prog == nil {
		return nil, false
	}
	prog.stateMu.RLock()
	defer prog.stateMu.RUnlock()
	return prog.nativeHelper, prog.nativeHelper != nil
}

func (RuntimeExecutionAdapter) InstallNativeHelper(prog *IRProgram, helper nativeF64Fn) {
	if prog != nil {
		prog.stateMu.Lock()
		prog.nativeHelper = helper
		prog.nativeChecked = true
		prog.stateMu.Unlock()
	}
}

func (RuntimeExecutionAdapter) NativeHelperChecked(prog *IRProgram) bool {
	if prog == nil {
		return false
	}
	prog.stateMu.RLock()
	defer prog.stateMu.RUnlock()
	return prog.nativeChecked
}

func (RuntimeExecutionAdapter) CanTryMemNth(prog *IRProgram) bool {
	return prog != nil && !prog.memNthFailed.Load()
}

func (RuntimeExecutionAdapter) MarkMemNthFailed(prog *IRProgram) {
	if prog != nil {
		prog.memNthFailed.Store(true)
	}
}

// ---- object.go ----

type (
	Nil = corert.Nil
	Var struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		ns             *Namespace
		name           coretypes.Symbol
		Value          coretypes.Object
		expr           Expr
		isMacro        bool
		isPrivate      bool
		isDynamic      bool
		isUsed         bool
		isGloballyUsed bool
		isFake         bool
		taggedType     *coretypes.Type
	}
	ProcFn func([]coretypes.Object) coretypes.Object
	Proc   struct {
		Fn      ProcFn
		Name    string
		Package string // "" for core (this package), else e.g. "std/string"
	}
	Fn struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		isMacro       bool
		fnExpr        *FnExpr
		env           *LocalEnv
		tailRewritten bool       // tail-self-calls rewritten to recur
		irProg        *IRProgram // cached IR compilation (nil = not attempted, irCompileFailed = failed)
		irProgInit    sync.Once  // serializes compilation and safely publishes irProg
		irProgOnce    uint32     // atomic: 0=not tried, 1=done
		defVar        *Var       // set when this fn is the value of a defn-created var
	}
	ExInfo struct {
		corecollections.ArrayMap
		rt *goroutineRT
	}
)

var NIL = corert.Nil{}

func init() {
	coretypes.RuntimeNil = NIL
	coretypes.RuntimeError = func(msg string) any { return RT.NewError(msg) }
	coretypes.RuntimePanicArityMinMax = PanicArityMinMax
	coretypes.RuntimePprintObject = corert.PprintObject
	coretypes.RuntimeFormatObject = corert.FormatObject
	coretypes.RuntimeMaybeNewLine = corert.MaybeNewLine
	coretypes.RuntimeWriteIndent = corert.WriteIndent
	coretypes.RuntimeIsComment = corert.IsComment
	coretypes.RuntimeIsReduced = corert.IsReduced
	coretypes.RuntimeDerefReduced = corert.DerefReduced
}

func PanicArity(n int) {
	grt := currentGRT()
	name := "<unknown>"
	if grt.CurrentExpr != nil {
		if tr, ok := grt.CurrentExpr.(Traceable); ok {
			name = tr.Name()
		}
	}
	panic(RT.NewError(fmt.Sprintf("Wrong number of args (%d) passed to %s", n, name)))
}

func PanicArityMinMax(n, min, max int) {
	grt := currentGRT()
	name := "<unknown>"
	if grt.CurrentExpr != nil {
		if tr, ok := grt.CurrentExpr.(Traceable); ok {
			name = tr.Name()
		}
	}
	panic(RT.NewError(fmt.Sprintf("Wrong number of args (%d) passed to %s; expects %s", n, name, corestr.IntRangeLabel(min, max))))
}

func CheckArity(args []coretypes.Object, min int, max int) {
	n := len(args)
	if n < min || n > max {
		PanicArityMinMax(n, min, max)
	}
}

func runtimeCheckArity(args []coretypes.Object, min int, max int) {
	n := len(args)
	if n < min || n > max {
		coretypes.RuntimePanicArityMinMax(n, min, max)
	}
}

func getMap(k coretypes.Object, args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 2)
	switch m := args[0].(type) {
	case coretypes.Map:
		ok, v := m.Get(k)
		if ok {
			return v
		}
	}
	if len(args) == 2 {
		return args[1]
	}
	return NIL
}

func (exInfo *ExInfo) ToString(escape bool) string {
	return exInfo.Error()
}

func (exInfo *ExInfo) Equals(other interface{}) bool {
	return exInfo == other
}

func (exInfo *ExInfo) GetType() *coretypes.Type {
	return TYPE.ExInfo
}

func (exInfo *ExInfo) Hash() uint32 {
	return hashutil.Ptr(uintptr(unsafe.Pointer(exInfo)))
}

func (exInfo *ExInfo) Message() coretypes.Object {
	if ok, res := exInfo.Get(KEYWORDS.message); ok {
		return res
	}
	return NIL
}

func (exInfo *ExInfo) Error() string {
	var pos coretypes.Position
	_, data := exInfo.Get(KEYWORDS.data)
	ok, form := data.(coretypes.Map).Get(KEYWORDS.form)
	if ok {
		if form.GetInfo() != nil {
			pos = form.GetInfo().Pos()
		}
	}
	prefix := "Exception"
	if ok, pr := data.(coretypes.Map).Get(KEYWORDS._prefix); ok {
		prefix = pr.ToString(false)
	}
	_, msg := exInfo.Get(KEYWORDS.message)
	if exInfo.rt.Callstack.Len() > 0 && !LINTER_MODE {
		return fmt.Sprintf("%s:%d:%d: %s: %s\nStacktrace:\n%s", pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn, prefix, msg.(coretypes.String).S, runtimeStacktrace(exInfo.rt))
	} else {
		return fmt.Sprintf("%s:%d:%d: %s: %s", pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn, prefix, msg.(coretypes.String).S)
	}
}

func (fn *Fn) ToString(escape bool) string {
	return "#object[Fn]"
}

func (fn *Fn) Equals(other interface{}) bool {
	switch other := other.(type) {
	case *Fn:
		return fn == other
	default:
		return false
	}
}

func (fn *Fn) WithMeta(meta coretypes.Map) coretypes.Object {
	res := &Fn{
		InfoHolder:    fn.InfoHolder,
		MetaHolder:    fn.MetaHolder,
		isMacro:       fn.isMacro,
		fnExpr:        fn.fnExpr,
		env:           fn.env,
		tailRewritten: fn.tailRewritten,
		defVar:        fn.defVar,
	}
	if atomic.LoadUint32(&fn.irProgOnce) == 1 {
		res.irProg = fn.irProg
		atomic.StoreUint32(&res.irProgOnce, 1)
	}
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return res
}

func (fn *Fn) GetType() *coretypes.Type {
	return TYPE.Fn
}

// clearArgs nils out an args slice to release references for GC.
// This prevents retention of large objects across recursive call chains.
func clearArgs(args []coretypes.Object) {
	for i := range args {
		args[i] = nil
	}
}

func (fn *Fn) Hash() uint32 {
	return hashutil.Ptr(uintptr(unsafe.Pointer(fn)))
}

func (fn *Fn) Call(args []coretypes.Object) coretypes.Object {
	defer traceFnCall(fn, len(args))()
	// Fast path: native Go codegen for defn-originated pure-integer recursive fns
	if fn.defVar != nil {
		if entry := tryNativeRecursive(fn); entry != nil {
			if result := callNativeRecursive(entry, args); result != nil {
				return result
			}
		}
	}
	if len(fn.fnExpr.arities) == 1 {
		arity := fn.fnExpr.arities[0]
		if len(arity.args) == len(args) {
			// If tail-self-calls were rewritten to recur at parse time,
			// use evalLoop directly (no trampoline needed)
			if fn.fnExpr.tailRewritten {
				RT.pushFrame()
				// Try WASM for rewritten tail-recursive fns
				if wp := wasmGetFn(fn); wp != nil {
					if result := wasmExec(wp, args); result != nil {
						clearArgs(args)
						RT.popFrame()
						return result
					}
				}
				// Try IR
				if prog := irCompileFn(fn); prog != nil {
					if result := irExec(prog, args); result != nil {
						clearArgs(args)
						RT.popFrame()
						return result
					}
				}
				// Fallback to evalLoop
				childEnv := LocalEnv{bindings: args, parent: fn.env}
				if fn.env != nil {
					childEnv.frame = fn.env.frame + 1
				}
				res := evalLoop(arity.body, &childEnv)
				RT.popFrame()
				return res
			}
			// TCO trampoline for self-recursive functions
			if fn.fnExpr.self.NameKey() != nil {
				// Try native Go codegen for pure-integer recursive fns
				if fn.defVar != nil {
					if entry := tryNativeRecursive(fn); entry != nil {
						if result := callNativeRecursive(entry, args); result != nil {
							return result
						}
					}
					// Try IR compilation
					if prog := irCompileFn(fn); prog != nil {
						if result := irExec(prog, args); result != nil {
							return result
						}
					}
				}
				RT.pushFrame()
				prevActiveFn := activeFn
				activeFn = fn
				for {
					childEnv := LocalEnv{bindings: args, parent: fn.env}
					if fn.env != nil {
						childEnv.frame = fn.env.frame + 1
					}
					result := evalBodyTCO(arity.body, &childEnv, fn)
					if tc, ok := result.(*TailCall); ok && tc.fn == fn {
						args = tc.args
						continue
					}
					activeFn = prevActiveFn
					RT.popFrame()
					return result
				}
			}
			// Normal single-arity execution
			childEnv := LocalEnv{bindings: args, parent: fn.env}
			if fn.env != nil {
				childEnv.frame = fn.env.frame + 1
			}
			RT.pushFrame()
			res := evalLoop(arity.body, &childEnv)
			RT.popFrame()
			return res
		}
	}

	min := math.MaxInt32
	max := -1
	for _, arity := range fn.fnExpr.arities {
		a := len(arity.args)
		if a == len(args) {
			childEnv := LocalEnv{bindings: args, parent: fn.env}
			if fn.env != nil {
				childEnv.frame = fn.env.frame + 1
			}
			RT.pushFrame()
			res := evalLoop(arity.body, &childEnv)
			RT.popFrame()
			return res
		}
		if min > a {
			min = a
		}
		if max < a {
			max = a
		}
	}
	v := fn.fnExpr.variadic
	if v == nil || len(args) < len(v.args)-1 {
		if v != nil {
			min = len(v.args)
			max = math.MaxInt32
		}
		c := len(args)
		if fn.isMacro {
			c -= 2
			min -= 2
			if max != math.MaxInt32 {
				max -= 2
			}
		}
		PanicArityMinMax(c, min, max)
	}
	var restArgs coretypes.Object = NIL
	if len(v.args)-1 < len(args) {
		restArgs = &corecollections.ArraySeq{Arr: args, Index: len(v.args) - 1}
	}
	vargs := make([]coretypes.Object, len(v.args))
	for i := 0; i < len(vargs)-1; i++ {
		vargs[i] = args[i]
	}
	vargs[len(vargs)-1] = restArgs
	childEnv := LocalEnv{bindings: vargs, parent: fn.env}
	if fn.env != nil {
		childEnv.frame = fn.env.frame + 1
	}
	RT.pushFrame()
	res := evalLoop(v.body, &childEnv)
	RT.popFrame()
	return res
}

func compare(c coretypes.Callable, a, b coretypes.Object) int {
	switch r := call2(c, a, b).(type) {
	case coretypes.Boolean:
		if r.B {
			return -1
		}
		if coretypes.EnsureObjectIsBoolean(call2(c, b, a), "").B {
			return 1
		}
		return 0
	default:
		return coretypes.EnsureObjectIsNumber(r, "Function is not a comparator since it returned a non-integer value%.s").Int().I
	}
}

func (fn *Fn) Compare(a, b coretypes.Object) int {
	return compare(fn, a.(coretypes.Object), b.(coretypes.Object))
}

func (p Proc) Call(args []coretypes.Object) coretypes.Object {
	defer traceProcCall(p, len(args))()
	return p.Fn(args)
}

func (p Proc) Compare(a, b coretypes.Object) int {
	return compare(p, a.(coretypes.Object), b.(coretypes.Object))
}

func (p Proc) ToString(escape bool) string {
	pkg := p.Package
	if pkg != "" {
		pkg += "."
	}
	return fmt.Sprintf("#object[Proc:%s%s]", pkg, p.Name)
}

func (p Proc) Equals(other interface{}) bool {
	switch other := other.(type) {
	case Proc:
		return reflect.ValueOf(p.Fn).Pointer() == reflect.ValueOf(other.Fn).Pointer()
	}
	return false
}

func (p Proc) GetInfo() *coretypes.ObjectInfo {
	return nil
}

func (p Proc) WithInfo(*coretypes.ObjectInfo) coretypes.Object {
	return p
}

func (p Proc) GetType() *coretypes.Type {
	return TYPE.Proc
}

func (p Proc) Hash() uint32 {
	return hashutil.Ptr(reflect.ValueOf(p.Fn).Pointer())
}

func AlterMeta(m *coretypes.MetaHolder, fn coretypes.Callable, args []coretypes.Object) coretypes.Map {
	meta := m.GetMeta()
	if meta == nil {
		meta = NIL
	}
	fargs := append([]coretypes.Object{meta}, args...)
	newMeta := coretypes.EnsureObjectIsMap(fn.Call(fargs), "")
	m.SetMeta(newMeta)
	return newMeta
}

func (v *Var) Name() string {
	return v.ns.Name.ToString(false) + "/" + v.name.ToString(false)
}

func (v *Var) ToString(escape bool) string {
	return "#'" + v.Name()
}

func (v *Var) Equals(other interface{}) bool {
	return v == other
}

func (v *Var) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *v
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (v *Var) ResetMeta(newMeta coretypes.Map) coretypes.Map {
	v.Meta = newMeta
	return v.Meta
}

func (v *Var) AlterMeta(fn coretypes.Callable, args []coretypes.Object) coretypes.Map {
	return AlterMeta(&v.MetaHolder, fn, args)
}

func (v *Var) GetType() *coretypes.Type {
	return TYPE.Var
}

func (v *Var) Hash() uint32 {
	return hashutil.Ptr(uintptr(unsafe.Pointer(v)))
}

func (v *Var) Resolve() coretypes.Object {
	traceVarDeref(v)
	defer symbolTraceMaybeWrite()
	if v.Value == nil {
		return NIL
	}
	return v.Value
}

func (v *Var) Call(args []coretypes.Object) coretypes.Object {
	vl := v.Resolve()
	return coretypes.EnsureObjectIsCallable(
		vl,
		"Var "+v.ToString(false)+" resolves to "+vl.ToString(false)+", which is not a Fn").Call(args)
}

func (v *Var) Deref() coretypes.Object {
	return v.Resolve()
}

func MakeMeta(arglists coretypes.Seq, docstring string, added string) *corecollections.ArrayMap {
	res := corecollections.EmptyArrayMap()
	if arglists != nil {
		res.Add(KEYWORDS.arglist, arglists)
	}
	res.Add(KEYWORDS.doc, coretypes.String{S: docstring})
	res.Add(KEYWORDS.added, coretypes.String{S: added})
	return res
}

// ---- function_trace.go ----
// ---- function_trace.go ----
var functionTracer = coretrace.NewFunctionTracerFromEnv()

func traceFnCall(fn *Fn, argc int) func() {
	if !functionTracer.Enabled() {
		return func() {}
	}
	return functionTracer.Enter(fnTraceName(fn, argc))
}

func traceIRProgramCall(prog *IRProgram, argc int) func() {
	if !functionTracer.Enabled() || prog == nil || prog.traceName == "" {
		return func() {}
	}
	return functionTracer.Enter(fmt.Sprintf("%s/%d", prog.traceName, argc))
}

func traceProcCall(p Proc, argc int) func() {
	if !functionTracer.Enabled() {
		return func() {}
	}
	name := "proc/" + p.Name
	if p.Package != "" {
		name = p.Package + "/" + p.Name
	}
	return functionTracer.Enter(fmt.Sprintf("%s/%d", name, argc))
}

func fnTraceName(fn *Fn, argc int) string {
	if fn.defVar != nil {
		if fn.defVar.ns != nil {
			return fmt.Sprintf("%s/%s/%d", fn.defVar.ns.Name.ToString(false), fn.defVar.name.ToString(false), argc)
		}
		return fmt.Sprintf("%s/%d", fn.defVar.name.ToString(false), argc)
	}
	if fn.fnExpr != nil && fn.fnExpr.traceName != "" {
		return fmt.Sprintf("%s/%d", fn.fnExpr.traceName, argc)
	}
	if fn.fnExpr != nil && fn.fnExpr.self.NameKey() != nil {
		return fmt.Sprintf("%s/%d", fn.fnExpr.self.ToString(false), argc)
	}
	if info := fn.GetInfo(); info != nil {
		return fmt.Sprintf("fn@%s:%d/%d", info.FilenameOrUnknown(), info.StartLine, argc)
	}
	return fmt.Sprintf("fn@%p/%d", fn, argc)
}

// ---- trace_adapters.go ----
var symbolTracer = coretrace.NewSymbolTracerFromEnv()
var zeroTime time.Time
var irProfile = coretrace.NewIRProfileFromEnv()

func traceSymbolResolve(ns *Namespace, sym coretypes.Symbol, ok bool) {
	if !symbolTracer.Enabled() || !ok {
		return
	}
	name := sym.ToString(false)
	if ns != nil && sym.NamespaceKey() == nil {
		name = ns.Name.ToString(false) + "/" + name
	}
	symbolTracer.Resolve(name)
}

func traceVarDeref(v *Var) {
	if !symbolTracer.Enabled() || v == nil {
		return
	}
	name := v.name.ToString(false)
	if v.ns != nil {
		name = v.ns.Name.ToString(false) + "/" + v.name.ToString(false)
	}
	symbolTracer.Deref(name)
}

func symbolTraceMaybeWrite() {
	symbolTracer.Write()
}

func irProfileExecStart() {
	irProfile.ExecStart()
}

func irProfileStart() time.Time {
	if !irProfile.Enabled() {
		return zeroTime
	}
	return irProfile.Start()
}

func irProfileOp(prev byte, op byte, hasPrev bool, prevStarted time.Time) time.Time {
	return irProfile.Op(prev, op, hasPrev, prevStarted)
}

func irProfileFinish(last byte, hasLast bool, started time.Time) {
	irProfile.Finish(last, hasLast, started)
}

func irProfileMaybeWrite() {
	irProfile.Write(coreir.OpcodeName)
}

// ---- common.go ----
// ---- root_object_support.go ----
func EnsureObjectIsNamespace(obj coretypes.Object, pattern string) *Namespace {
	if c, yes := obj.(*Namespace); yes {
		return c
	}
	panic(FailObject(obj, "Namespace", pattern))
}

func EnsureArgIsNamespace(args []coretypes.Object, index int) *Namespace {
	obj := args[index]
	if c, yes := obj.(*Namespace); yes {
		return c
	}
	panic(FailArg(obj, "Namespace", index))
}

func EnsureObjectIsVar(obj coretypes.Object, pattern string) *Var {
	if c, yes := obj.(*Var); yes {
		return c
	}
	panic(FailObject(obj, "Var", pattern))
}

func EnsureArgIsVar(args []coretypes.Object, index int) *Var {
	obj := args[index]
	if c, yes := obj.(*Var); yes {
		return c
	}
	panic(FailArg(obj, "Var", index))
}

func EnsureObjectIsFn(obj coretypes.Object, pattern string) *Fn {
	if c, yes := obj.(*Fn); yes {
		return c
	}
	panic(FailObject(obj, "Fn", pattern))
}

func EnsureArgIsFn(args []coretypes.Object, index int) *Fn {
	obj := args[index]
	if c, yes := obj.(*Fn); yes {
		return c
	}
	panic(FailArg(obj, "Fn", index))
}

func EnsureObjectIsAtom(obj coretypes.Object, pattern string) *corert.Atom {
	if c, yes := obj.(*corert.Atom); yes {
		return c
	}
	panic(FailObject(obj, "Atom", pattern))
}

func EnsureArgIsAtom(args []coretypes.Object, index int) *corert.Atom {
	obj := args[index]
	if c, yes := obj.(*corert.Atom); yes {
		return c
	}
	panic(FailArg(obj, "Atom", index))
}

func EnsureObjectIsFile(obj coretypes.Object, pattern string) *corert.File {
	if c, yes := obj.(*corert.File); yes {
		return c
	}
	panic(FailObject(obj, "File", pattern))
}

func EnsureArgIsFile(args []coretypes.Object, index int) *corert.File {
	obj := args[index]
	if c, yes := obj.(*corert.File); yes {
		return c
	}
	panic(FailArg(obj, "File", index))
}

func EnsureObjectIsChannel(obj coretypes.Object, pattern string) *corert.ObjectChannel {
	if c, yes := obj.(*corert.ObjectChannel); yes {
		return c
	}
	panic(FailObject(obj, "Channel", pattern))
}

func EnsureArgIsChannel(args []coretypes.Object, index int) *corert.ObjectChannel {
	obj := args[index]
	if c, yes := obj.(*corert.ObjectChannel); yes {
		return c
	}
	panic(FailArg(obj, "Channel", index))
}

// ---- with_info_root.go ----
func (x *ExInfo) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	x.Info = info
	return x
}

func (x *Fn) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	x.Info = info
	return x
}

func (x *Var) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	x.Info = info
	return x
}

// ---- string_cursor.go ----
// ---- string_cursor_procs.go ----

var stringCursorInitOnce sync.Once

func initStringCursorProcs() {
	stringCursorInitOnce.Do(func() {
		ns := GLOBAL_ENV.CoreNamespace
		procs := []struct {
			name  string
			fn    func([]coretypes.Object) coretypes.Object
			pname string
		}{
			{"string-cursor", procStringCursor, "procStringCursor"},
			{"cursor-char", procCursorChar, "procCursorChar"},
			{"cursor-next", procCursorNext, "procCursorNext"},
			{"cursor-done?", procCursorDone, "procCursorDone"},
			{"cursor-index", procCursorIndex, "procCursorIndex"},
		}
		for _, p := range procs {
			sym := coretypes.MakeSymbol(STRINGS.Intern, p.name)
			vr := ns.Intern(sym)
			vr.Value = Proc{Fn: p.fn, Name: p.pname}
			curNs := GLOBAL_ENV.CurrentNamespace()
			if curNs != nil && curNs != ns {
				curNs.mappings[sym.NameKey()] = vr
			}
		}
	})
}

func procStringCursor(args []coretypes.Object) coretypes.Object {
	s, ok := args[0].(coretypes.String)
	if !ok {
		panic(RT.NewError("string-cursor expects a string argument"))
	}
	return coretypes.NewStringCursor(s.S)
}

func procCursorChar(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*coretypes.StringCursor)
	if !ok {
		panic(RT.NewError("cursor-char expects a StringCursor"))
	}
	r := c.Char()
	if r < 0 {
		return NIL
	}
	return coretypes.Char{Ch: r}
}

func procCursorNext(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*coretypes.StringCursor)
	if !ok {
		panic(RT.NewError("cursor-next expects a StringCursor"))
	}
	return c.Next()
}

func procCursorDone(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*coretypes.StringCursor)
	if !ok {
		panic(RT.NewError("cursor-done? expects a StringCursor"))
	}
	return coretypes.Boolean{B: c.Done()}
}

func procCursorIndex(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*coretypes.StringCursor)
	if !ok {
		panic(RT.NewError("cursor-index expects a StringCursor"))
	}
	return coretypes.Int{I: c.Index()}
}

// ---- format.go ----

func seqFirst(seq coretypes.Seq, w io.Writer, indent int) (coretypes.Seq, int) {
	if !seq.IsEmpty() {
		indent = corert.FormatObject(seq.First(), indent, w)
		seq = seq.Rest()
	}
	return seq, indent
}

// TODO: maybe merge it with seqFirstAfterBreak
// or extract common part into a separate function
func seqFirstAfterSpace(seq coretypes.Seq, w io.Writer, indent int, insideDefRecord bool) (coretypes.Seq, coretypes.Object, int) {
	var obj coretypes.Object
	if !seq.IsEmpty() {
		fmt.Fprint(w, " ")
		obj = seq.First()
		// coretypes.Seq handling here is needed to properly format methods
		// inside defrecord
		if s, ok := obj.(coretypes.Seq); ok && !obj.Equals(NIL) {
			if info := obj.GetInfo(); info != nil {
				fmt.Fprint(w, info.Prefix)
				indent += utf8.RuneCountInString(info.Prefix)
			}
			indent = formatSeqEx(s, w, indent+1, insideDefRecord)
		} else {
			indent = corert.FormatObject(obj, indent+1, w)
		}
		seq = seq.Rest()
	}
	return seq, obj, indent
}

func seqFirstAfterBreak(prevObj coretypes.Object, seq coretypes.Seq, w io.Writer, indent int, insideDefRecord bool) (coretypes.Seq, coretypes.Object, int) {
	var obj coretypes.Object
	if !seq.IsEmpty() {
		obj = seq.First()
		corert.WriteNewLines(w, prevObj, obj)
		corert.WriteIndent(w, indent)
		// coretypes.Seq handling here is needed to properly format methods
		// inside defrecord
		if s, ok := obj.(coretypes.Seq); ok && !obj.Equals(NIL) {
			if info := obj.GetInfo(); info != nil {
				fmt.Fprint(w, info.Prefix)
				indent += utf8.RuneCountInString(info.Prefix)
			}
			indent = formatSeqEx(s, w, indent, insideDefRecord)
		} else {
			indent = corert.FormatObject(obj, indent, w)
		}
		seq = seq.Rest()
	}
	return seq, obj, indent
}

func seqFirstAfterForcedBreak(seq coretypes.Seq, w io.Writer, indent int) (coretypes.Seq, coretypes.Object, int) {
	var obj coretypes.Object
	if !seq.IsEmpty() {
		obj = seq.First()
		fmt.Fprint(w, "\n")
		corert.WriteIndent(w, indent)
		indent = corert.FormatObject(obj, indent, w)
		seq = seq.Rest()
	}
	return seq, obj, indent
}

func formatBindings(v coretypes.Vec, w io.Writer, indent int) int {
	return v.Format(w, indent)
}

func formatVectorVertically(v coretypes.Vec, w io.Writer, indent int) int {
	fmt.Fprint(w, "[")
	newIndent := indent + 1
	for i := 0; i < v.Count(); i++ {
		newIndent = corert.FormatObject(v.At(i), indent+1, w)
		if i+1 < v.Count() {
			fmt.Fprint(w, "\n")
			corert.WriteIndent(w, indent+1)
		}
	}
	if v.Count() > 0 {
		if corert.IsComment(v.At(v.Count() - 1)) {
			fmt.Fprint(w, "\n")
			corert.WriteIndent(w, indent+1)
			newIndent = indent + 1
		}
	}
	fmt.Fprint(w, "]")
	return newIndent + 1
}

var defRegex *regexp.Regexp = regexp.MustCompile("^def.*$")
var ifRegex *regexp.Regexp = regexp.MustCompile("^if(-.+)?$")
var whenRegex *regexp.Regexp = regexp.MustCompile("^when(-.+)?$")
var doIndentRegex *regexp.Regexp = regexp.MustCompile("^(do|try|finally|go|alt!|alt!!)$")
var bodyIndentRegexes []*regexp.Regexp = []*regexp.Regexp{
	regexp.MustCompile("^(bound-fn|if|if-not|case|cond|cond->|cond->>|as->|condp|when|while|when-not|when-first|do|future|thread)$"),
	regexp.MustCompile("^(comment|doto|locking|proxy|with-[^\\s]*|reify|fdef)$"),
	regexp.MustCompile("^(defprotocol|extend|extend-protocol|extend-type|catch|let|letfn|binding|loop|for|go-loop)$"),
	regexp.MustCompile("^(doseq|dotimes|when-let|if-let|defstruct|struct-map|defmethod|testing|are|deftest|context|use-fixtures)$"),
	regexp.MustCompile("^(POST|GET|PUT|DELETE)"),
	regexp.MustCompile("^(handler-case|handle|dotrace|deftrace|match)$"),
}

func isOneAndBodyExpr(obj coretypes.Object) bool {
	switch s := obj.(type) {
	case coretypes.Symbol:
		name := s.Name()
		return defRegex.MatchString(name) ||
			ifRegex.MatchString(name) ||
			whenRegex.MatchString(name)
	default:
		return false
	}
}

func isDoIndent(obj coretypes.Object) bool {
	switch s := obj.(type) {
	case coretypes.Symbol:
		return doIndentRegex.MatchString(s.Name())
	default:
		return false
	}
}

func isBodyIndent(obj coretypes.Object) bool {
	switch s := obj.(type) {
	case coretypes.Symbol:
		name := s.Name()
		for _, re := range bodyIndentRegexes {
			if re.MatchString(name) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func formatSeq(seq coretypes.Seq, w io.Writer, indent int) int {
	return formatSeqEx(seq, w, indent, false)
}

func formatSeqSimple(seq coretypes.Seq, w io.Writer, indent int) int {
	ind := indent + 1
	fmt.Fprint(w, "(")
	var prevObj coretypes.Object
	for !seq.IsEmpty() {
		obj := seq.First()
		if prevObj != nil {
			ind = corert.MaybeNewLine(w, prevObj, obj, indent+1, ind)
		}
		ind = corert.FormatObject(obj, ind, w)
		prevObj = obj
		seq = seq.Rest()
	}

	if prevObj != nil {
		if corert.IsComment(prevObj) {
			fmt.Fprint(w, "\n")
			corert.WriteIndent(w, indent+1)
			ind = indent + 1
		}
	}

	fmt.Fprint(w, ")")
	return ind + 1
}

type RequireSort []coretypes.Object

func (rs RequireSort) Len() int      { return len(rs) }
func (rs RequireSort) Swap(i, j int) { rs[i], rs[j] = rs[j], rs[i] }
func (rs RequireSort) Less(i, j int) bool {
	a := rs[i]
	if s, ok := a.(coretypes.Seqable); ok {
		a = s.Seq().First()
	}
	b := rs[j]
	if s, ok := b.(coretypes.Seqable); ok {
		b = s.Seq().First()
	}
	return a.ToString(false) < b.ToString(false)
}

func sortRequire(seq coretypes.Seq) coretypes.Seq {
	s := RequireSort(corecollections.ToSlice(seq))
	sort.Sort(s)
	return &corecollections.ArraySeq{Arr: s}
}

func formatSeqEx(seq coretypes.Seq, w io.Writer, indent int, formatAsDef bool) int {
	if info := seq.GetInfo(); info != nil {
		if info.Prefix == "#?" || info.Prefix == "#?@" {
			return formatSeqSimple(seq, w, indent)
		}
	}

	i := indent + 1
	restIndent := indent + 2
	fmt.Fprint(w, "(")
	obj := seq.First()
	prevObj := obj
	seq, i = seqFirst(seq, w, i)
	isDefRecord := false
	if obj.Equals(SYMBOLS.defrecord) ||
		obj.Equals(SYMBOLS.defprotocol) ||
		obj.Equals(SYMBOLS.extendProtocol) ||
		obj.Equals(SYMBOLS.reify) ||
		obj.Equals(SYMBOLS.deftype) ||
		obj.Equals(SYMBOLS.proxy) ||
		obj.Equals(SYMBOLS.extendType) {
		isDefRecord = true
	}
	if obj.Equals(SYMBOLS.ns) || isOneAndBodyExpr(obj) {
		seq, prevObj, i = seqFirstAfterSpace(seq, w, i, isDefRecord)
	} else if obj.Equals(KEYWORDS.require) || obj.Equals(KEYWORDS._import) {
		seq = sortRequire(seq)
		seq, obj, _ = seqFirstAfterSpace(seq, w, i, isDefRecord)
		for !seq.IsEmpty() {
			seq, obj, _ = seqFirstAfterForcedBreak(seq, w, i+1)
		}
	} else if obj.Equals(SYMBOLS.catch) {
		if !seq.IsEmpty() {
			seq, _, i = seqFirstAfterSpace(seq, w, i, isDefRecord)
			seq, prevObj, i = seqFirstAfterSpace(seq, w, i, isDefRecord)
		}
	} else if obj.Equals(SYMBOLS.fn) {
		if !seq.IsEmpty() {
			switch seq.First().(type) {
			case coretypes.Vec:
				seq, prevObj, i = seqFirstAfterSpace(seq, w, i, isDefRecord)
			case coretypes.Symbol:
				seq, _, i = seqFirstAfterSpace(seq, w, i, isDefRecord)
				seq, prevObj, i = seqFirstAfterSpace(seq, w, i, isDefRecord)
			default:
				if !corert.IsNewLine(obj, seq.First()) {
					restIndent = i + 1
				}
			}
		}
	} else if obj.Equals(SYMBOLS.let) || obj.Equals(SYMBOLS.loop) {
		if v, ok := seq.First().(coretypes.Vec); ok {
			fmt.Fprint(w, " ")
			i = formatBindings(v, w, i+1)
			seq = seq.Rest()
		}
	} else if obj.Equals(SYMBOLS.letfn) {
		if v, ok := seq.First().(coretypes.Vec); ok {
			fmt.Fprint(w, " ")
			i = formatVectorVertically(v, w, i+1)
			seq = seq.Rest()
		}
	} else if isDoIndent(obj) {
		if !seq.IsEmpty() && !corert.IsNewLine(obj, seq.First()) {
			restIndent = i + 1
		}
	} else if formatAsDef {
	} else if isBodyIndent(obj) {
		restIndent = indent + 2
	} else {
		// Indent function call arguments.
		restIndent = indent + 1
		if !seq.IsEmpty() && !corert.IsNewLine(obj, seq.First()) {
			restIndent = i + 1
		}
	}

	for !seq.IsEmpty() {
		nextObj := seq.First()
		if corert.IsNewLine(obj, nextObj) {
			seq, prevObj, i = seqFirstAfterBreak(prevObj, seq, w, restIndent, isDefRecord)
		} else {
			seq, prevObj, i = seqFirstAfterSpace(seq, w, i, isDefRecord)
		}
		obj = nextObj
	}

	if corert.IsComment(obj) {
		fmt.Fprint(w, "\n")
		corert.WriteIndent(w, restIndent)
		i = restIndent
	}

	fmt.Fprint(w, ")")
	return i + 1
}

// ---- environment.go ----

var (
	Stdin          io.Reader = os.Stdin
	Stdout         io.Writer = os.Stdout
	Stderr         io.Writer = os.Stderr
	VerbosityLevel           = 0
)

type (
	Env struct {
		Namespaces    map[*string]*Namespace
		CoreNamespace *Namespace
		stdout        *Var
		stdin         *Var
		stderr        *Var
		printReadably *Var
		file          *Var
		MainFile      *Var
		args          *Var
		classPath     *Var
		ns            *Var
		NS_VAR        *Var
		IN_NS_VAR     *Var
		version       *Var
		libs          *Var
		Features      coretypes.Set
	}
)

func (env *Env) SetEnvArgs(newArgs []string) {
	args := corecollections.EmptyArrayVector()
	for _, arg := range newArgs {
		args.Append(coretypes.MakeString(arg))
	}
	if args.Count() > 0 {
		env.args.Value = args.Seq()
	} else {
		env.args.Value = NIL
	}
}

func (env *Env) SetClassPath(cp string) {
	cpVec := corecollections.EmptyArrayVector()
	for _, cpelem := range osutil.ClassPathElements(cp) {
		cpVec.Append(coretypes.MakeString(cpelem))
	}
	env.classPath.Value = cpVec
}

func (env *Env) InitEnv(stdin io.Reader, stdout, stderr io.Writer, args []string) {
	env.stdin.Value = corert.MakeBufferedReader(stdin)
	env.stdout.Value = corert.MakeIOWriter(stdout)
	env.stderr.Value = corert.MakeIOWriter(stderr)
	if vr := env.CoreNamespace.Resolve("constantly"); vr != nil {
		vr.Value = Proc{Name: "procConstantly", Fn: func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			x := args[0]
			return Proc{Name: "procConstantlyValue", Fn: func(_ []coretypes.Object) coretypes.Object { return x }}
		}}
	}
	env.SetEnvArgs(args)
}

func (env *Env) SetStdIO(stdin, stdout, stderr coretypes.Object) {
	env.stdin.Value = stdin
	env.stdout.Value = stdout
	env.stderr.Value = stderr
}

func (env *Env) StdIO() (stdin, stdout, stderr coretypes.Object) {
	return env.stdin.Value, env.stdout.Value, env.stderr.Value
}

func (env *Env) SetMainFilename(filename string) {
	env.MainFile.Value = coretypes.MakeString(filename)
}

func (env *Env) SetFilename(obj coretypes.Object) {
	env.file.Value = obj
}

func (env *Env) IsStdIn(obj coretypes.Object) bool {
	return env.stdin.Value == obj
}

func (env *Env) CurrentNamespace() *Namespace {
	return EnsureObjectIsNamespace(env.ns.Value, "")
}

func (env *Env) SetCurrentNamespace(ns *Namespace) {
	env.ns.Value = ns
}

func (env *Env) EnsureSymbolIsNamespace(sym coretypes.Symbol) *Namespace {
	if sym.NamespaceKey() != nil {
		panic(coretypes.RuntimeError("Namespace's name cannot be qualified: " + sym.ToString(false)))
	}
	nameKey := sym.NameKey()
	corert.NamespaceMu.RLock()
	ns := env.Namespaces[nameKey]
	corert.NamespaceMu.RUnlock()
	if ns != nil {
		return ns
	}
	corert.NamespaceMu.Lock()
	if env.Namespaces[nameKey] == nil {
		env.Namespaces[nameKey] = NewNamespace(sym)
	}
	ns = env.Namespaces[nameKey]
	corert.NamespaceMu.Unlock()
	return ns
}

func (env *Env) EnsureSymbolIsLib(sym coretypes.Symbol) *Namespace {
	ns := env.EnsureSymbolIsNamespace(sym)
	env.libs.Value.(*corecollections.MapSet).Add(sym)
	return ns
}

func (env *Env) NamespaceFor(ns *Namespace, s coretypes.Symbol) *Namespace {
	var res *Namespace
	if s.NamespaceKey() == nil {
		res = ns
	} else {
		nsKey := s.NamespaceKey()
		res = ns.aliases[nsKey]
		if res == nil {
			corert.NamespaceMu.RLock()
			res = env.Namespaces[nsKey]
			corert.NamespaceMu.RUnlock()
		}
	}
	if res != nil {
		res.MaybeLazy("NamespaceFor")
	}
	return res
}

func (env *Env) ResolveIn(n *Namespace, s coretypes.Symbol) (*Var, bool) {
	ns := env.NamespaceFor(n, s)
	if ns == nil {
		return nil, false
	}
	if v, ok := ns.mappings[s.NameKey()]; ok {
		traceSymbolResolve(ns, s, true)
		return v, true
	}
	if s.Equals(env.IN_NS_VAR.name) {
		traceSymbolResolve(ns, s, true)
		return env.IN_NS_VAR, true
	}
	if s.Equals(env.NS_VAR.name) {
		traceSymbolResolve(ns, s, true)
		return env.NS_VAR, true
	}
	return nil, false
}

func (env *Env) Resolve(s coretypes.Symbol) (*Var, bool) {
	return env.ResolveIn(env.CurrentNamespace(), s)
}

func (env *Env) FindNamespace(s coretypes.Symbol) *Namespace {
	if s.NamespaceKey() != nil {
		return nil
	}
	corert.NamespaceMu.RLock()
	ns := env.Namespaces[s.NameKey()]
	corert.NamespaceMu.RUnlock()
	if ns != nil {
		ns.MaybeLazy("FindNameSpace")
	}
	return ns
}

func (env *Env) RemoveNamespace(s coretypes.Symbol) *Namespace {
	if s.NamespaceKey() != nil {
		return nil
	}
	if s.Equals(SYMBOLS.joker_core) {
		panic(coretypes.RuntimeError("Cannot remove core namespace"))
	}
	nameKey := s.NameKey()
	corert.NamespaceMu.Lock()
	ns := env.Namespaces[nameKey]
	delete(env.Namespaces, nameKey)
	corert.NamespaceMu.Unlock()
	return ns
}

func (env *Env) ResolveSymbol(s coretypes.Symbol) coretypes.Symbol {
	if corestr.HasNamespaceSeparator(s.Name(), '.') {
		return s
	}
	nameKey := s.NameKey()
	nsKey := s.NamespaceKey()
	if nsKey == nil && TYPES.Contains(nameKey) {
		return s
	}
	currentNs := env.CurrentNamespace()
	if nsKey != nil {
		ns := env.NamespaceFor(currentNs, s)
		if ns == nil || ns.Name.NameKey() == nsKey {
			if ns != nil {
				ns.isUsed = true
				ns.isGloballyUsed = true
			}
			return s
		}
		ns.isUsed = true
		ns.isGloballyUsed = true
		return coretypes.MakeSymbolFromKeys(ns.Name.NameKey(), nameKey)
	}
	vr, ok := currentNs.mappings[nameKey]
	if !ok {
		return coretypes.MakeSymbolFromKeys(currentNs.Name.NameKey(), nameKey)
	}
	vr.isUsed = true
	vr.isGloballyUsed = true
	vr.ns.isUsed = true
	vr.ns.isGloballyUsed = true
	return coretypes.MakeSymbolFromKeys(vr.ns.Name.NameKey(), vr.name.NameKey())
}

func init() {
	GLOBAL_ENV.SetCurrentNamespace(GLOBAL_ENV.EnsureSymbolIsNamespace(coretypes.MakeSymbol(STRINGS.Intern, "user")))
}

// ---- goroutine_rt.go ----
// goroutine_rt.go — Per-goroutine runtime state, replacing the GIL.
//
// Each goroutine gets its own callstack and currentExpr for error reporting.
// The main goroutine uses a fast path (zero overhead when no spawned goroutines).
// Spawned goroutines use a sync.Map keyed by goroutine ID.
//
// With the GIL removed:
// - Immutable data structures (vectors, maps, lists, seqs) are already thread-safe.
// - Atoms use a per-atom mutex for swap!/reset!/compare-and-set! correctness.
// - Var.Value writes (def, alter-var-root) are rare and only safe from the main
//   goroutine or under user coordination (same semantics as Clojure's JVM runtime).
// - Namespace map mutations are protected by corert.NamespaceMu.

// goroutineRT is runtime-owned per-goroutine interpreter state.
type goroutineRT = corert.GoroutineRT

var goroutineState *corert.InterpreterStatePool

func init() {
	goroutineState = corert.NewInterpreterStatePool(corert.NewGoroutineRT(50))
}

// currentGRT returns the goroutineRT for the current goroutine.
// HOT PATH: When no spawned goroutines exist (the common case for
// single-threaded execution), returns &mainRT with a single atomic load.
func currentGRT() *goroutineRT {
	return goroutineState.Current()
}

// registerGoroutineRT sets up a new goroutineRT for the current goroutine.
// Called once at goroutine start.
func registerGoroutineRT() *goroutineRT {
	return goroutineState.Register(20)
}

// unregisterGoroutineRT removes the current goroutine's state.
// Called once at goroutine exit.
func unregisterGoroutineRT() {
	goroutineState.Unregister()
}

// ---- ns.go ----

type (
	Namespace struct {
		coretypes.MetaHolder
		Name           coretypes.Symbol
		Lazy           func()
		mappings       map[*string]*Var
		aliases        map[*string]*Namespace
		isUsed         bool
		isGloballyUsed bool
		hash           uint32
	}
)

func (ns *Namespace) ToString(escape bool) string {
	return ns.Name.ToString(escape)
}

func (ns *Namespace) Print(w io.Writer, printReadably bool) {
	fmt.Fprint(w, "#object[Namespace \""+ns.Name.ToString(true)+"\"]")
}

func (ns *Namespace) Equals(other interface{}) bool {
	return ns == other
}

func (ns *Namespace) GetInfo() *coretypes.ObjectInfo {
	return nil
}

func (ns *Namespace) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	return ns
}

func (ns *Namespace) GetType() *coretypes.Type {
	return TYPE.Namespace
}

func (ns *Namespace) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *ns
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (ns *Namespace) ResetMeta(newMeta coretypes.Map) coretypes.Map {
	ns.Meta = newMeta
	return ns.Meta
}

func (ns *Namespace) AlterMeta(fn coretypes.Callable, args []coretypes.Object) coretypes.Map {
	return AlterMeta(&ns.MetaHolder, fn, args)
}

func (ns *Namespace) Hash() uint32 {
	return ns.hash
}

func (ns *Namespace) MaybeLazy(doc string) {
	if ns.Lazy != nil {
		lazyFn := ns.Lazy
		ns.Lazy = nil
		lazyFn()
		if VerbosityLevel > 0 {
			fmt.Fprintf(Stderr, "NamespaceFor: Lazily initialized %s for %s\n", ns.Name.Name(), doc)
		}
	}
}

const nsHashMask uint32 = 0x90569f6f

func NewNamespace(sym coretypes.Symbol) *Namespace {
	return &Namespace{
		Name:     sym,
		mappings: make(map[*string]*Var),
		aliases:  make(map[*string]*Namespace),
		hash:     sym.Hash() ^ nsHashMask,
	}
}

func (ns *Namespace) Refer(sym coretypes.Symbol, vr *Var) *Var {
	if sym.NamespaceKey() != nil {
		panic(RT.NewError("Can't intern namespace-qualified symbol " + sym.ToString(false)))
	}
	ns.mappings[sym.NameKey()] = vr
	return vr
}

func (ns *Namespace) ReferAll(other *Namespace) {
	for name, vr := range other.mappings {
		if !vr.isPrivate {
			ns.mappings[name] = vr
		}
	}
}

func (ns *Namespace) InternFake(sym coretypes.Symbol) *Var {
	vr := ns.Intern(sym)
	vr.isFake = true
	return vr
}

func (ns *Namespace) Intern(sym coretypes.Symbol) *Var {
	if sym.NamespaceKey() != nil {
		panic(RT.NewError("Can't intern namespace-qualified symbol " + sym.ToString(false)))
	}
	nameKey := sym.NameKey()
	if LINTER_MODE {
		if LINTER_TYPES[nameKey] {
			msg := fmt.Sprintf("Expecting var, but %s is a type", sym.Name())
			pos := sym.GetInfo().Pos()
			printParseWarning(pos, msg)
		}
	}
	sym = sym.WithMeta(nil).(coretypes.Symbol)
	existingVar, ok := ns.mappings[nameKey]
	if !ok {
		newVar := &Var{
			ns:   ns,
			name: sym,
		}
		ns.mappings[nameKey] = newVar
		return newVar
	}
	if existingVar.ns != ns {
		if existingVar.ns.Name.Equals(SYMBOLS.joker_core) {
			newVar := &Var{
				ns:   ns,
				name: sym,
			}
			ns.mappings[nameKey] = newVar
			if !corestr.HasJokerNamespacePrefix(ns.Name.Name()) {
				printParseWarning(GetPosition(sym), fmt.Sprintf("WARNING: %s already refers to: %s in namespace %s, being replaced by: %s\n",
					sym.ToString(false), existingVar.ToString(false), ns.Name.ToString(false), newVar.ToString(false)))
			}
			return newVar
		}
		panic(RT.NewErrorWithPos(fmt.Sprintf("WARNING: %s already refers to: %s in namespace %s",
			sym.ToString(false), existingVar.ToString(false), ns.ToString(false)), sym.GetInfo().Pos()))
	}
	if LINTER_MODE && existingVar.expr != nil && !existingVar.ns.Name.Equals(SYMBOLS.joker_core) {
		if !isDeclaredInConfig(existingVar) {
			if sym.GetInfo() == nil {
				printParseWarning(existingVar.GetInfo().Pos(), "Subsequent duplicate def of "+existingVar.ToString(false))
			} else {
				printParseWarning(sym.GetInfo().Pos(), "Duplicate def of "+existingVar.ToString(false))
			}
		}
	}
	return existingVar
}

func isDeclaredInConfig(vr *Var) bool {
	m := vr.GetMeta()
	if m == nil {
		return false
	}
	ok, v := m.Get(KEYWORDS.file)
	if !ok {
		return false
	}
	s, ok := v.(coretypes.String)
	if !ok {
		return false
	}
	return corestr.IsJokerdPath(s.S)
}

func (ns *Namespace) InternVar(name string, val coretypes.Object, meta *corecollections.ArrayMap) *Var {
	vr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, name))
	vr.Value = val
	meta.Add(KEYWORDS.ns, ns)
	meta.Add(KEYWORDS.name, vr.name)
	vr.Meta = meta
	return vr
}

func (ns *Namespace) AddAlias(alias coretypes.Symbol, namespace *Namespace) {
	if alias.NamespaceKey() != nil {
		panic(RT.NewError("Alias can't be namespace-qualified"))
	}
	aliasKey := alias.NameKey()
	existing := ns.aliases[aliasKey]
	if existing != nil && existing != namespace {
		msg := "Alias " + alias.ToString(false) + " already exists in namespace " + ns.Name.ToString(false) + ", aliasing " + existing.Name.ToString(false)
		if LINTER_MODE {
			printParseError(GetPosition(alias), msg)
			return
		}
		panic(RT.NewError(msg))
	}
	ns.aliases[aliasKey] = namespace
}

func (ns *Namespace) Resolve(name string) *Var {
	return ns.mappings[STRINGS.Intern(name)]
}

func (ns *Namespace) Mappings() map[*string]*Var {
	return ns.mappings
}

func (ns *Namespace) Aliases() map[*string]*Namespace {
	return ns.aliases
}

// ---- z_doc_meta.go ----
// z_doc_meta.go — metadata hygiene for native/runtime-installed Vars.
//
// Most public vars are generated from .joke sources and carry rich metadata.
// A few runtime-installed compatibility helpers are installed directly from Go;
// make sure they still have enough metadata for doc generation and lint-style
// checks instead of surfacing as noisy <internal> warnings.

func fillNativeVarMetadata() {
	if GLOBAL_ENV == nil {
		return
	}
	corert.NamespaceMu.RLock()
	namespaces := make([]*Namespace, 0, len(GLOBAL_ENV.Namespaces))
	for _, ns := range GLOBAL_ENV.Namespaces {
		namespaces = append(namespaces, ns)
	}
	corert.NamespaceMu.RUnlock()
	for _, ns := range namespaces {
		for _, vr := range ns.mappings {
			if vr == nil || vr.ns != ns || vr.isFake {
				continue
			}
			m, _ := vr.Meta.(*corecollections.ArrayMap)
			if m == nil {
				m = corecollections.EmptyArrayMap()
				if vr.Meta != nil {
					for it := vr.Meta.Iter(); it.HasNext(); {
						p := it.Next()
						m.Add(p.Key, p.Value)
					}
				}
				vr.Meta = m
			}
			if ok, _ := m.Get(KEYWORDS.ns); !ok {
				m.Add(KEYWORDS.ns, ns)
			}
			if ok, _ := m.Get(KEYWORDS.name); !ok {
				m.Add(KEYWORDS.name, vr.name)
			}
			if vr.isPrivate {
				if ok, _ := m.Get(KEYWORDS.private); !ok {
					m.Add(KEYWORDS.private, coretypes.Boolean{B: true})
				}
				continue
			}
			if ok, _ := m.Get(KEYWORDS.added); !ok {
				m.Add(KEYWORDS.added, coretypes.MakeString("1.0"))
			}
			if ok, _ := m.Get(KEYWORDS.doc); !ok {
				m.Add(KEYWORDS.doc, coretypes.MakeString("Native runtime helper installed by go-joker."))
			}
		}
	}
}

// ---- protocol.go ----
// protocol.go — Protocol support for Clojure parity.
//
// Implements:
// - defprotocol: defines a protocol with method signatures
// - extend-type: extends a protocol to a Go type
// - satisfies?: checks if a value satisfies a protocol
//
// Protocols are represented as a Protocol object holding method name → dispatch map.
// Each method dispatch map maps Go type names to implementing functions.

// Protocol represents a Clojure-style protocol.
type Protocol struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	name    coretypes.Symbol
	methods map[string]*ProtocolMethod // method name → method descriptor
	ns      *Namespace
}

// ProtocolMethod holds one method's signature and dispatch table.
type ProtocolMethod struct {
	name        string
	arities     []int              // accepted arities (including 'this')
	dispatch    sync.Map           // type name (string) → coretypes.Callable
	defaultImpl coretypes.Callable // nil or default implementation
}

func (p *Protocol) ToString(escape bool) string {
	return fmt.Sprintf("#object[Protocol %s]", p.name.ToString(false))
}

func (p *Protocol) Equals(other interface{}) bool {
	if o, ok := other.(*Protocol); ok {
		return p == o
	}
	return false
}

func (p *Protocol) GetType() *coretypes.Type { return TYPE.Fn }
func (p *Protocol) Hash() uint32             { return hashutil.Ptr(uintptr(unsafe.Pointer(p))) }

func (p *Protocol) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *p
	res.Info = info
	return &res
}

func (p *Protocol) WithMeta(m coretypes.Map) coretypes.Object {
	res := *p
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}

// lookupMethod finds the implementation of a method for a given object.
func (pm *ProtocolMethod) lookupMethod(obj coretypes.Object) coretypes.Callable {
	typeName := typeNameOf(obj)
	if fn, ok := pm.dispatch.Load(typeName); ok {
		return fn.(coretypes.Callable)
	}
	// Try "coretypes.Object" catch-all
	if fn, ok := pm.dispatch.Load("coretypes.Object"); ok {
		return fn.(coretypes.Callable)
	}
	if pm.defaultImpl != nil {
		return pm.defaultImpl
	}
	return nil
}

// typeNameOf returns the dispatch type name for an object.
func typeNameOf(obj coretypes.Object) string {
	if obj == nil {
		return "nil"
	}
	switch obj := obj.(type) {
	case Nil:
		return "nil"
	case coretypes.Int:
		return "Int"
	case coretypes.Double:
		return "Double"
	case coretypes.Boolean:
		return "Boolean"
	case coretypes.String:
		return "String"
	case coretypes.Char:
		return "Char"
	case coretypes.Keyword:
		return "Keyword"
	case coretypes.Symbol:
		return "Symbol"
	case *coretypes.Regex:
		return "Regex"
	case *corecollections.Vector:
		return "corecollections.Vector"
	case *corecollections.ArrayVector:
		return "corecollections.Vector"
	case *corecollections.ArrayMap:
		return "Map"
	case *corecollections.HashMap:
		return "Map"
	case *corecollections.MapSet:
		return "Set"
	case *corecollections.List:
		return "corecollections.List"
	case *corecollections.LazySeq:
		return "corecollections.LazySeq"
	case *corecollections.ConsSeq:
		return "coretypes.Seq"
	case *corecollections.ArraySeq:
		return "coretypes.Seq"
	case *corecollections.MappingSeq:
		return "coretypes.Seq"
	case *Fn:
		return "Fn"
	case Proc:
		return "Fn"
	case *corert.Atom:
		return "Atom"
	case *Record:
		return obj.rtype.Name
	default:
		return obj.GetType().ToString(false)
	}
}

// makeProtocolMethod creates a dispatch proc for a protocol method.
func makeProtocolMethodProc(proto *Protocol, methodName string, pm *ProtocolMethod) Proc {
	return Proc{
		Name: proto.name.ToString(false) + "/" + methodName,
		Fn: func(args []coretypes.Object) coretypes.Object {
			if len(args) == 0 {
				panic(coretypes.RuntimeError(fmt.Sprintf("Protocol method %s/%s called with no arguments",
					proto.name.ToString(false), methodName)))
			}
			impl := pm.lookupMethod(args[0])
			if impl == nil {
				panic(coretypes.RuntimeError(fmt.Sprintf("No implementation of protocol method %s/%s for type %s",
					proto.name.ToString(false), methodName, typeNameOf(args[0]))))
			}
			return impl.Call(args)
		},
	}
}

// DefineProtocol creates a new Protocol and installs its method vars.
// Called from the defprotocol special form handler.
func DefineProtocol(ns *Namespace, name coretypes.Symbol, methods []ProtocolMethodDef) *Protocol {
	proto := &Protocol{
		name:    name,
		methods: make(map[string]*ProtocolMethod),
		ns:      ns,
	}

	for _, mdef := range methods {
		pm := &ProtocolMethod{
			name:    mdef.Name,
			arities: mdef.Arities,
		}
		proto.methods[mdef.Name] = pm

		// Install the dispatch proc as a var in the protocol's namespace
		sym := coretypes.MakeSymbol(STRINGS.Intern, mdef.Name)
		vr := ns.Intern(sym)
		vr.Value = makeProtocolMethodProc(proto, mdef.Name, pm)
	}

	// Store the protocol itself
	protoVr := ns.Intern(name)
	protoVr.Value = proto

	return proto
}

// ProtocolMethodDef defines a method in a protocol.
type ProtocolMethodDef struct {
	Name    string
	Arities []int
}

// ExtendType extends a protocol's methods for a given type name.
func ExtendType(proto *Protocol, typeName string, impls map[string]coretypes.Callable) {
	for methodName, impl := range impls {
		pm, ok := proto.methods[methodName]
		if !ok {
			panic(coretypes.RuntimeError(fmt.Sprintf("No method %s in protocol %s",
				methodName, proto.name.ToString(false))))
		}
		pm.dispatch.Store(typeName, impl)
	}
}

// Satisfies checks if an object satisfies a protocol (has implementations for all methods).
func Satisfies(proto *Protocol, obj coretypes.Object) bool {
	typeName := typeNameOf(obj)
	for _, pm := range proto.methods {
		if _, ok := pm.dispatch.Load(typeName); !ok {
			if _, ok := pm.dispatch.Load("coretypes.Object"); !ok {
				if pm.defaultImpl == nil {
					return false
				}
			}
		}
	}
	return true
}

// ---- protocol_init.go ----
// protocol_init.go — Register defprotocol, extend-type, extend-protocol, satisfies?
// as runtime procs/macros in the core namespace.

func init() {
	registerProtocolProcs()
}

func registerProtocolProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// satisfies? — checks if an object satisfies a protocol
	satVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "satisfies?"))
	satVr.Value = Proc{Name: "procSatisfiesQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		proto, ok := args[0].(*Protocol)
		if !ok {
			panic(coretypes.RuntimeError("First argument to satisfies? must be a Protocol"))
		}
		return coretypes.MakeBoolean(Satisfies(proto, args[1]))
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "satisfies?"), satVr)

	// extends? — checks if a type extends a protocol
	extVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "extends?"))
	extVr.Value = Proc{Name: "procExtendsQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		proto, ok := args[0].(*Protocol)
		if !ok {
			panic(coretypes.RuntimeError("First argument to extends? must be a Protocol"))
		}
		return coretypes.MakeBoolean(Satisfies(proto, args[1]))
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "extends?"), extVr)

	// __defprotocol — internal helper called by defprotocol macro
	// Args: [protocol-name-string method1-name arity1 method2-name arity2 ...]
	defProtoVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "__defprotocol"))
	defProtoVr.Value = Proc{Name: "procDefProtocolInternal", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 1 {
			panic(coretypes.RuntimeError("__defprotocol requires at least a name"))
		}
		name := coretypes.EnsureObjectIsSymbol(args[0], "defprotocol name must be a symbol")

		var methods []ProtocolMethodDef
		i := 1
		for i < len(args) {
			methodName := coretypes.EnsureObjectIsString(args[i], "method name must be a string").S
			i++
			if i >= len(args) {
				break
			}
			arity := coretypes.EnsureObjectIsInt(args[i], "method arity must be an int").I
			i++
			methods = append(methods, ProtocolMethodDef{
				Name:    methodName,
				Arities: []int{arity},
			})
		}

		currentNs := GLOBAL_ENV.CurrentNamespace()
		proto := DefineProtocol(currentNs, name, methods)
		return proto
	}}
	defProtoVr.isPrivate = true
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "__defprotocol"), defProtoVr)

	// __extend-type — internal helper called by extend-type macro
	// Args: [protocol type-name-string method1-name fn1 method2-name fn2 ...]
	extTypeVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "__extend-type"))
	extTypeVr.Value = Proc{Name: "procExtendTypeInternal", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			panic(coretypes.RuntimeError("__extend-type requires protocol and type-name"))
		}
		proto, ok := args[0].(*Protocol)
		if !ok {
			panic(coretypes.RuntimeError("First argument to __extend-type must be a Protocol"))
		}
		typeName := coretypes.EnsureObjectIsString(args[1], "type name must be a string").S

		if len(args[2:])%2 != 0 {
			panic(coretypes.RuntimeError("__extend-type method implementations must be name/function pairs"))
		}
		impls := make(map[string]coretypes.Callable)
		i := 2
		for i+1 < len(args) {
			methodName := coretypes.EnsureObjectIsString(args[i], "method name must be a string").S
			fn := coretypes.EnsureObjectIsCallable(args[i+1], "method implementation must be callable, got %s")
			impls[methodName] = fn
			i += 2
		}

		ExtendType(proto, typeName, impls)
		return NIL
	}}
	extTypeVr.isPrivate = true
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "__extend-type"), extTypeVr)
}

// ---- public_forms.go ----
// public_forms.go — Public macro forms for protocols and records.
//
// The runtime helpers (__defprotocol, __extend-type, __defrecord) are useful
// for bootstrapping and tests, but Clojure users expect public forms. These
// macros expand to the internal helpers and are registered early so the parser
// can resolve them before user code is parsed.

func init() {
	registerPublicParityMacros()
}

func registerPublicParityMacros() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}
	installMacro(ns, "defprotocol", macroDefProtocol)
	installMacro(ns, "extend-type", macroExtendType)
	installMacro(ns, "extend-protocol", macroExtendProtocol)
	installMacro(ns, "defrecord", macroDefRecord)
}

func installMacro(ns *Namespace, name string, fn func([]coretypes.Object) coretypes.Object) {
	sym := coretypes.MakeSymbol(STRINGS.Intern, name)
	vr := ns.Intern(sym)
	vr.Value = Proc{Name: "macro" + name, Fn: fn}
	vr.isMacro = true
	referToUser(sym, vr)
}

func listObjs(objs ...coretypes.Object) *corecollections.List {
	return corecollections.NewListFrom(objs...)
}
func quoteObj(obj coretypes.Object) *corecollections.List {
	return listObjs(coretypes.MakeSymbol(STRINGS.Intern, "quote"), obj)
}
func doObj(forms ...coretypes.Object) *corecollections.List {
	return corecollections.NewListFrom(append([]coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "do")}, forms...)...)
}

func macroDefProtocol(args []coretypes.Object) coretypes.Object {
	// macro args: &form, &env, name, method...
	if len(args) < 3 {
		panic(coretypes.RuntimeError("defprotocol requires a name"))
	}
	name, ok := args[2].(coretypes.Symbol)
	if !ok {
		panic(coretypes.RuntimeError("defprotocol name must be a symbol"))
	}
	forms := []coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "__defprotocol"), quoteObj(name)}
	for _, raw := range args[3:] {
		seqable, ok := raw.(coretypes.Seqable)
		if !ok {
			continue // docstrings/options are ignored by the compact runtime protocol helper
		}
		s := seqable.Seq()
		if s.IsEmpty() {
			continue
		}
		mname, ok := s.First().(coretypes.Symbol)
		if !ok {
			continue
		}
		s = s.Rest()
		if s.IsEmpty() {
			continue
		}
		argv, ok := s.First().(coretypes.Counted)
		if !ok {
			continue
		}
		forms = append(forms, coretypes.String{S: mname.ToString(false)}, coretypes.Int{I: argv.Count()})
	}
	return corecollections.NewListFrom(forms...)
}

func macroExtendType(args []coretypes.Object) coretypes.Object {
	// (extend-type Type Proto (method [args] body...) Proto2 ...)
	if len(args) < 5 {
		panic(coretypes.RuntimeError("extend-type requires a type, protocol, and method implementations"))
	}
	typeName := macroTypeName(args[2])
	forms := make([]coretypes.Object, 0)
	i := 3
	for i < len(args) {
		proto := args[i]
		i++
		call := []coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "__extend-type"), proto, coretypes.String{S: typeName}}
		for i < len(args) {
			if _, isProto := args[i].(coretypes.Symbol); isProto && i+1 < len(args) {
				if _, nextIsMethod := args[i+1].(coretypes.Seqable); nextIsMethod {
					break
				}
			}
			method, ok := args[i].(coretypes.Seqable)
			if !ok {
				break
			}
			s := method.Seq()
			if s.IsEmpty() {
				i++
				continue
			}
			mname, ok := s.First().(coretypes.Symbol)
			if !ok {
				i++
				continue
			}
			fnTail := corecollections.ToSlice(s.Rest())
			fnForm := corecollections.NewListFrom(append([]coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "fn")}, fnTail...)...)
			call = append(call, coretypes.String{S: mname.ToString(false)}, fnForm)
			i++
		}
		forms = append(forms, corecollections.NewListFrom(call...))
	}
	return doObj(forms...)
}

func macroExtendProtocol(args []coretypes.Object) coretypes.Object {
	// (extend-protocol Proto Type (method [args] body...) Type2 ...)
	if len(args) < 5 {
		panic(coretypes.RuntimeError("extend-protocol requires a protocol, type, and method implementations"))
	}
	proto := args[2]
	forms := make([]coretypes.Object, 0)
	i := 3
	for i < len(args) {
		typeName := macroTypeName(args[i])
		i++
		call := []coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "__extend-type"), proto, coretypes.String{S: typeName}}
		for i < len(args) {
			method, ok := args[i].(coretypes.Seqable)
			if !ok {
				break
			}
			s := method.Seq()
			if s.IsEmpty() {
				i++
				continue
			}
			mname, ok := s.First().(coretypes.Symbol)
			if !ok {
				i++
				continue
			}
			fnTail := corecollections.ToSlice(s.Rest())
			fnForm := corecollections.NewListFrom(append([]coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "fn")}, fnTail...)...)
			call = append(call, coretypes.String{S: mname.ToString(false)}, fnForm)
			i++
			// Stop if the next form looks like a type followed by methods. In practice
			// a new type is a symbol/string/keyword and a method implementation is a list.
			if i < len(args) {
				if _, ok := args[i].(coretypes.Seqable); !ok {
					break
				}
			}
		}
		forms = append(forms, corecollections.NewListFrom(call...))
	}
	return doObj(forms...)
}

func macroDefRecord(args []coretypes.Object) coretypes.Object {
	// (defrecord Name [fields] Protocol (method [args] body...) ...)
	if len(args) < 4 {
		panic(coretypes.RuntimeError("defrecord requires a name and fields vector"))
	}
	name, ok := args[2].(coretypes.Symbol)
	if !ok {
		panic(coretypes.RuntimeError("defrecord name must be a symbol"))
	}
	fieldsSeq, ok := args[3].(coretypes.Seqable)
	if !ok {
		panic(coretypes.RuntimeError("defrecord fields must be seqable"))
	}
	defCall := []coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "__defrecord"), quoteObj(name)}
	for s := fieldsSeq.Seq(); !s.IsEmpty(); s = s.Rest() {
		field, ok := s.First().(coretypes.Symbol)
		if !ok {
			panic(coretypes.RuntimeError("defrecord field must be a symbol"))
		}
		defCall = append(defCall, coretypes.String{S: field.ToString(false)})
	}
	forms := []coretypes.Object{corecollections.NewListFrom(defCall...)}
	if len(args) > 4 {
		// Reuse extend-type semantics with the record's dispatch type name.
		extendArgs := append([]coretypes.Object{args[0], args[1], name}, args[4:]...)
		forms = append(forms, macroExtendType(extendArgs))
	}
	return doObj(forms...)
}

func macroTypeName(obj coretypes.Object) string {
	switch t := obj.(type) {
	case coretypes.Symbol:
		return t.ToString(false)
	case coretypes.String:
		return t.S
	case coretypes.Keyword:
		return t.ToString(false)[1:]
	default:
		return obj.ToString(false)
	}
}

// ---- record.go ----
// record.go — Record support for Clojure parity.
//
// A Record is a named, typed map with fixed fields plus optional extension fields.
// Records support:
// - Keyword access: (:field record)
// - get/assoc/dissoc (dissoc to extension fields only; dissoc of base field returns plain map)
// - coretypes.Equality by type + fields
// - Protocol satisfaction via extend-type with the record's type name

// Record is an instance of a RecordType.
type Record struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	rtype *coretypes.RecordType
	bases []coretypes.Object        // values for base fields (same order as rtype.fields)
	ext   *corecollections.ArrayMap // extension fields (nil if none)
}

func (r *Record) ToString(escape bool) string {
	var b strings.Builder
	b.WriteString("#")
	b.WriteString(r.rtype.Name)
	b.WriteString("{")
	first := true
	for i, fname := range r.rtype.Fields {
		if !first {
			b.WriteString(", ")
		}
		first = false
		b.WriteString(":")
		b.WriteString(fname)
		b.WriteString(" ")
		b.WriteString(r.bases[i].ToString(escape))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			if !first {
				b.WriteString(", ")
			}
			first = false
			b.WriteString(p.Key.ToString(escape))
			b.WriteString(" ")
			b.WriteString(p.Value.ToString(escape))
		}
	}
	b.WriteString("}")
	return b.String()
}

func (r *Record) Equals(other interface{}) bool {
	o, ok := other.(*Record)
	if !ok {
		return false
	}
	if r.rtype != o.rtype {
		return false
	}
	for i := range r.bases {
		if !r.bases[i].Equals(o.bases[i]) {
			return false
		}
	}
	// Compare extension fields
	if r.ext == nil && o.ext == nil {
		return true
	}
	if r.ext == nil || o.ext == nil {
		rCount := 0
		oCount := 0
		if r.ext != nil {
			rCount = r.ext.Count()
		}
		if o.ext != nil {
			oCount = o.ext.Count()
		}
		return rCount == 0 && oCount == 0
	}
	return r.ext.Equals(o.ext)
}

func (r *Record) GetType() *coretypes.Type { return TYPE.ArrayMap }
func (r *Record) Hash() uint32 {
	h := uint32(0x9e3779b9)
	for _, v := range r.bases {
		h = h*31 + v.Hash()
	}
	if r.ext != nil {
		h = h*31 + r.ext.Hash()
	}
	return h
}

func (r *Record) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := r.clone()
	res.Info = info
	return res
}

func (r *Record) WithMeta(m coretypes.Map) coretypes.Object {
	res := r.clone()
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return res
}

func (r *Record) clone() *Record {
	bases := make([]coretypes.Object, len(r.bases))
	copy(bases, r.bases)
	var ext *corecollections.ArrayMap
	if r.ext != nil {
		ext = r.ext.Clone()
	}
	return &Record{
		InfoHolder: r.InfoHolder,
		MetaHolder: r.MetaHolder,
		rtype:      r.rtype,
		bases:      bases,
		ext:        ext,
	}
}

// --- coretypes.Map interface ---

// Get implements coretypes.Gettable for keyword access.
func (r *Record) Get(key coretypes.Object) (bool, coretypes.Object) {
	if kw, ok := key.(coretypes.Keyword); ok {
		name := kw.ToString(false)[1:] // strip leading ":"
		if idx, ok := r.rtype.FieldIdx[name]; ok {
			return true, r.bases[idx]
		}
	}
	if r.ext != nil {
		return r.ext.Get(key)
	}
	return false, nil
}

// EntryAt returns a MapEntry for the given key.
func (r *Record) EntryAt(key coretypes.Object) coretypes.Object {
	if ok, v := r.Get(key); ok {
		av := corecollections.EmptyArrayVector().Conj(key).(*corecollections.ArrayVector).Conj(v).(*corecollections.ArrayVector)
		return av
	}
	return nil
}

// Assoc returns a new record with the key set to val.
// If key is a base field, returns a new record. Otherwise extends.
func (r *Record) Assoc(key, val coretypes.Object) coretypes.Associative {
	if kw, ok := key.(coretypes.Keyword); ok {
		name := kw.ToString(false)[1:]
		if idx, ok := r.rtype.FieldIdx[name]; ok {
			res := r.clone()
			res.bases[idx] = val
			return res
		}
	}
	res := r.clone()
	if res.ext == nil {
		res.ext = corecollections.EmptyArrayMap()
	}
	res.ext = res.ext.Assoc(key, val).(*corecollections.ArrayMap)
	return res
}

// Count returns the number of fields (base + extension).
func (r *Record) Count() int {
	n := len(r.bases)
	if r.ext != nil {
		n += r.ext.Count()
	}
	return n
}

// coretypes.Seq returns a sequence of MapEntry pairs.
func (r *Record) Seq() coretypes.Seq {
	entries := make([]coretypes.Object, 0, r.Count())
	for i, fname := range r.rtype.Fields {
		entries = append(entries, corecollections.NewVectorFrom(coretypes.MakeKeyword(STRINGS.Intern, fname), r.bases[i]))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			entries = append(entries, corecollections.NewVectorFrom(p.Key, p.Value))
		}
	}
	return &corecollections.ArraySeq{Arr: entries, Index: 0}
}

// Keys returns all keys.
func (r *Record) Keys() coretypes.Seq {
	keys := make([]coretypes.Object, 0, r.Count())
	for _, fname := range r.rtype.Fields {
		keys = append(keys, coretypes.MakeKeyword(STRINGS.Intern, fname))
	}
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			keys = append(keys, p.Key)
		}
	}
	return &corecollections.ArraySeq{Arr: keys, Index: 0}
}

// Vals returns all values.
func (r *Record) Vals() coretypes.Seq {
	vals := make([]coretypes.Object, 0, r.Count())
	vals = append(vals, r.bases...)
	if r.ext != nil {
		for iter := r.ext.Iter(); iter.HasNext(); {
			p := iter.Next()
			vals = append(vals, p.Value)
		}
	}
	return &corecollections.ArraySeq{Arr: vals, Index: 0}
}

// Conj adds a map entry to the record.
func (r *Record) Conj(obj coretypes.Object) coretypes.Conjable {
	switch v := obj.(type) {
	case *corecollections.Vector:
		if v.Count() != 2 {
			panic(coretypes.RuntimeError("corecollections.Vector arg to conj on record must be a pair"))
		}
		return r.Assoc(v.At(0), v.At(1)).(coretypes.Conjable)
	}
	panic(coretypes.RuntimeError(fmt.Sprintf("Cannot conj %s onto record", obj.GetType().ToString(false))))
}

// Call implements keyword-style access: (record :field)
func (r *Record) Call(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 2)
	ok, v := r.Get(args[0])
	if ok {
		return v
	}
	if len(args) == 2 {
		return args[1]
	}
	return NIL
}

// Merge merges a map into the record.
func (r *Record) Merge(other coretypes.Map) coretypes.Map {
	res := r.clone()
	for iter := other.Iter(); iter.HasNext(); {
		p := iter.Next()
		assocResult := res.Assoc(p.Key, p.Value)
		res = assocResult.(*Record)
	}
	return res
}

// Iter returns a map iterator.
func (r *Record) Iter() coretypes.MapIterator {
	return &recordIterator{r: r, idx: 0}
}

// Containskey
func (r *Record) ContainsKey(key coretypes.Object) bool {
	ok, _ := r.Get(key)
	return ok
}

// Without (dissoc) — dissoc of a base field returns a plain map
func (r *Record) Without(key coretypes.Object) coretypes.Map {
	if kw, ok := key.(coretypes.Keyword); ok {
		name := kw.ToString(false)[1:]
		if _, ok := r.rtype.FieldIdx[name]; ok {
			// Dissoc base field → degrade to plain map
			m := corecollections.EmptyArrayMap()
			for i, fname := range r.rtype.Fields {
				if fname != name {
					m.Add(coretypes.MakeKeyword(STRINGS.Intern, fname), r.bases[i])
				}
			}
			if r.ext != nil {
				for iter := r.ext.Iter(); iter.HasNext(); {
					p := iter.Next()
					m.Add(p.Key, p.Value)
				}
			}
			return m
		}
	}
	if r.ext != nil {
		res := r.clone()
		res.ext = res.ext.Without(key).(*corecollections.ArrayMap)
		return res
	}
	return r
}

type recordIterator struct {
	r       *Record
	idx     int
	extIter coretypes.MapIterator
}

func (it *recordIterator) HasNext() bool {
	if it.idx < len(it.r.rtype.Fields) {
		return true
	}
	if it.r.ext != nil {
		if it.extIter == nil {
			it.extIter = it.r.ext.Iter()
		}
		return it.extIter.HasNext()
	}
	return false
}

func (it *recordIterator) Next() *coretypes.Pair {
	if it.idx < len(it.r.rtype.Fields) {
		p := &coretypes.Pair{
			Key:   coretypes.MakeKeyword(STRINGS.Intern, it.r.rtype.Fields[it.idx]),
			Value: it.r.bases[it.idx],
		}
		it.idx++
		return p
	}
	if it.extIter == nil {
		it.extIter = it.r.ext.Iter()
	}
	return it.extIter.Next()
}

// NewRecord creates a new record instance.
func NewRecord(rtype *coretypes.RecordType, fields []coretypes.Object) *Record {
	if len(fields) != len(rtype.Fields) {
		panic(coretypes.RuntimeError(fmt.Sprintf("Wrong number of fields for record %s: expected %d, got %d",
			rtype.Name, len(rtype.Fields), len(fields))))
	}
	bases := make([]coretypes.Object, len(fields))
	copy(bases, fields)
	return &Record{rtype: rtype, bases: bases}
}

// ---- record_init.go ----
// record_init.go — Register __defrecord and record constructors.

func init() {
	registerRecordProcs()
}

func registerRecordProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// record? — always available
	recordQVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "record?"))
	recordQVr.Value = Proc{Name: "procRecordQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		_, ok := args[0].(*Record)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "record?"), recordQVr)

	// __defrecord — internal helper
	// Args: [record-name-symbol field1-string field2-string ...]
	// Returns: the RecordType, and installs:
	//   - ->RecordName constructor fn
	//   - map->RecordName factory fn
	defRecordVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "__defrecord"))
	defRecordVr.Value = Proc{Name: "procDefRecordInternal", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 1 {
			panic(coretypes.RuntimeError("__defrecord requires at least a name"))
		}
		name := coretypes.EnsureObjectIsSymbol(args[0], "defrecord name must be a symbol")
		nameStr := name.ToString(false)

		fields := make([]string, len(args)-1)
		for i := 1; i < len(args); i++ {
			fields[i-1] = coretypes.EnsureObjectIsString(args[i], "field name must be a string").S
		}

		rtype := coretypes.MakeRecordType(nameStr, fields)

		currentNs := GLOBAL_ENV.CurrentNamespace()

		// Install positional constructor: (->RecordName field1 field2 ...)
		ctorName := "->" + nameStr
		ctorVr := currentNs.Intern(coretypes.MakeSymbol(STRINGS.Intern, ctorName))
		ctorVr.Value = Proc{Name: "proc" + ctorName, Fn: func(ctorArgs []coretypes.Object) coretypes.Object {
			return NewRecord(rtype, ctorArgs)
		}}

		// Install map factory: (map->RecordName {:field1 v1 :field2 v2})
		mapCtorName := "map->" + nameStr
		mapCtorVr := currentNs.Intern(coretypes.MakeSymbol(STRINGS.Intern, mapCtorName))
		mapCtorVr.Value = Proc{Name: "proc" + mapCtorName, Fn: func(ctorArgs []coretypes.Object) coretypes.Object {
			runtimeCheckArity(ctorArgs, 1, 1)
			m := coretypes.EnsureObjectIsMap(ctorArgs[0], "map->"+nameStr+" requires a map argument")
			vals := make([]coretypes.Object, len(fields))
			for i, fname := range fields {
				kw := coretypes.MakeKeyword(STRINGS.Intern, fname)
				if ok, v := m.Get(kw); ok {
					vals[i] = v
				} else {
					vals[i] = NIL
				}
			}
			rec := NewRecord(rtype, vals)
			// Add any extra keys as extension fields
			for iter := m.Iter(); iter.HasNext(); {
				p := iter.Next()
				if kw, ok := p.Key.(coretypes.Keyword); ok {
					kwName := kw.ToString(false)[1:]
					if _, isBase := rtype.FieldIdx[kwName]; isBase {
						continue
					}
				}
				rec = rec.Assoc(p.Key, p.Value).(*Record)
			}
			return rec
		}}

		return NIL
	}}
	defRecordVr.isPrivate = true
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "__defrecord"), defRecordVr)
}

// ---- hierarchy.go ----
// hierarchy.go — Clojure hierarchy support for isa?/derive/underive.
//
// A hierarchy is a directed acyclic graph (DAG) of parent-child
// relationships between keywords and symbols. The global hierarchy
// is stored as a var and used by default for isa?/derive/underive.

// Hierarchy represents a Clojure hierarchy.
type Hierarchy struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	mu         sync.RWMutex
	parents    map[string]map[string]bool  // child key → set of parent keys
	parentKeys map[string]coretypes.Object // key → object (for iteration)
	childKeys  map[string]coretypes.Object
}

func MakeHierarchy() *Hierarchy {
	return &Hierarchy{
		parents:    make(map[string]map[string]bool),
		parentKeys: make(map[string]coretypes.Object),
		childKeys:  make(map[string]coretypes.Object),
	}
}

func (h *Hierarchy) ToString(escape bool) string   { return "#object[Hierarchy]" }
func (h *Hierarchy) Equals(other interface{}) bool { return h == other }
func (h *Hierarchy) GetType() *coretypes.Type      { return TYPE.Fn }
func (h *Hierarchy) Hash() uint32                  { return hashutil.Ptr(uintptr(unsafe.Pointer(h))) }
func (h *Hierarchy) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	h.Info = info
	return h
}
func (h *Hierarchy) WithMeta(m coretypes.Map) coretypes.Object {
	h.Meta = coretypes.SafeMerge(h.Meta, m)
	return h
}

func objKey(obj coretypes.Object) string {
	if obj == nil {
		return "nil"
	}
	return obj.GetType().ToString(false) + "|" + obj.ToString(false)
}

// Derive adds a parent relationship: child isa? parent
func (h *Hierarchy) Derive(child, parent coretypes.Object) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ck := objKey(child)
	pk := objKey(parent)

	if h.parents[ck] == nil {
		h.parents[ck] = make(map[string]bool)
	}
	h.parents[ck][pk] = true
	h.parentKeys[pk] = parent
	h.childKeys[ck] = child
}

// Underive removes a parent relationship.
func (h *Hierarchy) Underive(child, parent coretypes.Object) {
	h.mu.Lock()
	defer h.mu.Unlock()

	ck := objKey(child)
	pk := objKey(parent)

	if ps, ok := h.parents[ck]; ok {
		delete(ps, pk)
		if len(ps) == 0 {
			delete(h.parents, ck)
		}
	}
}

// IsA checks if child isa? parent (direct or transitive).
func (h *Hierarchy) IsA(child, parent coretypes.Object) bool {
	if child.Equals(parent) {
		return true
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.isALocked(objKey(child), objKey(parent), make(map[string]bool))
}

func (h *Hierarchy) isALocked(ck, pk string, visited map[string]bool) bool {
	if visited[ck] {
		return false
	}
	visited[ck] = true

	ps, ok := h.parents[ck]
	if !ok {
		return false
	}
	if ps[pk] {
		return true
	}
	// Transitive check
	for parentKey := range ps {
		if h.isALocked(parentKey, pk, visited) {
			return true
		}
	}
	return false
}

// Parents returns direct parents of tag.
func (h *Hierarchy) Parents(tag coretypes.Object) []coretypes.Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	tk := objKey(tag)
	ps, ok := h.parents[tk]
	if !ok {
		return nil
	}
	result := make([]coretypes.Object, 0, len(ps))
	for pk := range ps {
		if obj, ok := h.parentKeys[pk]; ok {
			result = append(result, obj)
		}
	}
	return result
}

// Ancestors returns all transitive ancestors of tag.
func (h *Hierarchy) Ancestors(tag coretypes.Object) []coretypes.Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]coretypes.Object, 0)
	visited := make(map[string]bool)
	h.collectAncestors(objKey(tag), &result, visited)
	return result
}

func (h *Hierarchy) collectAncestors(tk string, result *[]coretypes.Object, visited map[string]bool) {
	ps, ok := h.parents[tk]
	if !ok {
		return
	}
	for pk := range ps {
		if !visited[pk] {
			visited[pk] = true
			if obj, ok := h.parentKeys[pk]; ok {
				*result = append(*result, obj)
			}
			h.collectAncestors(pk, result, visited)
		}
	}
}

// Descendants returns all transitive descendants of tag.
func (h *Hierarchy) Descendants(tag coretypes.Object) []coretypes.Object {
	h.mu.RLock()
	defer h.mu.RUnlock()

	pk := objKey(tag)
	result := make([]coretypes.Object, 0)
	visited := make(map[string]bool)

	for ck, ps := range h.parents {
		if ps[pk] && !visited[ck] {
			visited[ck] = true
			if obj, ok := h.childKeys[ck]; ok {
				result = append(result, obj)
			}
			h.collectDescendants(ck, &result, visited)
		}
	}
	return result
}

func (h *Hierarchy) collectDescendants(pk string, result *[]coretypes.Object, visited map[string]bool) {
	for ck, ps := range h.parents {
		if ps[pk] && !visited[ck] {
			visited[ck] = true
			if obj, ok := h.childKeys[ck]; ok {
				*result = append(*result, obj)
			}
			h.collectDescendants(ck, result, visited)
		}
	}
}

// Global hierarchy
var globalHierarchy = MakeHierarchy()

// ---- hierarchy_init.go ----
// hierarchy_init.go — Register derive, underive, isa?, ancestors, descendants, parents, make-hierarchy.

func init() {
	registerHierarchyProcs()
}

func registerHierarchyProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// make-hierarchy
	mhVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "make-hierarchy"))
	mhVr.Value = Proc{Name: "procMakeHierarchy", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 0, 0)
		return MakeHierarchy()
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "make-hierarchy"), mhVr)

	// derive — (derive child parent) or (derive h child parent)
	deriveVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "derive"))
	deriveVr.Value = Proc{Name: "procDerive", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 2:
			globalHierarchy.Derive(args[0], args[1])
			return NIL
		case 3:
			h, ok := args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 3-arity derive must be a hierarchy"))
			}
			h.Derive(args[1], args[2])
			return h
		default:
			PanicArityMinMax(len(args), 2, 3)
			return NIL
		}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "derive"), deriveVr)

	// underive — (underive child parent) or (underive h child parent)
	underiveVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "underive"))
	underiveVr.Value = Proc{Name: "procUnderive", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 2:
			globalHierarchy.Underive(args[0], args[1])
			return NIL
		case 3:
			h, ok := args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 3-arity underive must be a hierarchy"))
			}
			h.Underive(args[1], args[2])
			return h
		default:
			PanicArityMinMax(len(args), 2, 3)
			return NIL
		}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "underive"), underiveVr)

	// isa? — (isa? child parent) or (isa? h child parent)
	isaVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "isa?"))
	isaVr.Value = Proc{Name: "procIsaQ", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 2:
			return coretypes.MakeBoolean(globalHierarchy.IsA(args[0], args[1]))
		case 3:
			h, ok := args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 3-arity isa? must be a hierarchy"))
			}
			return coretypes.MakeBoolean(h.IsA(args[1], args[2]))
		default:
			PanicArityMinMax(len(args), 2, 3)
			return NIL
		}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "isa?"), isaVr)

	// parents — (parents tag) or (parents h tag)
	parentsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "parents"))
	parentsVr.Value = Proc{Name: "procParents", Fn: func(args []coretypes.Object) coretypes.Object {
		var h *Hierarchy
		var tag coretypes.Object
		switch len(args) {
		case 1:
			h = globalHierarchy
			tag = args[0]
		case 2:
			var ok bool
			h, ok = args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 2-arity parents must be a hierarchy"))
			}
			tag = args[1]
		default:
			PanicArityMinMax(len(args), 1, 2)
			return NIL
		}
		ps := h.Parents(tag)
		if len(ps) == 0 {
			return NIL
		}
		s := corecollections.EmptySet()
		for _, p := range ps {
			s = s.Conj(p).(*corecollections.MapSet)
		}
		return s
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "parents"), parentsVr)

	// ancestors — (ancestors tag) or (ancestors h tag)
	ancestorsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "ancestors"))
	ancestorsVr.Value = Proc{Name: "procAncestors", Fn: func(args []coretypes.Object) coretypes.Object {
		var h *Hierarchy
		var tag coretypes.Object
		switch len(args) {
		case 1:
			h = globalHierarchy
			tag = args[0]
		case 2:
			var ok bool
			h, ok = args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 2-arity ancestors must be a hierarchy"))
			}
			tag = args[1]
		default:
			PanicArityMinMax(len(args), 1, 2)
			return NIL
		}
		as := h.Ancestors(tag)
		if len(as) == 0 {
			return NIL
		}
		s := corecollections.EmptySet()
		for _, a := range as {
			s = s.Conj(a).(*corecollections.MapSet)
		}
		return s
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "ancestors"), ancestorsVr)

	// descendants — (descendants tag) or (descendants h tag)
	descendantsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "descendants"))
	descendantsVr.Value = Proc{Name: "procDescendants", Fn: func(args []coretypes.Object) coretypes.Object {
		var h *Hierarchy
		var tag coretypes.Object
		switch len(args) {
		case 1:
			h = globalHierarchy
			tag = args[0]
		case 2:
			var ok bool
			h, ok = args[0].(*Hierarchy)
			if !ok {
				panic(coretypes.RuntimeError("First argument to 2-arity descendants must be a hierarchy"))
			}
			tag = args[1]
		default:
			PanicArityMinMax(len(args), 1, 2)
			return NIL
		}
		ds := h.Descendants(tag)
		if len(ds) == 0 {
			return NIL
		}
		s := corecollections.EmptySet()
		for _, d := range ds {
			s = s.Conj(d).(*corecollections.MapSet)
		}
		return s
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "descendants"), descendantsVr)
}

// ---- procs.go ----

func FailArg(obj coretypes.Object, typeName string, index int) *corert.EvalError {
	return RT.NewArgTypeError(index, obj, typeName)
}

func FailObject(obj coretypes.Object, typeName, pattern string) *corert.EvalError {
	if pattern == "" {
		pattern = "%s"
	}
	msg := fmt.Sprintf("Expected %s, got %s", typeName, obj.GetType().ToString(false))
	return RT.NewError(fmt.Sprintf(pattern, msg))
}

func installAssertionErrors() {
	coretypes.AssertionFailArg = func(obj coretypes.Object, typeName string, index int) any {
		return FailArg(obj, typeName, index)
	}
	coretypes.AssertionFailObject = func(obj coretypes.Object, typeName, pattern string) any {
		return FailObject(obj, typeName, pattern)
	}
}

var procMeta = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	switch obj := args[0].(type) {
	case coretypes.Meta:
		meta := obj.GetMeta()
		if meta != nil {
			return meta
		}
	}
	return NIL
}

var procWithMeta = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 2)
	m := coretypes.EnsureArgIsMeta(args, 0)
	if args[1].Equals(NIL) {
		return args[0]
	}
	return m.WithMeta(coretypes.EnsureArgIsMap(args, 1))
}

var procIsZero = func(args []coretypes.Object) coretypes.Object {
	switch n := args[0].(type) {
	case coretypes.Int:
		return coretypes.Boolean{B: n.I == 0}
	case coretypes.Double:
		return coretypes.Boolean{B: n.D == 0}
	}
	n := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(n)
	return coretypes.Boolean{B: ops.IsZero(n)}
}

var procIsPos = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(n)
	return coretypes.Boolean{B: ops.Gt(n, coretypes.Int{I: 0})}
}

var procIsNeg = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(n)
	return coretypes.Boolean{B: ops.Lt(n, coretypes.Int{I: 0})}
}

var procAdd = func(args []coretypes.Object) coretypes.Object {
	switch x := args[0].(type) {
	case coretypes.Int:
		switch y := args[1].(type) {
		case coretypes.Int:
			return coretypes.INT_OPS.Add(x, y)
		case coretypes.Double:
			return coretypes.Double{D: float64(x.I) + y.D}
		}
	case coretypes.Double:
		switch y := args[1].(type) {
		case coretypes.Int:
			return coretypes.Double{D: x.D + float64(y.I)}
		case coretypes.Double:
			return coretypes.Double{D: x.D + y.D}
		}
	}
	x := coretypes.EnsureObjectIsNumber(args[0], "")
	y := coretypes.EnsureObjectIsNumber(args[1], "")
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y))
	return ops.Add(x, y)
}

var procAddEx = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureObjectIsNumber(args[0], "")
	y := coretypes.EnsureObjectIsNumber(args[1], "")
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y)).Combine(coretypes.BIGINT_OPS)
	return ops.Add(x, y)
}

var procMultiply = func(args []coretypes.Object) coretypes.Object {
	switch x := args[0].(type) {
	case coretypes.Int:
		switch y := args[1].(type) {
		case coretypes.Int:
			return coretypes.INT_OPS.Multiply(x, y)
		case coretypes.Double:
			return coretypes.Double{D: float64(x.I) * y.D}
		}
	case coretypes.Double:
		switch y := args[1].(type) {
		case coretypes.Int:
			return coretypes.Double{D: x.D * float64(y.I)}
		case coretypes.Double:
			return coretypes.Double{D: x.D * y.D}
		}
	}
	x := coretypes.EnsureObjectIsNumber(args[0], "")
	y := coretypes.EnsureObjectIsNumber(args[1], "")
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y))
	return ops.Multiply(x, y)
}

var procMultiplyEx = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureObjectIsNumber(args[0], "")
	y := coretypes.EnsureObjectIsNumber(args[1], "")
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y)).Combine(coretypes.BIGINT_OPS)
	return ops.Multiply(x, y)
}

var procSubtract = func(args []coretypes.Object) coretypes.Object {
	if len(args) == 1 {
		switch x := args[0].(type) {
		case coretypes.Int:
			return coretypes.INT_OPS.Subtract(coretypes.Int{I: 0}, x)
		case coretypes.Double:
			return coretypes.Double{D: -x.D}
		}
		a := coretypes.Int{I: 0}
		b := coretypes.EnsureObjectIsNumber(args[0], "")
		ops := coretypes.GetOps(a).Combine(coretypes.GetOps(b))
		return ops.Subtract(a, b)
	}
	switch a := args[0].(type) {
	case coretypes.Int:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.INT_OPS.Subtract(a, b)
		case coretypes.Double:
			return coretypes.Double{D: float64(a.I) - b.D}
		}
	case coretypes.Double:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.Double{D: a.D - float64(b.I)}
		case coretypes.Double:
			return coretypes.Double{D: a.D - b.D}
		}
	}
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	ops := coretypes.GetOps(a).Combine(coretypes.GetOps(b))
	return ops.Subtract(a, b)
}

var procSubtractEx = func(args []coretypes.Object) coretypes.Object {
	var a, b coretypes.Object
	if len(args) == 1 {
		a = coretypes.Int{I: 0}
		b = args[0]
	} else {
		a = args[0]
		b = args[1]
	}
	an := coretypes.EnsureObjectIsNumber(a, "")
	bn := coretypes.EnsureObjectIsNumber(b, "")
	ops := coretypes.GetOps(an).Combine(coretypes.GetOps(bn)).Combine(coretypes.BIGINT_OPS)
	return ops.Subtract(an, bn)
}

var procDivide = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureArgIsNumber(args, 0)
	y := coretypes.EnsureArgIsNumber(args, 1)
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y))
	return ops.Divide(x, y)
}

var procQuot = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureArgIsNumber(args, 0)
	y := coretypes.EnsureArgIsNumber(args, 1)
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y))
	return ops.Quotient(x, y)
}

var procRem = func(args []coretypes.Object) coretypes.Object {
	switch x := args[0].(type) {
	case coretypes.Int:
		if y, ok := args[1].(coretypes.Int); ok {
			if y.I == 0 {
				coretypes.PanicOnZero(coretypes.INT_OPS, y)
			}
			return coretypes.Int{I: x.I % y.I}
		}
	}
	x := coretypes.EnsureArgIsNumber(args, 0)
	y := coretypes.EnsureArgIsNumber(args, 1)
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y))
	return ops.Rem(x, y)
}

var procBitNot = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureObjectIsInt(args[0], "Bit operation not supported for "+args[0].GetType().ToString(false))
	return coretypes.Int{I: ^x.I}
}

func EnsureObjectIsInts(args []coretypes.Object) (coretypes.Int, coretypes.Int) {
	x := coretypes.EnsureObjectIsInt(args[0], "Bit operation not supported: %s")
	y := coretypes.EnsureObjectIsInt(args[1], "Bit operation not supported: %s")
	return x, y
}

var procBitAnd = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I & y.I}
}

var procBitOr = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I | y.I}
}

var procBitXor = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I ^ y.I}
}

var procBitAndNot = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I &^ y.I}
}

func checkedBitIndex(index int, op string) uint {
	if index < 0 {
		panic(RT.NewError(op + " bit index must be non-negative"))
	}
	if index >= strconv.IntSize {
		panic(RT.NewError(op + " bit index is too large"))
	}
	return uint(index)
}

func checkedShiftCount(count int, op string) uint {
	if count < 0 {
		panic(RT.NewError(op + " shift count must be non-negative"))
	}
	return uint(count)
}

var procBitClear = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I &^ (1 << checkedBitIndex(y.I, "bit-clear"))}
}

var procBitSet = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I | (1 << checkedBitIndex(y.I, "bit-set"))}
}

var procBitFlip = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I ^ (1 << checkedBitIndex(y.I, "bit-flip"))}
}

var procBitTest = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Boolean{B: x.I&(1<<checkedBitIndex(y.I, "bit-test")) != 0}
}

var procBitShiftLeft = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I << checkedShiftCount(y.I, "bit-shift-left")}
}

var procBitShiftRight = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I >> checkedShiftCount(y.I, "bit-shift-right")}
}

var procUnsignedBitShiftRight = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: int(uint(x.I) >> checkedShiftCount(y.I, "unsigned-bit-shift-right"))}
}

var procExInfo = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 3)
	res := &ExInfo{
		rt: cloneGRT(),
	}
	res.Add(KEYWORDS.message, coretypes.EnsureArgIsString(args, 0))
	res.Add(KEYWORDS.data, coretypes.EnsureArgIsMap(args, 1))
	if len(args) == 3 {
		res.Add(KEYWORDS.cause, coretypes.EnsureArgIsError(args, 2))
	}
	return res
}

var procExData = func(args []coretypes.Object) coretypes.Object {
	if ok, res := args[0].(*ExInfo).Get(KEYWORDS.data); ok {
		return res
	}
	return NIL
}

var procExCause = func(args []coretypes.Object) coretypes.Object {
	if ok, res := args[0].(*ExInfo).Get(KEYWORDS.cause); ok {
		return res
	}
	return NIL
}

var procExMessage = func(args []coretypes.Object) coretypes.Object {
	return args[0].(coretypes.Error).Message()
}

var procRegex = func(args []coretypes.Object) coretypes.Object {
	r, err := regexp.Compile(coretypes.EnsureArgIsString(args, 0).S)
	if err != nil {
		panic(RT.NewError("Invalid regex: " + err.Error()))
	}
	return coretypes.MakeRegex(r)
}

func reGroups(s string, indexes []int) coretypes.Object {
	if indexes == nil {
		return NIL
	} else if len(indexes) == 2 {
		if indexes[0] == -1 {
			return NIL
		} else {
			return coretypes.String{S: s[indexes[0]:indexes[1]]}
		}
	} else {
		v := corecollections.EmptyVector()
		for i := 0; i < len(indexes); i += 2 {
			if indexes[i] == -1 {
				v = v.Conjoin(NIL)
			} else {
				v = v.Conjoin(coretypes.String{S: s[indexes[i]:indexes[i+1]]})
			}
		}
		return v
	}
}

var procReSeq = func(args []coretypes.Object) coretypes.Object {
	re := coretypes.EnsureArgIsRegex(args, 0)
	s := coretypes.EnsureArgIsString(args, 1)
	matches := re.R.FindAllStringSubmatchIndex(s.S, -1)
	if matches == nil {
		return NIL
	}
	res := make([]coretypes.Object, len(matches))
	for i, match := range matches {
		res[i] = reGroups(s.S, match)
	}
	return &corecollections.ArraySeq{Arr: res}
}

var procReFind = func(args []coretypes.Object) coretypes.Object {
	re := coretypes.EnsureArgIsRegex(args, 0)
	s := coretypes.EnsureArgIsString(args, 1)
	match := re.R.FindStringSubmatchIndex(s.S)
	return reGroups(s.S, match)
}

var procRand = func(args []coretypes.Object) coretypes.Object {
	r := rand.Float64()
	return coretypes.Double{D: r}
}

var procIsSpecialSymbol = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Boolean{B: coretypes.IsSpecialSymbol(args[0])}
}

var procSubs = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0).S
	start := coretypes.EnsureArgIsInt(args, 1).I
	slen := utf8.RuneCountInString(s)
	end := slen
	if len(args) > 2 {
		end = coretypes.EnsureArgIsInt(args, 2).I
	}
	if start < 0 || start > slen {
		panic(RT.NewError(fmt.Sprintf("String index out of range: %d", start)))
	}
	if end < 0 || end > slen {
		panic(RT.NewError(fmt.Sprintf("String index out of range: %d", end)))
	}
	return coretypes.String{S: string([]rune(s)[start:end])}
}

var procIntern = func(args []coretypes.Object) coretypes.Object {
	ns := EnsureArgIsNamespace(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	vr := ns.Intern(sym)
	if len(args) == 3 {
		vr.Value = args[2]
	}
	return vr
}

var procSetMeta = func(args []coretypes.Object) coretypes.Object {
	vr := EnsureArgIsVar(args, 0)
	meta := coretypes.EnsureArgIsMap(args, 1)
	vr.Meta = meta
	return NIL
}

var procAtom = func(args []coretypes.Object) coretypes.Object {
	res := corert.NewAtom(args[0], nil)
	if len(args) > 1 {
		m := corecollections.NewHashMap(args[1:]...)
		if ok, v := m.Get(KEYWORDS.meta); ok {
			res = corert.NewAtom(args[0], coretypes.EnsureObjectIsMap(v, ""))
		}
	}
	return res
}

var procDeref = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsDeref(args, 0).Deref()
}

var procSwap = func(args []coretypes.Object) coretypes.Object {
	a := EnsureArgIsAtom(args, 0)
	f := coretypes.EnsureArgIsCallable(args, 1)
	oldValue, newValue := a.Swap(f, args[2:], func(v coretypes.Object) { validateAtom(a, v) })
	notifyWatches(a, oldValue, newValue)
	return newValue
}

var procSwapVals = func(args []coretypes.Object) coretypes.Object {
	a := EnsureArgIsAtom(args, 0)
	f := coretypes.EnsureArgIsCallable(args, 1)
	oldValue, newValue := a.Swap(f, args[2:], func(v coretypes.Object) { validateAtom(a, v) })
	notifyWatches(a, oldValue, newValue)
	return corecollections.NewVectorFrom(oldValue, newValue)
}

var procReset = func(args []coretypes.Object) coretypes.Object {
	a := EnsureArgIsAtom(args, 0)
	newValue := args[1]
	oldValue := a.Reset(newValue, func(v coretypes.Object) { validateAtom(a, v) })
	notifyWatches(a, oldValue, newValue)
	return newValue
}

var procResetVals = func(args []coretypes.Object) coretypes.Object {
	a := EnsureArgIsAtom(args, 0)
	newValue := args[1]
	oldValue := a.Reset(newValue, func(v coretypes.Object) { validateAtom(a, v) })
	notifyWatches(a, oldValue, newValue)
	return corecollections.NewVectorFrom(oldValue, newValue)
}

var procAlterMeta = func(args []coretypes.Object) coretypes.Object {
	r := coretypes.EnsureArgIsRef(args, 0)
	f := EnsureArgIsFn(args, 1)
	return r.AlterMeta(f, args[2:])
}

var procResetMeta = func(args []coretypes.Object) coretypes.Object {
	r := coretypes.EnsureArgIsRef(args, 0)
	m := coretypes.EnsureArgIsMap(args, 1)
	return r.ResetMeta(m)
}

var procEmpty = func(args []coretypes.Object) coretypes.Object {
	switch c := args[0].(type) {
	case coretypes.Collection:
		return c.Empty()
	default:
		return NIL
	}
}

var procIsBound = func(args []coretypes.Object) coretypes.Object {
	vr := EnsureArgIsVar(args, 0)
	return coretypes.Boolean{B: vr.Value != nil}
}

// Convert Joker object to native Go object. For those satisfying the
// coretypes.Native type, that's straightforward. For other Joker objects, try
// converting them to suitable native Go objects. E.g. a coretypes.BigInt might
// hold a value > MaxInt64 but < MaxUint64, in which case conversion
// to a uint64 makes more sense than returning the stringized version,
// for use cases such as `(format "%x" value)`. Even for coretypes.BigFloat and
// BigRat, try to (accurately) convert them to native types so they
// can be formatted via the usual ways.
func ToNative(obj coretypes.Object) interface{} {
	switch obj := obj.(type) {
	case coretypes.Native:
		return obj.Native()
	case *coretypes.BigInt:
		b := obj.BigInt()
		if b.IsInt64() {
			return b.Int64()
		}
		if b.IsUint64() {
			return b.Uint64()
		}
	case *coretypes.BigFloat:
		b := obj.BigFloat()
		if f, acc := b.Float64(); acc == big.Exact {
			return f
		}
	case *coretypes.Ratio:
		b := obj.Ratio()
		if f, exact := b.Float64(); exact {
			return f
		}
	}
	return obj.ToString(false)
}

var procFormat = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0)
	objs := args[1:]
	fargs := make([]interface{}, len(objs))
	for i, v := range objs {
		fargs[i] = ToNative(v)
	}
	res := fmt.Sprintf(s.S, fargs...)
	return coretypes.String{S: res}
}

var procList = func(args []coretypes.Object) coretypes.Object {
	return corecollections.NewListFrom(args...)
}

var procCons = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 2)
	s := coretypes.EnsureArgIsSeqable(args, 1).Seq()
	return s.Cons(args[0])
}

var procFirst = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	s := coretypes.EnsureArgIsSeqable(args, 0).Seq()
	return s.First()
}

var procNext = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	s := coretypes.EnsureArgIsSeqable(args, 0).Seq()
	res := s.Rest()
	if res.IsEmpty() {
		return NIL
	}
	return res
}

var procRest = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	s := coretypes.EnsureArgIsSeqable(args, 0).Seq()
	return s.Rest()
}

var procConj = func(args []coretypes.Object) coretypes.Object {
	switch c := args[0].(type) {
	case coretypes.Conjable:
		return c.Conj(args[1])
	case coretypes.Seq:
		return c.Cons(args[1])
	default:
		panic(RT.NewError("conj's first argument must be a collection, got " + c.GetType().ToString(false)))
	}
}

var procSeq = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	s := coretypes.EnsureArgIsSeqable(args, 0).Seq()
	if s.IsEmpty() {
		return NIL
	}
	return s
}

var procIsInstance = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 2)
	t := coretypes.EnsureArgIsType(args, 0)
	return coretypes.Boolean{B: coretypes.IsInstance(t, args[1])}
}

var procAssoc = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsAssociative(args, 0).Assoc(args[1], args[2])
}

var procEquals = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Boolean{B: args[0].Equals(args[1])}
}

var procCount = func(args []coretypes.Object) coretypes.Object {
	switch obj := args[0].(type) {
	case coretypes.Counted:
		return coretypes.Int{I: obj.Count()}
	default:
		s := coretypes.EnsureObjectIsSeqable(obj, "count not supported on this type: %s")
		return coretypes.Int{I: corecollections.SeqCount(s.Seq())}
	}
}

var procSubvec = func(args []coretypes.Object) coretypes.Object {
	// TODO: implement proper Subvector structure
	v := coretypes.EnsureArgIsVec(args, 0)
	start := coretypes.EnsureArgIsInt(args, 1).I
	end := coretypes.EnsureArgIsInt(args, 2).I
	if start > end {
		panic(RT.NewError(fmt.Sprintf("subvec's start index (%d) is greater than end index (%d)", start, end)))
	}
	if end > v.Count() {
		panic(RT.NewError(fmt.Sprintf("subvec's end index (%d) is greater than vector's count (%d)", end, v.Count())))
	}
	subv := make([]coretypes.Object, 0, end-start)
	for i := start; i < end; i++ {
		subv = append(subv, v.At(i))
	}
	return corecollections.NewVectorFrom(subv...)
}

var procCast = func(args []coretypes.Object) coretypes.Object {
	t := coretypes.EnsureArgIsType(args, 0)
	if coretypes.IsEqualOrImplements(t, args[1].GetType()) {
		return args[1]
	}
	panic(RT.NewError("Cannot cast " + args[1].GetType().ToString(false) + " to " + t.ToString(false)))
}

var procVec = func(args []coretypes.Object) coretypes.Object {
	return corecollections.NewVectorFromSeq(coretypes.EnsureArgIsSeqable(args, 0).Seq())
}

var procHashMap = func(args []coretypes.Object) coretypes.Object {
	if len(args)%2 != 0 {
		panic(RT.NewError("No value supplied for key " + args[len(args)-1].ToString(false)))
	}
	return corecollections.NewHashMap(args...)
}

var procHashSet = func(args []coretypes.Object) coretypes.Object {
	res := corecollections.EmptySet()
	for i := 0; i < len(args); i++ {
		res.Add(args[i])
	}
	return res
}

func str(args ...coretypes.Object) string {
	var buffer bytes.Buffer
	for _, obj := range args {
		if !obj.Equals(NIL) {
			t := obj.GetType()
			// TODO: this is a hack. Rethink escape parameter in ToString
			escaped := (t == TYPE.String) || (t == TYPE.Char) || (t == TYPE.Regex)
			buffer.WriteString(obj.ToString(!escaped))
		}
	}
	return buffer.String()
}

var procStr = func(args []coretypes.Object) coretypes.Object {
	// Fast path: 2-arg str (common in parsers: (str buf c))
	if len(args) == 2 {
		a, b := args[0], args[1]
		// Fastest: string + char (the parser hot path)
		if as, ok := a.(coretypes.String); ok {
			if bc, ok := b.(coretypes.Char); ok {
				return coretypes.String{S: as.S + corestr.CharToStringFast(bc.Ch)}
			}
			if bs, ok := b.(coretypes.String); ok {
				return coretypes.String{S: as.S + bs.S}
			}
		}
		// General 2-arg
		if a.Equals(NIL) {
			if b.Equals(NIL) {
				return coretypes.String{S: ""}
			}
			return coretypes.String{S: b.ToString(false)}
		}
		if b.Equals(NIL) {
			return coretypes.String{S: a.ToString(false)}
		}
		return coretypes.String{S: a.ToString(false) + b.ToString(false)}
	}
	// 1-arg str
	if len(args) == 1 {
		a := args[0]
		if a.Equals(NIL) {
			return coretypes.String{S: ""}
		}
		if s, ok := a.(coretypes.String); ok {
			return s
		}
		return coretypes.String{S: a.ToString(false)}
	}
	return coretypes.String{S: str(args...)}
}

var procSymbol = func(args []coretypes.Object) coretypes.Object {
	if len(args) == 1 {
		return coretypes.MakeSymbol(STRINGS.Intern, coretypes.EnsureArgIsString(args, 0).S)
	}
	var ns *string = nil
	if !args[0].Equals(NIL) {
		ns = STRINGS.Intern(coretypes.EnsureArgIsString(args, 0).S)
	}
	return coretypes.MakeSymbolFromKeys(ns, STRINGS.Intern(coretypes.EnsureArgIsString(args, 1).S))
}

var procKeyword = func(args []coretypes.Object) coretypes.Object {
	if len(args) == 1 {
		switch obj := args[0].(type) {
		case coretypes.String:
			return coretypes.MakeKeyword(STRINGS.Intern, obj.S)
		case coretypes.Symbol:
			return coretypes.MakeKeywordFromKeys(obj.NamespaceKey(), obj.NameKey())
		default:
			return NIL
		}
	}
	var ns *string = nil
	if !args[0].Equals(NIL) {
		ns = STRINGS.Intern(coretypes.EnsureArgIsString(args, 0).S)
	}
	name := STRINGS.Intern(coretypes.EnsureArgIsString(args, 1).S)
	return coretypes.MakeKeywordFromKeys(ns, name)
}

var procGensym = func(args []coretypes.Object) coretypes.Object {
	return genSym(coretypes.EnsureArgIsString(args, 0).S, "")
}

var procApply = func(args []coretypes.Object) coretypes.Object {
	// TODO:
	// coretypes.Stacktrace is broken. Need to somehow know
	// the name of the function passed ...
	f := coretypes.EnsureArgIsCallable(args, 0)
	return f.Call(corecollections.ToSlice(coretypes.EnsureArgIsSeqable(args, 1).Seq()))
}

var procLazySeq = func(args []coretypes.Object) coretypes.Object {
	return &corecollections.LazySeq{
		Fn: args[0].(*Fn),
	}
}

var procDelay = func(args []coretypes.Object) coretypes.Object {
	return coretypes.NewDelay(args[0].(*Fn))
}

var procForce = func(args []coretypes.Object) coretypes.Object {
	switch d := args[0].(type) {
	case *coretypes.Delay:
		return d.Force()
	default:
		return d
	}
}

var procIdentical = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Boolean{B: args[0] == args[1]}
}

var procCompare = func(args []coretypes.Object) coretypes.Object {
	k1, k2 := args[0], args[1]
	if k1.Equals(k2) {
		return coretypes.Int{I: 0}
	}
	switch k2.(type) {
	case Nil:
		return coretypes.Int{I: 1}
	}
	switch k1 := k1.(type) {
	case Nil:
		return coretypes.Int{I: -1}
	case coretypes.Comparable:
		return coretypes.Int{I: k1.Compare(k2)}
	}
	panic(RT.NewError(fmt.Sprintf("%s (type: %s) is not a Comparable", k1.ToString(true), k1.GetType().ToString(false))))
}

var procInt = func(args []coretypes.Object) coretypes.Object {
	switch obj := args[0].(type) {
	case coretypes.Char:
		return coretypes.Int{I: int(obj.Ch)}
	case coretypes.Number:
		return obj.Int()
	default:
		panic(RT.NewError(fmt.Sprintf("Cannot cast %s (type: %s) to Int", obj.ToString(true), obj.GetType().ToString(false))))
	}
}

var procNumber = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureObjectIsNumber(args[0], "Cannot cast "+args[0].ToString(true)+": %s")
}

var procDouble = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureObjectIsNumber(args[0], "Cannot cast "+args[0].ToString(true)+": %s")
	return n.Double()
}

var procChar = func(args []coretypes.Object) coretypes.Object {
	switch c := args[0].(type) {
	case coretypes.Char:
		return c
	case coretypes.Number:
		i := c.Int().I
		if i < coretypes.MIN_RUNE || i > coretypes.MAX_RUNE {
			panic(RT.NewError(fmt.Sprintf("Value out of range for char: %d", i)))
		}
		return coretypes.Char{Ch: rune(i)}
	default:
		panic(RT.NewError(fmt.Sprintf("Cannot cast %s (type: %s) to Char", c.ToString(true), c.GetType().ToString(false))))
	}
}

var procBoolean = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Boolean{B: corert.ToBool(args[0])}
}

var procNumerator = func(args []coretypes.Object) coretypes.Object {
	bi := coretypes.EnsureArgIsRatio(args, 0).R.Num()
	return &coretypes.BigInt{B: bi}
}

var procDenominator = func(args []coretypes.Object) coretypes.Object {
	bi := coretypes.EnsureArgIsRatio(args, 0).R.Denom()
	return &coretypes.BigInt{B: bi}
}

var procBigInt = func(args []coretypes.Object) coretypes.Object {
	switch n := args[0].(type) {
	case coretypes.Number:
		return &coretypes.BigInt{B: n.BigInt()}
	case coretypes.String:
		bi := &big.Int{}
		if _, ok := bi.SetString(n.S, 10); ok {
			return &coretypes.BigInt{B: bi}
		}
		panic(RT.NewError("Invalid number format " + n.S))
	default:
		panic(RT.NewError(fmt.Sprintf("Cannot cast %s (type: %s) to coretypes.BigInt", n.ToString(true), n.GetType().ToString(false))))
	}
}

var procBigFloat = func(args []coretypes.Object) coretypes.Object {
	switch n := args[0].(type) {
	case coretypes.Number:
		return &coretypes.BigFloat{B: n.BigFloat()}
	case coretypes.String:
		b := &big.Float{}
		if _, ok := b.SetString(n.S); ok {
			return &coretypes.BigFloat{B: b}
		}
		panic(RT.NewError("Invalid number format " + n.S))
	default:
		panic(RT.NewError(fmt.Sprintf("Cannot cast %s (type: %s) to coretypes.BigFloat", n.ToString(true), n.GetType().ToString(false))))
	}
}

var procNth = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 1).Int().I
	switch coll := args[0].(type) {
	case coretypes.Indexed:
		if len(args) == 3 {
			return coll.TryNth(n, args[2])
		}
		return coll.Nth(n)
	case Nil:
		return NIL
	case coretypes.Sequential:
		switch coll := args[0].(type) {
		case coretypes.Seqable:
			if len(args) == 3 {
				return corecollections.SeqTryNth(coll.Seq(), n, args[2])
			}
			return corecollections.SeqNth(coll.Seq(), n)
		}
	}
	panic(RT.NewError("nth not supported on this type: " + args[0].GetType().ToString(false)))
}

var procLt = func(args []coretypes.Object) coretypes.Object {
	switch a := args[0].(type) {
	case coretypes.Int:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.Boolean{B: a.I < b.I}
		case coretypes.Double:
			return coretypes.Boolean{B: float64(a.I) < b.D}
		}
	case coretypes.Double:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.Boolean{B: a.D < float64(b.I)}
		case coretypes.Double:
			return coretypes.Boolean{B: a.D < b.D}
		}
	}
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Boolean{B: coretypes.GetOps(a).Combine(coretypes.GetOps(b)).Lt(a, b)}
}

var procLte = func(args []coretypes.Object) coretypes.Object {
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Boolean{B: coretypes.GetOps(a).Combine(coretypes.GetOps(b)).Lte(a, b)}
}

var procGt = func(args []coretypes.Object) coretypes.Object {
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Boolean{B: coretypes.GetOps(a).Combine(coretypes.GetOps(b)).Gt(a, b)}
}

var procGte = func(args []coretypes.Object) coretypes.Object {
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Boolean{B: coretypes.GetOps(a).Combine(coretypes.GetOps(b)).Gte(a, b)}
}

var procEq = func(args []coretypes.Object) coretypes.Object {
	switch a := args[0].(type) {
	case coretypes.Int:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.Boolean{B: a.I == b.I}
		case coretypes.Double:
			return coretypes.Boolean{B: float64(a.I) == b.D}
		}
	case coretypes.Double:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.Boolean{B: a.D == float64(b.I)}
		case coretypes.Double:
			return coretypes.Boolean{B: a.D == b.D}
		}
	}
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Boolean{B: coretypes.NumbersEq(a, b)}
}

var procMax = func(args []coretypes.Object) coretypes.Object {
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Max(a, b)
}

var procMin = func(args []coretypes.Object) coretypes.Object {
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Min(a, b)
}

var procIncEx = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(x).Combine(coretypes.BIGINT_OPS)
	return ops.Add(x, coretypes.Int{I: 1})
}

var procDecEx = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(x).Combine(coretypes.BIGINT_OPS)
	return ops.Subtract(x, coretypes.Int{I: 1})
}

var procInc = func(args []coretypes.Object) coretypes.Object {
	switch x := args[0].(type) {
	case coretypes.Int:
		return coretypes.INT_OPS.Add(x, coretypes.Int{I: 1})
	case coretypes.Double:
		return coretypes.Double{D: x.D + 1}
	}
	x := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(x).Combine(coretypes.INT_OPS)
	return ops.Add(x, coretypes.Int{I: 1})
}

var procDec = func(args []coretypes.Object) coretypes.Object {
	switch x := args[0].(type) {
	case coretypes.Int:
		return coretypes.INT_OPS.Subtract(x, coretypes.Int{I: 1})
	case coretypes.Double:
		return coretypes.Double{D: x.D - 1}
	}
	x := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(x).Combine(coretypes.INT_OPS)
	return ops.Subtract(x, coretypes.Int{I: 1})
}

var procPeek = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureObjectIsStack(args[0], "")
	return s.Peek()
}

var procPop = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureObjectIsStack(args[0], "")
	return s.Pop().(coretypes.Object)
}

var procContains = func(args []coretypes.Object) coretypes.Object {
	switch c := args[0].(type) {
	case coretypes.Gettable:
		ok, _ := c.Get(args[1])
		if ok {
			return coretypes.Boolean{B: true}
		}
		return coretypes.Boolean{B: false}
	}
	panic(RT.NewError("contains? not supported on type " + args[0].GetType().ToString(false)))
}

var procGet = func(args []coretypes.Object) coretypes.Object {
	switch c := args[0].(type) {
	case coretypes.Gettable:
		ok, v := c.Get(args[1])
		if ok {
			return v
		}
	}
	if len(args) == 3 {
		return args[2]
	}
	return NIL
}

var procDissoc = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsMap(args, 0).Without(args[1])
}

var procDisj = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsSet(args, 0).Disjoin(args[1])
}

var procFind = func(args []coretypes.Object) coretypes.Object {
	res := coretypes.EnsureArgIsAssociative(args, 0).EntryAt(args[1])
	if res == nil {
		return NIL
	}
	return res
}

var procKeys = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsMap(args, 0).Keys()
}

var procVals = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsMap(args, 0).Vals()
}

var procRseq = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsReversible(args, 0).Rseq()
}

var procName = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: coretypes.EnsureArgIsNamed(args, 0).Name()}
}

var procNamespace = func(args []coretypes.Object) coretypes.Object {
	ns := coretypes.EnsureArgIsNamed(args, 0).Namespace()
	if ns == "" {
		return NIL
	}
	return coretypes.String{S: ns}
}

var procFindVar = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	if sym.NamespaceKey() == nil {
		panic(RT.NewError("find-var argument must be namespace-qualified symbol"))
	}
	if v, ok := GLOBAL_ENV.Resolve(sym); ok {
		return v
	}
	return NIL
}

var procSort = func(args []coretypes.Object) coretypes.Object {
	cmp := coretypes.EnsureArgIsComparator(args, 0)
	coll := coretypes.EnsureArgIsSeqable(args, 1)
	s := coretypes.ComparatorSlice[coretypes.Object]{
		Items: corecollections.ToSlice(coll.Seq()),
		Cmp:   cmp,
	}
	sort.Sort(s)
	return &corecollections.ArraySeq{Arr: s.Items}
}

var procEval = func(args []coretypes.Object) coretypes.Object {
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	expr := Parse(args[0], parseContext)
	return Eval(expr, nil)
}

var procType = func(args []coretypes.Object) coretypes.Object {
	return args[0].GetType()
}

var procPprint = func(args []coretypes.Object) coretypes.Object {
	obj := args[0]
	w := coretypes.EnsureObjectIsio_Writer(GLOBAL_ENV.stdout.Value, "")
	corert.PprintObject(obj, 0, w)
	fmt.Fprint(w, "\n")
	return NIL
}

func PrintObject(obj coretypes.Object, w io.Writer) {
	printReadably := corert.ToBool(GLOBAL_ENV.printReadably.Value)
	switch obj := obj.(type) {
	case coretypes.Printer:
		obj.Print(w, printReadably)
	default:
		fmt.Fprint(w, obj.ToString(printReadably))
	}
}

var procPr = func(args []coretypes.Object) coretypes.Object {
	n := len(args)
	if n > 0 {
		f := coretypes.EnsureObjectIsio_Writer(GLOBAL_ENV.stdout.Value, "")
		for _, arg := range args[:n-1] {
			PrintObject(arg, f)
			fmt.Fprint(f, " ")
		}
		PrintObject(args[n-1], f)
	}
	return NIL
}

var procNewline = func(args []coretypes.Object) coretypes.Object {
	f := coretypes.EnsureObjectIsio_Writer(GLOBAL_ENV.stdout.Value, "")
	fmt.Fprintln(f)
	return NIL
}

var procFlush = func(args []coretypes.Object) coretypes.Object {
	switch f := args[0].(type) {
	case *corert.File:
		f.Sync()
	}
	return NIL
}

func readFromReader(reader io.RuneReader) coretypes.Object {
	r := readerConstruction.NewReader(reader, "<>")
	obj, err := readerConstruction.TryRead(r)
	corert.PanicOnErr(err)
	return obj
}

var procRead = func(args []coretypes.Object) coretypes.Object {
	switch f := args[0].(type) {
	case io.RuneReader:
		return readFromReader(f)
	case io.Reader:
		return readFromReader(osutil.AsRuneReader(f))
	default:
		panic(RT.NewArgTypeError(0, args[0], "io.RuneReader or io.Reader"))
	}
}

var procReadString = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	return readFromReader(osutil.StringRuneReader(coretypes.EnsureArgIsString(args, 0).S))
}

var procReadLine = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 0, 0)
	f := coretypes.EnsureObjectIsStringReader(GLOBAL_ENV.stdin.Value, "")
	line, err := osutil.ReadLine(f)
	if err != nil {
		return NIL
	}
	return coretypes.String{S: line}
}

var procReaderReadLine = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	rdr := coretypes.EnsureArgIsStringReader(args, 0)
	line, err := osutil.ReadLine(rdr)
	if err != nil {
		return NIL
	}
	return coretypes.String{S: line}
}

var procNanoTime = func(args []coretypes.Object) coretypes.Object {
	return &coretypes.BigInt{B: big.NewInt(time.Now().UnixNano())}
}

var procMacroexpand1 = func(args []coretypes.Object) coretypes.Object {
	switch s := args[0].(type) {
	case coretypes.Seq:
		parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
		return macroexpand1(s, parseContext)
	default:
		return s
	}
}

func loadReader(reader *Reader) (coretypes.Object, error) {
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	var lastObj coretypes.Object = NIL
	for {
		obj, err := readerConstruction.TryRead(reader)
		if err == io.EOF {
			return lastObj, nil
		}
		if err != nil {
			return nil, err
		}
		expr, err := TryParse(obj, parseContext)
		if err != nil {
			return nil, err
		}
		lastObj, err = TryEval(expr)
		if err != nil {
			return nil, err
		}
	}
}

var procLoadString = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0)
	obj, err := loadReader(readerConstruction.NewReader(osutil.StringRuneReader(s.S), "<string>"))
	if err != nil {
		panic(RT.NewError(err.Error()))
	}
	return obj
}

var procFindNamespace = func(args []coretypes.Object) coretypes.Object {
	ns := GLOBAL_ENV.FindNamespace(coretypes.EnsureArgIsSymbol(args, 0))
	if ns == nil {
		return NIL
	}
	return ns
}

var procCreateNamespace = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	res := GLOBAL_ENV.EnsureSymbolIsNamespace(sym)
	// In linter mode the latest create-ns call overrides position info.
	// This is for the cases when (ns ...) is called in .jokerd/linter.clj file and alike.
	// Also, isUsed needs to be reset in this case.
	if LINTER_MODE {
		res.Name = res.Name.WithInfo(sym.GetInfo()).(coretypes.Symbol)
		res.isUsed = false
	}
	return res
}

var procInjectNamespace = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	ns := GLOBAL_ENV.EnsureSymbolIsNamespace(sym)
	ns.isUsed = true
	ns.isGloballyUsed = true
	return ns
}

var procInjectLinterType = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	LINTER_TYPES[sym.NameKey()] = true
	return NIL
}

var procRemoveNamespace = func(args []coretypes.Object) coretypes.Object {
	ns := GLOBAL_ENV.RemoveNamespace(coretypes.EnsureArgIsSymbol(args, 0))
	if ns == nil {
		return NIL
	}
	return ns
}

var procAllNamespaces = func(args []coretypes.Object) coretypes.Object {
	s := make([]coretypes.Object, 0, len(GLOBAL_ENV.Namespaces))
	for _, ns := range GLOBAL_ENV.Namespaces {
		s = append(s, ns)
	}
	return &corecollections.ArraySeq{Arr: s}
}

var procNamespaceName = func(args []coretypes.Object) coretypes.Object {
	return EnsureArgIsNamespace(args, 0).Name
}

var procNamespaceMap = func(args []coretypes.Object) coretypes.Object {
	r := &corecollections.ArrayMap{}
	for k, v := range EnsureArgIsNamespace(args, 0).mappings {
		r.Add(coretypes.MakeSymbol(STRINGS.Intern, *k), v)
	}
	return r
}

var procNamespaceUnmap = func(args []coretypes.Object) coretypes.Object {
	ns := EnsureArgIsNamespace(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	if sym.NamespaceKey() != nil {
		panic(RT.NewError("Can't unintern namespace-qualified symbol"))
	}
	delete(ns.mappings, sym.NameKey())
	return NIL
}

var procVarNamespace = func(args []coretypes.Object) coretypes.Object {
	v := EnsureArgIsVar(args, 0)
	return v.ns
}

var procRefer = func(args []coretypes.Object) coretypes.Object {
	ns := EnsureArgIsNamespace(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	v := EnsureArgIsVar(args, 2)
	return ns.Refer(sym, v)
}

var procAlias = func(args []coretypes.Object) coretypes.Object {
	EnsureArgIsNamespace(args, 0).AddAlias(coretypes.EnsureArgIsSymbol(args, 1), EnsureArgIsNamespace(args, 2))
	return NIL
}

var procNamespaceAliases = func(args []coretypes.Object) coretypes.Object {
	r := &corecollections.ArrayMap{}
	for k, v := range EnsureArgIsNamespace(args, 0).aliases {
		r.Add(coretypes.MakeSymbol(STRINGS.Intern, *k), v)
	}
	return r
}

var procNamespaceUnalias = func(args []coretypes.Object) coretypes.Object {
	ns := EnsureArgIsNamespace(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	if sym.NamespaceKey() != nil {
		panic(RT.NewError("Alias can't be namespace-qualified"))
	}
	delete(ns.aliases, sym.NameKey())
	return NIL
}

var procVarGet = func(args []coretypes.Object) coretypes.Object {
	return EnsureArgIsVar(args, 0).Resolve()
}

var procVarSet = func(args []coretypes.Object) coretypes.Object {
	EnsureArgIsVar(args, 0).Value = args[1]
	return args[1]
}

var procNsResolve = func(args []coretypes.Object) coretypes.Object {
	ns := EnsureArgIsNamespace(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	if sym.NamespaceKey() == nil && TYPES.Contains(sym.NameKey()) {
		return TYPES.Lookup(sym.NameKey())
	}
	if vr, ok := GLOBAL_ENV.ResolveIn(ns, sym); ok {
		return vr
	}
	return NIL
}

var procArrayMap = func(args []coretypes.Object) coretypes.Object {
	if len(args)%2 == 1 {
		panic(RT.NewError("No value supplied for key " + args[len(args)-1].ToString(false)))
	}
	res := corecollections.EmptyArrayMap()
	for i := 0; i < len(args); i += 2 {
		res.Set(args[i], args[i+1])
	}
	return res
}

const bufferHashMask uint32 = 0x5ed19e84

var procBuffer = func(args []coretypes.Object) coretypes.Object {
	if len(args) > 0 {
		s := coretypes.EnsureArgIsString(args, 0)
		return corert.MakeBuffer(bytes.NewBufferString(s.S))
	}
	return corert.MakeBuffer(&bytes.Buffer{})
}

var procBufferedReader = func(args []coretypes.Object) coretypes.Object {
	switch rdr := args[0].(type) {
	case io.Reader:
		return corert.MakeBufferedReader(rdr)
	default:
		panic(RT.NewArgTypeError(0, args[0], "IOReader"))
	}
}

var procSlurp = func(args []coretypes.Object) coretypes.Object {
	switch f := args[0].(type) {
	case coretypes.String:
		s, err := osutil.ReadFileString(f.S)
		corert.PanicOnErr(err)
		return coretypes.String{S: s}
	case io.Reader:
		s, err := osutil.ReadAllString(f)
		corert.PanicOnErr(err)
		return coretypes.String{S: s}
	default:
		panic(RT.NewArgTypeError(0, args[0], "String or IOReader"))
	}
}

var procSpit = func(args []coretypes.Object) coretypes.Object {
	f := args[0]
	content := args[1]
	opts := coretypes.EnsureArgIsMap(args, 2)
	appendFile := false
	if ok, append := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "append")); ok {
		appendFile = corert.ToBool(append)
	}
	switch f := f.(type) {
	case coretypes.String:
		err := osutil.WriteFileString(f.S, str(content), appendFile)
		corert.PanicOnErr(err)
	case io.Writer:
		err := osutil.WriteString(f, str(content))
		corert.PanicOnErr(err)
	default:
		panic(RT.NewArgTypeError(0, args[0], "String or IOWriter"))
	}
	return NIL
}

var procShuffle = func(args []coretypes.Object) coretypes.Object {
	s := corecollections.ToSlice(coretypes.EnsureArgIsSeqable(args, 0).Seq())
	for i := range s {
		j := rand.Intn(i + 1)
		s[i], s[j] = s[j], s[i]
	}
	return corecollections.NewVectorFrom(s...)
}

var procIsRealized = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Boolean{B: coretypes.EnsureArgIsPending(args, 0).IsRealized()}
}

var procDeriveInfo = func(args []coretypes.Object) coretypes.Object {
	dest := args[0]
	src := args[1]
	return coretypes.WithInfo(dest, src.GetInfo())
}

var procJokerVersion = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: corert.VERSION[1:]}
}

var procHash = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Int{I: int(args[0].Hash())}
}

func loadFile(filename string) coretypes.Object {
	var reader *Reader
	f, rr, err := osutil.OpenRuneFile(filename)
	corert.PanicOnErr(err)
	defer func() { corert.PanicOnErr(f.Close()) }()
	reader = readerConstruction.NewReader(rr, filename)
	ProcessReaderFromEval(reader, filename)
	return NIL
}

var procLoadFile = func(args []coretypes.Object) coretypes.Object {
	filename := coretypes.EnsureArgIsString(args, 0)
	return loadFile(filename.S)
}

var procLoadLibFromPath = func(args []coretypes.Object) coretypes.Object {
	libname := coretypes.EnsureArgIsSymbol(args, 0).Name()
	pathname := coretypes.EnsureArgIsString(args, 1).S
	cp := GLOBAL_ENV.classPath.Value
	cpvec := coretypes.EnsureObjectIsVec(cp, "*classpath*: %s")
	count := cpvec.Count()
	var f *os.File
	var err error
	var canonicalErr error
	var filename string
	for i := 0; i < count; i++ {
		elem := cpvec.At(i)
		cpelem := coretypes.EnsureObjectIsString(elem, "*classpath*["+strconv.Itoa(i)+"]: %s")
		s := cpelem.S
		if s == "" {
			filename = pathname
		} else {
			filename = deps.ResolveLibPath(s, libname)
		}
		f, _, err = osutil.OpenRuneFile(filename)
		if err == nil {
			canonicalErr = nil
			break
		}
		if s == "" {
			canonicalErr = err
		}
	}
	corert.PanicOnErr(canonicalErr)
	corert.PanicOnErr(err)
	defer func() { corert.PanicOnErr(f.Close()) }()
	reader := readerConstruction.NewReader(osutil.AsRuneReader(f), filename)
	ProcessReaderFromEval(reader, filename)
	return NIL
}

var procReduceKv = func(args []coretypes.Object) coretypes.Object {
	f := coretypes.EnsureArgIsCallable(args, 0)
	init := args[1]
	coll := coretypes.EnsureArgIsKVReduce(args, 2)
	return coll.KVReduce(f, init)
}

var procReduce = func(args []coretypes.Object) coretypes.Object {
	f := coretypes.EnsureArgIsCallable(args, 0)
	if len(args) == 2 {
		coll := coretypes.EnsureArgIsReduce(args, 1)
		return coll.Reduce(f)
	}
	init := args[1]
	coll := coretypes.EnsureArgIsReduce(args, 2)
	return coll.ReduceInit(f, init)
}

var procIndexOf = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0)
	ch := coretypes.EnsureArgIsChar(args, 1)
	for i, r := range s.S {
		if r == ch.Ch {
			return coretypes.Int{I: i}
		}
	}
	return coretypes.Int{I: -1}
}

func libExternalPath(sym coretypes.Symbol) (path string, ok bool) {
	nsSourcesVar, _ := GLOBAL_ENV.Resolve(coretypes.MakeSymbol(STRINGS.Intern, "joker.core/*ns-sources*"))
	nsSources := corecollections.ToSlice(nsSourcesVar.Value.(coretypes.Vec).Seq())

	var sourceKey string
	var sourceMap coretypes.Map
	for _, source := range nsSources {
		sourceKey = source.(coretypes.Vec).Nth(0).ToString(false)
		match, _ := regexp.MatchString(sourceKey, sym.Name())
		if match {
			sourceMap = source.(coretypes.Vec).Nth(1).(coretypes.Map)
			break
		}
	}
	if sourceMap != nil {
		ok, url := sourceMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "url"))
		if !ok {
			panic(RT.NewError("Key :url not found in ns-sources for: " + sourceKey))
		} else {
			path, err := deps.ExternalSourceToPath(osutil.HomeDir(), sym.Name(), url.ToString(false))
			corert.PanicOnErr(err)
			return path, true
		}
	}
	return
}

var procLibPath = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	var path string

	path, ok := libExternalPath(sym)

	if !ok {
		var file string
		if GLOBAL_ENV.file.Value == nil {
			var err error
			file, err = osutil.Abs("user")
			corert.PanicOnErr(err)
		} else {
			file = coretypes.EnsureObjectIsString(GLOBAL_ENV.file.Value, "").S
			file = osutil.ResolveSymlink(file)
		}
		ns := GLOBAL_ENV.CurrentNamespace().Name
		path = deps.ResolveRelativeLibPath(file, ns.Name(), sym.Name())
	}
	return coretypes.String{S: path}
}

var procInternFakeVar = func(args []coretypes.Object) coretypes.Object {
	nsSym := coretypes.EnsureArgIsSymbol(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	isMacro := corert.ToBool(args[2])
	res := InternFakeSymbol(GLOBAL_ENV.FindNamespace(nsSym), sym)
	res.isMacro = isMacro
	return res
}

var procParse = func(args []coretypes.Object) coretypes.Object {
	lm, _ := GLOBAL_ENV.Resolve(coretypes.MakeSymbol(STRINGS.Intern, "joker.core/*linter-mode*"))
	lm.Value = coretypes.Boolean{B: true}
	LINTER_MODE = true
	defer func() {
		LINTER_MODE = false
		lm.Value = coretypes.Boolean{B: false}
	}()
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	res := Parse(args[0], parseContext)
	return res.Dump(false)
}

var procTypes = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 0, 0)
	res := corecollections.EmptyArrayMap()
	for k, v := range TYPES {
		res.Add(coretypes.String{S: *k}, v)
	}
	return res
}

var procCreateChan = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	n := coretypes.EnsureArgIsInt(args, 0)
	ch := make(chan corert.FutureResult, n.I)
	return corert.NewObjectChannel(ch)
}

var procCloseChan = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	EnsureArgIsChannel(args, 0).Close()
	return NIL
}

var procSend = func(args []coretypes.Object) (obj coretypes.Object) {
	CheckArity(args, 2, 2)
	ch := EnsureArgIsChannel(args, 0)
	v := args[1]
	if v.Equals(NIL) {
		panic(RT.NewError("Can't put nil on channel"))
	}
	if ch.IsClosed() {
		return coretypes.MakeBoolean(false)
	}
	return coretypes.MakeBoolean(ch.Send(v))
}

var procReceive = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	ch := EnsureArgIsChannel(args, 0)
	value, status, err := ch.Receive(nil)
	if status == corert.ChannelReceiveClosed {
		return NIL
	}
	if err != nil {
		panic(coretypes.Object(err))
	}
	return value
}

var procGo = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	f := coretypes.EnsureArgIsCallable(args, 0)
	ch := corert.NewObjectChannel(make(chan corert.FutureResult, 1))
	go func() {
		registerGoroutineRT()
		defer unregisterGoroutineRT()

		defer func() {
			if r := recover(); r != nil {
				switch r := r.(type) {
				case coretypes.Error:
					ch.SendResult(corert.NewFutureResult(NIL, r))
					ch.Close()
				default:
					panic(r)
				}
			}
		}()

		res := call0(f)
		ch.SendResult(corert.NewFutureResult(res, nil))
		ch.Close()
	}()
	return ch
}

var procVerbosityLevel = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 0, 0)
	return coretypes.MakeInt(VerbosityLevel)
}

var procExit = func(args []coretypes.Object) coretypes.Object {
	corert.ExitJoker(coretypes.EnsureArgIsInt(args, 0).I)
	return NIL
}

var procIsNaN = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0)
	return coretypes.Boolean{B: math.IsNaN(n.Double().D)}
}

var procAbs = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0)
	switch n := n.(type) {
	case coretypes.Double:
		return coretypes.Double{D: math.Abs(n.D)}
	case *coretypes.BigInt:
		b := &big.Int{}
		return &coretypes.BigInt{B: b.Abs(n.B)}
	case *coretypes.BigFloat:
		b := &big.Float{}
		return &coretypes.BigFloat{B: b.Abs(n.B)}
	case *coretypes.Ratio:
		r := &big.Rat{}
		return &coretypes.Ratio{R: r.Abs(n.R)}
	case coretypes.Int:
		x := n.I
		if x < 0 {
			x = -x
		}
		return coretypes.Int{I: x}
	}
	panic(FailArg(n, "coretypes.Number", 0))
}

var procIsInfinite = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0)
	return coretypes.Boolean{B: math.IsInf(n.Double().D, 0)}
}

var procParseDouble = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0)
	d, err := numerical.ParseFloat64(s.S)
	if err != nil {
		return NIL
	}
	return coretypes.Double{D: d}
}

var procParseLong = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0)
	i, err := numerical.ParseInt(s.S, 10, 64)
	if err != nil {
		return NIL
	}
	return coretypes.Int{I: int(i)}
}

func PackReader(reader *Reader, filename string) ([]byte, error) {
	var p []byte
	packEnv := NewPackEnv()
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	if filename != "" {
		currentFilename := parseContext.GlobalEnv.file.Value
		defer func() {
			parseContext.GlobalEnv.SetFilename(currentFilename)
		}()
		s, err := osutil.Abs(filename)
		corert.PanicOnErr(err)
		parseContext.GlobalEnv.SetFilename(coretypes.MakeString(s))
	}
	for {
		obj, err := readerConstruction.TryRead(reader)
		if err == io.EOF {
			var hp []byte
			hp = packEnv.Pack(hp)
			return append(hp, p...), nil
		}
		if err != nil {
			fmt.Fprintln(Stderr, err)
			return nil, err
		}
		expr, err := TryParse(obj, parseContext)
		if err != nil {
			fmt.Fprintln(Stderr, err)
			return nil, err
		}
		p = expr.Pack(p, packEnv)
		_, err = TryEval(expr)
		if err != nil {
			fmt.Fprintln(Stderr, err)
			return nil, err
		}
	}
}

var procIncProblemCount = func(args []coretypes.Object) coretypes.Object {
	PROBLEM_COUNT++
	return NIL
}

func ProcessReader(reader *Reader, filename string, phase corereader.Phase) error {
	if phase == corereader.FormatPhase {
		FORMAT_MODE = true
		coretypes.FormatMode = true
		corecollections.HASHMAP_THRESHOLD = 100000
	}
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	if filename != "" {
		currentFilename := parseContext.GlobalEnv.file.Value
		defer func() {
			parseContext.GlobalEnv.SetFilename(currentFilename)
		}()
		s, err := osutil.Abs(filename)
		corert.PanicOnErr(err)
		parseContext.GlobalEnv.SetFilename(coretypes.MakeString(s))
	}
	var prevObj coretypes.Object
	for {
		obj, err := readerConstruction.TryRead(reader)
		if err == io.EOF {
			if FORMAT_MODE && prevObj != nil {
				fmt.Fprint(Stdout, "\n")
			}
			return nil
		}
		if err != nil {
			fmt.Fprintln(Stderr, err)
			return err
		}
		if phase == corereader.ReadPhase {
			continue
		}
		if phase == corereader.FormatPhase {
			if prevObj != nil {
				cnt := corert.NewLineCount(prevObj, obj)
				for i := 0; i < cnt; i++ {
					fmt.Fprint(Stdout, "\n")
				}
				if cnt == 0 {
					fmt.Fprint(Stdout, " ")
				}
			}
			corert.FormatObject(obj, 0, Stdout)
			prevObj = obj
			continue
		}
		expr, err := TryParse(obj, parseContext)
		if err != nil {
			fmt.Fprintln(Stderr, err)
		}
		if phase == corereader.ParsePhase {
			continue
		}
		if err != nil {
			return err
		}
		obj, err = TryEval(expr)
		if err != nil {
			fmt.Fprintln(Stderr, err)
			return err
		}
		if phase == corereader.EvalPhase {
			continue
		}
		if _, ok := obj.(Nil); !ok {
			fmt.Fprintln(Stdout, obj.ToString(true))
		}
	}
}

func ProcessReaderFromEval(reader *Reader, filename string) {
	maybeOverrideRange()
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	if filename != "" {
		currentFilename := parseContext.GlobalEnv.file.Value
		defer func() {
			parseContext.GlobalEnv.SetFilename(currentFilename)
		}()
		s, err := osutil.Abs(filename)
		corert.PanicOnErr(err)
		parseContext.GlobalEnv.SetFilename(coretypes.MakeString(s))
	}
	for {
		obj, err := readerConstruction.TryRead(reader)
		if err == io.EOF {
			return
		}
		corert.PanicOnErr(err)
		expr, err := TryParse(obj, parseContext)
		corert.PanicOnErr(err)
		_, err = TryEval(expr)
		corert.PanicOnErr(err)
	}
}

var haveSetCoreNamespaces bool

func ProcessCoreData() {
	// Let MaybeLazy() handle initialization.
	if !haveSetCoreNamespaces {
		setCoreNamespaces()
		haveSetCoreNamespaces = true
	}
}

func ProcessReplData() {
	// Let MaybeLazy() handle initialization.
}

func ProcessLinterData(dialect corereader.Dialect) {
	if dialect == corereader.EDNDialect {
		markJokerNamespacesAsUsed()
		return
	}
	processGeneratedLinterPayload("linter_all.joke")
	if dialect == corereader.JokerDialect {
		markJokerNamespacesAsUsed()
		processGeneratedLinterPayload("linter_joker.joke")
		return
	}
	processGeneratedLinterPayload("linter_cljx.joke")
	switch dialect {
	case corereader.CLJDialect:
		processGeneratedLinterPayload("linter_clj.joke")
	case corereader.CLJSDialect:
		processGeneratedLinterPayload("linter_cljs.joke")
	}
}

func processGeneratedLinterPayload(path string) {
	data, ok := coregenerated.LinterDataByPath(path)
	if !ok {
		panic(RT.NewError("missing generated linter payload: " + path))
	}
	processData(data)
}

func processData(data []byte) {
	ns := GLOBAL_ENV.CurrentNamespace()
	GLOBAL_ENV.SetCurrentNamespace(GLOBAL_ENV.CoreNamespace)
	defer func() { GLOBAL_ENV.SetCurrentNamespace(ns) }()
	header, p := UnpackHeader(data, GLOBAL_ENV)
	for len(p) > 0 {
		var expr Expr
		expr, p = UnpackExpr(p, header)
		_, err := TryEval(expr)
		corert.PanicOnErr(err)
	}
	if VerbosityLevel > 0 {
		fmt.Fprintf(Stderr, "processData: Evaluated code for %s\n", GLOBAL_ENV.CurrentNamespace().ToString(false))
	}
}

func setCoreNamespaces() {
	ns := GLOBAL_ENV.CoreNamespace
	ns.MaybeLazy("joker.core")

	vr := ns.Resolve("*core-namespaces*")
	set := vr.Value.(*corecollections.MapSet)
	for _, ns := range coregenerated.CoreNamespaces() {
		set = set.Conj(coretypes.MakeSymbol(STRINGS.Intern, ns)).(*corecollections.MapSet)
	}
	set = set.Conj(coretypes.MakeSymbol(STRINGS.Intern, "user")).(*corecollections.MapSet)
	vr.Value = set

	// Add 'joker.core to *loaded-libs*, now that it's loaded.
	vr = ns.Resolve("*loaded-libs*")
	set = vr.Value.(*corecollections.MapSet).Conj(ns.Name).(*corecollections.MapSet)
	vr.Value = set

	// Install runtime overrides that depend on core.joke vars existing.
	maybeOverrideRange()
	maybeOverrideSeqOps()
}

var procIsNamespaceInitialized = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	if sym.NamespaceKey() != nil {
		panic(RT.NewError("Can't ask for namespace info on namespace-qualified symbol"))
	}
	// First look for registered (e.g. std) libs
	ns, found := GLOBAL_ENV.Namespaces[sym.NameKey()]
	return coretypes.MakeBoolean(found && ns.Lazy == nil)
}

func findConfigFile(filename string, workingDir string, findDir bool) string {
	configName := ".joker"
	if findDir {
		configName = ".jokerd"
	}
	path, err := osutil.FindConfigPath(filename, workingDir, configName, osutil.HomeDir(), findDir)
	if err != nil {
		fmt.Fprintln(Stderr, "coretypes.Error reading config file "+filename+": ", err)
		return ""
	}
	return path
}

func printConfigError(filename, msg string) {
	fmt.Fprintln(Stderr, "coretypes.Error reading config file "+filename+": ", msg)
}

func knownMacrosToMap(km coretypes.Object) (coretypes.Map, error) {
	s := km.(coretypes.Seqable).Seq()
	res := corecollections.EmptyArrayMap()
	for !s.IsEmpty() {
		obj := s.First()
		switch obj := obj.(type) {
		case coretypes.Symbol:
			res.Add(obj, NIL)
		case coretypes.Vec:
			if obj.Count() != 2 {
				return nil, errors.New(":known-macros item must be a symbol or a vector with two elements")
			}
			res.Add(obj.At(0), obj.At(1))
		default:
			return nil, errors.New(":known-macros item must be a symbol or a vector, got " + obj.GetType().ToString(false))
		}
		s = s.Rest()
	}
	return res, nil
}

func ReadConfig(filename string, workingDir string) {
	LINTER_CONFIG = GLOBAL_ENV.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*linter-config*"))
	LINTER_CONFIG.Value = corecollections.EmptyArrayMap()
	configFileName := findConfigFile(filename, workingDir, false)
	if configFileName == "" {
		return
	}
	f, rr, err := osutil.OpenRuneFile(configFileName)
	if err != nil {
		printConfigError(configFileName, err.Error())
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			printConfigError(configFileName, closeErr.Error())
		}
	}()
	r := readerConstruction.NewReader(rr, configFileName)
	config, err := readerConstruction.TryRead(r)
	if err != nil {
		printConfigError(configFileName, err.Error())
		return
	}
	configMap, ok := config.(coretypes.Map)
	if !ok {
		printConfigError(configFileName, "config root object must be a map, got "+config.GetType().ToString(false))
		return
	}
	ok, ignoredUnusedNamespaces := configMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "ignored-unused-namespaces"))
	if ok {
		seq, ok1 := ignoredUnusedNamespaces.(coretypes.Seqable)
		if ok1 {
			WARNINGS.ignoredUnusedNamespaces = corecollections.NewSetFromSeq(seq.Seq())
		} else {
			printConfigError(configFileName, ":ignored-unused-namespaces value must be a vector, got "+ignoredUnusedNamespaces.GetType().ToString(false))
			return
		}
	}
	ok, ignoredFileRegexes := configMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "ignored-file-regexes"))
	if ok {
		seq, ok1 := ignoredFileRegexes.(coretypes.Seqable)
		if ok1 {
			s := seq.Seq()
			for !s.IsEmpty() {
				regex, ok2 := s.First().(*coretypes.Regex)
				if !ok2 {
					printConfigError(configFileName, ":ignored-file-regexes elements must be regexes, got "+s.First().GetType().ToString(false))
					return
				}
				WARNINGS.IgnoredFileRegexes = append(WARNINGS.IgnoredFileRegexes, regex.R)
				s = s.Rest()
			}
		} else {
			printConfigError(configFileName, ":ignored-file-regexes value must be a vector, got "+ignoredFileRegexes.GetType().ToString(false))
			return
		}
	}
	ok, entryPoints := configMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "entry-points"))
	if ok {
		seq, ok1 := entryPoints.(coretypes.Seqable)
		if ok1 {
			WARNINGS.entryPoints = corecollections.NewSetFromSeq(seq.Seq())
		} else {
			printConfigError(configFileName, ":entry-points value must be a vector, got "+entryPoints.GetType().ToString(false))
			return
		}
	}
	ok, knownNamespaces := configMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "known-namespaces"))
	if ok {
		if _, ok1 := knownNamespaces.(coretypes.Seqable); !ok1 {
			printConfigError(configFileName, ":known-namespaces value must be a vector, got "+knownNamespaces.GetType().ToString(false))
			return
		}
	}
	ok, knownTags := configMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "known-tags"))
	if ok {
		if _, ok1 := knownTags.(coretypes.Seqable); !ok1 {
			printConfigError(configFileName, ":known-tags value must be a vector, got "+knownTags.GetType().ToString(false))
			return
		}
	}
	ok, knownMacros := configMap.Get(KEYWORDS.knownMacros)
	if ok {
		_, ok1 := knownMacros.(coretypes.Seqable)
		if !ok1 {
			printConfigError(configFileName, ":known-macros value must be a vector, got "+knownMacros.GetType().ToString(false))
			return
		}
		m, err := knownMacrosToMap(knownMacros)
		if err != nil {
			printConfigError(configFileName, err.Error())
			return
		}
		configMap = configMap.Assoc(KEYWORDS.knownMacros, m).(coretypes.Map)
	}
	ok, rules := configMap.Get(KEYWORDS.rules)
	if ok {
		m, ok := rules.(coretypes.Map)
		if !ok {
			printConfigError(configFileName, ":rules value must be a map, got "+rules.GetType().ToString(false))
			return
		}
		if ok, v := m.Get(KEYWORDS.ifWithoutElse); ok {
			WARNINGS.ifWithoutElse = corert.ToBool(v)
		}
		if ok, v := m.Get(KEYWORDS.unusedFnParameters); ok {
			WARNINGS.unusedFnParameters = corert.ToBool(v)
		}
		if ok, v := m.Get(KEYWORDS.fnWithEmptyBody); ok {
			WARNINGS.fnWithEmptyBody = corert.ToBool(v)
		}
	}
	if ok, valid := configMap.Get(KEYWORDS.validIdent); ok {
		m, ok := valid.(coretypes.Map)
		if !ok {
			printConfigError(configFileName, ":valid-ident value must be a map, got "+valid.GetType().ToString(false))
			return
		}
		if ok, v := m.Get(KEYWORDS.characterSet); ok {
			switch {
			case v.Equals(KEYWORDS.core):
				SetIdentSetCore()
			case v.Equals(KEYWORDS.symbol):
				SetIdentSetSymbol()
			case v.Equals(KEYWORDS.visible):
				SetIdentSetVisible()
			case v.Equals(KEYWORDS.any):
				SetIdentSetAny()
			default:
				printConfigError(configFileName, ":character-set value (in :valid-ident) value must be :core, :symbol, :visible, or :any; got "+v.GetType().ToString(false)+" "+v.ToString(false))
				return
			}
		}
		if ok, v := m.Get(KEYWORDS.encodingRange); ok {
			switch {
			case v.Equals(KEYWORDS.unicode):
				SetIdentRangeUnicode()
			case v.Equals(KEYWORDS.ascii):
				SetIdentRangeASCII()
			case v.Equals(KEYWORDS.any):
				SetIdentRangeAny()
			default:
				printConfigError(configFileName, ":encoding-range value (in :valid-ident) value must be :unicode, :ascii, or :any; got "+v.GetType().ToString(false)+" "+v.ToString(false))
				return
			}
		}
	}
	LINTER_CONFIG.Value = configMap
}

func RemoveJokerNamespaces() {
	for k, ns := range GLOBAL_ENV.Namespaces {
		if ns != GLOBAL_ENV.CoreNamespace && corestr.HasJokerNamespacePrefix(*k) {
			delete(GLOBAL_ENV.Namespaces, k)
		}
	}
}

func markJokerNamespacesAsUsed() {
	for k, ns := range GLOBAL_ENV.Namespaces {
		if ns != GLOBAL_ENV.CoreNamespace && corestr.HasJokerNamespacePrefix(*k) {
			ns.isUsed = true
			ns.isGloballyUsed = true
		}
	}
}

func NewReaderFromFile(filename string) (*Reader, error) {
	data, err := osutil.ReadFileBytes(filename)
	if err != nil {
		fmt.Fprintln(Stderr, "coretypes.Error: ", err)
		return nil, err
	}
	return readerConstruction.NewReader(osutil.ByteRuneReader(data), filename), nil
}

func ProcessLinterFile(configDir string, filename string) {
	if linterFileName := osutil.ExistingChild(configDir, filename); linterFileName != "" {
		if reader, err := NewReaderFromFile(linterFileName); err == nil {
			ProcessReader(reader, linterFileName, corereader.EvalPhase)
		}
	}
}

func ProcessLinterFiles(dialect corereader.Dialect, filename string, workingDir string) {
	if dialect == corereader.EDNDialect {
		return
	}
	configDir := findConfigFile(filename, workingDir, true)
	if configDir == "" {
		return
	}
	if dialect == corereader.JokerDialect {
		ProcessLinterFile(configDir, "linter.joke")
		return
	}
	ProcessLinterFile(configDir, "linter.cljc")
	switch dialect {
	case corereader.CLJSDialect:
		ProcessLinterFile(configDir, "linter.cljs")
	case corereader.CLJDialect:
		ProcessLinterFile(configDir, "linter.clj")
	}
}

// ---- atom_ext.go ----
// atom_ext.go — Atom extensions: validators, watches, compare-and-set!

// atomExtras holds validator and watches for an Atom.
// Stored in a side table to avoid changing the Atom struct.
type atomWatch struct {
	key coretypes.Object
	fn  coretypes.Callable
}

type atomExtras struct {
	mu        sync.RWMutex
	validator coretypes.Callable
	watches   map[string]atomWatch // key.ToString → watch
}

var atomExtrasMap sync.Map // *corert.Atom → *atomExtras

func getAtomExtras(a *corert.Atom) *atomExtras {
	if v, ok := atomExtrasMap.Load(a); ok {
		return v.(*atomExtras)
	}
	return nil
}

func getOrCreateAtomExtras(a *corert.Atom) *atomExtras {
	ext := &atomExtras{watches: make(map[string]atomWatch)}
	actual, _ := atomExtrasMap.LoadOrStore(a, ext)
	return actual.(*atomExtras)
}

// notifyWatches calls all watch functions with (key atom old-val new-val).
func notifyWatches(a *corert.Atom, oldVal, newVal coretypes.Object) {
	ext := getAtomExtras(a)
	if ext == nil {
		return
	}
	ext.mu.RLock()
	watches := make([]atomWatch, 0, len(ext.watches))
	for _, watch := range ext.watches {
		watches = append(watches, watch)
	}
	ext.mu.RUnlock()
	for _, w := range watches {
		call4(w.fn, w.key, a, oldVal, newVal)
	}
}

// validateAtom checks the validator, panics if invalid.
func validateAtom(a *corert.Atom, newVal coretypes.Object) {
	ext := getAtomExtras(a)
	if ext == nil {
		return
	}
	ext.mu.RLock()
	validator := ext.validator
	ext.mu.RUnlock()
	if validator == nil {
		return
	}
	result := call1(validator, newVal)
	if !corert.ToBool(result) {
		panic(coretypes.RuntimeError("Invalid reference state"))
	}
}

func init() {
	registerAtomExtProcs()
}

func registerAtomExtProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// set-validator! — (set-validator! atom fn)
	svVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "set-validator!"))
	svVr.Value = Proc{Name: "procSetValidator", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		a := EnsureObjectIsAtom(args[0], "set-validator! requires an atom, got %s")
		ext := getOrCreateAtomExtras(a)
		var validator coretypes.Callable
		if args[1] != nil && !corert.IsNil(args[1]) {
			validator = coretypes.EnsureObjectIsCallable(args[1], "validator must be a function, got %s")
			// Validate current value before installing the validator.
			result := call1(validator, a.Deref())
			if !corert.ToBool(result) {
				panic(coretypes.RuntimeError("Invalid reference state"))
			}
		}
		ext.mu.Lock()
		ext.validator = validator
		ext.mu.Unlock()
		return NIL
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "set-validator!"), svVr)

	// get-validator — (get-validator atom)
	gvVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "get-validator"))
	gvVr.Value = Proc{Name: "procGetValidator", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		a := EnsureObjectIsAtom(args[0], "get-validator requires an atom, got %s")
		ext := getAtomExtras(a)
		if ext == nil {
			return NIL
		}
		ext.mu.RLock()
		validator := ext.validator
		ext.mu.RUnlock()
		if validator == nil {
			return NIL
		}
		return validator.(coretypes.Object)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "get-validator"), gvVr)

	// add-watch — (add-watch atom key fn)
	awVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "add-watch"))
	awVr.Value = Proc{Name: "procAddWatch", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 3, 3)
		a := EnsureObjectIsAtom(args[0], "add-watch requires an atom, got %s")
		key := args[1]
		fn := coretypes.EnsureObjectIsCallable(args[2], "watch function must be callable, got %s")
		ext := getOrCreateAtomExtras(a)
		ext.mu.Lock()
		ext.watches[key.ToString(false)] = atomWatch{key: key, fn: fn}
		ext.mu.Unlock()
		return a
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "add-watch"), awVr)

	// remove-watch — (remove-watch atom key)
	rwVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "remove-watch"))
	rwVr.Value = Proc{Name: "procRemoveWatch", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		a := EnsureObjectIsAtom(args[0], "remove-watch requires an atom, got %s")
		key := args[1]
		ext := getAtomExtras(a)
		if ext != nil {
			ext.mu.Lock()
			delete(ext.watches, key.ToString(false))
			ext.mu.Unlock()
		}
		return a
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "remove-watch"), rwVr)

	// compare-and-set! — (compare-and-set! atom oldval newval)
	casVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "compare-and-set!"))
	casVr.Value = Proc{Name: "procCompareAndSet", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 3, 3)
		a := EnsureObjectIsAtom(args[0], "compare-and-set! requires an atom, got %s")
		oldVal := args[1]
		newVal := args[2]
		old, ok := a.CompareAndSet(oldVal, newVal, func(v coretypes.Object) { validateAtom(a, v) })
		if ok {
			notifyWatches(a, old, newVal)
		}
		return coretypes.Boolean{B: ok}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "compare-and-set!"), casVr)
}

// ---- chunked_procs.go ----
func init() {
	registerChunkedSeqProcs()
}

func registerChunkedSeqProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	cbVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-buffer"))
	cbVr.Value = Proc{Name: "procChunkBuffer", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		n := coretypes.EnsureArgIsInt(args, 0).I
		return &corecollections.ChunkBuffer{Arr: make([]coretypes.Object, 0, n)}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-buffer"), cbVr)

	caVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-append"))
	caVr.Value = Proc{Name: "procChunkAppend", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		buf, ok := args[0].(*corecollections.ChunkBuffer)
		if !ok {
			panic(coretypes.RuntimeError("chunk-append requires a ChunkBuffer"))
		}
		buf.Arr, buf.CountN = corecollections.ChunkAppend(buf.Arr, args[1])
		return coretypes.RuntimeNil
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-append"), caVr)

	chunkVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk"))
	chunkVr.Value = Proc{Name: "procChunk", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		buf, ok := args[0].(*corecollections.ChunkBuffer)
		if !ok {
			panic(coretypes.RuntimeError("chunk requires a ChunkBuffer"))
		}
		return &corecollections.ArrayChunk{Arr: buf.Arr, Off: 0, End: len(buf.Arr)}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk"), chunkVr)

	cfVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-first"))
	cfVr.Value = Proc{Name: "procChunkFirst", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		if cc, ok := args[0].(*corecollections.ChunkedCons); ok {
			return cc.Chunk
		}
		s := coretypes.EnsureObjectIsSeqable(args[0], "chunk-first requires a seq").Seq()
		arr := corecollections.ChunkFirstSingle(s)
		return &corecollections.ArrayChunk{Arr: arr, Off: 0, End: len(arr)}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-first"), cfVr)

	crVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-rest"))
	crVr.Value = Proc{Name: "procChunkRest", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		if cc, ok := args[0].(*corecollections.ChunkedCons); ok {
			return corecollections.ChunkRestFromRest(cc.RestSeq, corecollections.EmptyList)
		}
		s := coretypes.EnsureObjectIsSeqable(args[0], "chunk-rest requires a seq").Seq()
		return s.Rest()
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-rest"), crVr)

	cnVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-next"))
	cnVr.Value = Proc{Name: "procChunkNext", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		if cc, ok := args[0].(*corecollections.ChunkedCons); ok {
			return corecollections.ChunkNextFromRest(cc.RestSeq, coretypes.RuntimeNil)
		}
		s := coretypes.EnsureObjectIsSeqable(args[0], "chunk-next requires a seq").Seq()
		r := s.Rest()
		if r.IsEmpty() {
			return coretypes.RuntimeNil
		}
		return r
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-next"), cnVr)

	ccVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-cons"))
	ccVr.Value = Proc{Name: "procChunkCons", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		ac, ok := args[0].(*corecollections.ArrayChunk)
		if !ok {
			panic(coretypes.RuntimeError("chunk-cons requires an ArrayChunk as first argument"))
		}
		if ac.Count() == 0 {
			if args[1] == nil || corert.IsNil(args[1]) {
				return corecollections.EmptyList
			}
			if s, ok := args[1].(coretypes.Seqable); ok {
				return s.Seq()
			}
			return corecollections.EmptyList
		}
		rest := corecollections.ChunkConsRest(args[1], corert.IsNil)
		return &corecollections.ChunkedCons{Chunk: ac, RestSeq: rest, Idx: 0}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-cons"), ccVr)

	csqVr := ns.Resolve("chunked-seq?")
	if csqVr != nil {
		csqVr.Value = Proc{Name: "procChunkedSeqQ", Fn: func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			_, ok := args[0].(*corecollections.ChunkedCons)
			return coretypes.MakeBoolean(ok)
		}}
	}
}

// ---- core_api_gaps.go ----
// core_api_gaps.go — Fills remaining core API gaps from divergence matrix.

func init() {
	registerCoreAPIGaps()
}

func registerCoreAPIGaps() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// alter-var-root — (alter-var-root var fn & args)
	avrVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "alter-var-root"))
	avrVr.Value = Proc{Name: "procAlterVarRoot", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			PanicArityMinMax(len(args), 2, 999)
		}
		vr := EnsureObjectIsVar(args[0], "alter-var-root requires a var, got %s")
		fn := coretypes.EnsureObjectIsCallable(args[1], "alter-var-root requires a function, got %s")
		fnArgs := make([]coretypes.Object, 1+len(args)-2)
		fnArgs[0] = vr.Value
		for i := 2; i < len(args); i++ {
			fnArgs[i-1] = args[i]
		}
		vr.Value = fn.Call(fnArgs)
		return vr.Value
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "alter-var-root"), avrVr)

	// re-groups — (re-groups matcher) — returns groups from last regex match
	// In Joker, re-find already returns groups. Provide re-groups for compat.
	rgVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "re-groups"))
	rgVr.Value = Proc{Name: "procReGroups", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		// re-groups expects a Matcher, but Joker doesn't have Matcher objects.
		// Instead, accept [pattern string] and return groups.
		switch v := args[0].(type) {
		case *corecollections.ArrayVector:
			if v.Count() >= 2 {
				re := coretypes.EnsureObjectIsRegex(v.At(0), "re-groups requires [regex string]")
				s := coretypes.EnsureObjectIsString(v.At(1), "re-groups requires [regex string]")
				matches := regexp.MustCompile(re.R.String()).FindStringSubmatch(s.S)
				if matches == nil {
					return NIL
				}
				if len(matches) == 1 {
					return coretypes.String{S: matches[0]}
				}
				result := corecollections.EmptyArrayVector()
				for _, m := range matches {
					result = result.Conj(coretypes.String{S: m}).(*corecollections.ArrayVector)
				}
				return result
			}
		}
		return NIL
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "re-groups"), rgVr)

	// file-seq — (file-seq dir) — returns a lazy seq of files
	fsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "file-seq"))
	fsVr.Value = Proc{Name: "procFileSeq", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		dir := coretypes.EnsureObjectIsString(args[0], "file-seq requires a string path, got %s")
		var files []coretypes.Object
		filepath.Walk(dir.S, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			files = append(files, coretypes.String{S: path})
			return nil
		})
		if len(files) == 0 {
			return NIL
		}
		return &corecollections.ArraySeq{Arr: files, Index: 0}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "file-seq"), fsVr)

	// var-get — (var-get var)
	vgVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "var-get"))
	vgVr.Value = Proc{Name: "procVarGet", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		vr := EnsureObjectIsVar(args[0], "var-get requires a var, got %s")
		if vr.Value == nil {
			return NIL
		}
		return vr.Value
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "var-get"), vgVr)

	// var-set — (var-set var val)
	vsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "var-set"))
	vsVr.Value = Proc{Name: "procVarSet", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		vr := EnsureObjectIsVar(args[0], "var-set requires a var, got %s")
		vr.Value = args[1]
		return args[1]
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "var-set"), vsVr)

	// var? — (var? x)
	vqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "var?"))
	vqVr.Value = Proc{Name: "procVarQ", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		_, ok := args[0].(*Var)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "var?"), vqVr)
}

// ---- unchecked_arith.go ----
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

// ---- sorted_colls.go ----
// sorted_colls.go — sorted-map, sorted-set, sorted-map-by, sorted-set-by.
//
// Implementation: delegates to corecollections.ArrayMap/corecollections.MapSet but sorts entries on creation.
// Not a true balanced tree — O(n log n) creation, O(n) lookup.
// Sufficient for parity; can be upgraded to a tree later.

var sortedMetaCache coretypes.Map

func sortedCollMeta() coretypes.Map {
	if sortedMetaCache != nil {
		return sortedMetaCache
	}
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "sorted"), coretypes.Boolean{B: true})
	sortedMetaCache = m
	return sortedMetaCache
}

func init() {
	registerSortedCollProcs()
}

func registerSortedCollProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// sorted-map — (sorted-map k1 v1 k2 v2 ...)
	smVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sorted-map"))
	smVr.Value = Proc{Name: "procSortedMap", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args)%2 != 0 {
			panic(coretypes.RuntimeError("sorted-map requires an even number of arguments"))
		}
		pairs := sortedKeyValuePairs(args, nil)
		m := corecollections.EmptyArrayMap()
		for _, p := range pairs {
			addOrReplaceSortedMap(m, p.Key, p.Val, nil)
		}
		return m.WithMeta(sortedCollMeta())
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "sorted-map"), smVr)

	// sorted-map-by — (sorted-map-by comparator k1 v1 k2 v2 ...)
	smbVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sorted-map-by"))
	smbVr.Value = Proc{Name: "procSortedMapBy", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 999)
		comp := coretypes.EnsureArgIsCallable(args, 0)
		keyvals := args[1:]
		if len(keyvals)%2 != 0 {
			panic(coretypes.RuntimeError("sorted-map-by requires an even number of key/value arguments"))
		}
		pairs := sortedKeyValuePairs(keyvals, comp)
		m := corecollections.EmptyArrayMap()
		for _, p := range pairs {
			addOrReplaceSortedMap(m, p.Key, p.Val, comp)
		}
		return m.WithMeta(sortedCollMeta())
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "sorted-map-by"), smbVr)

	// sorted-set — (sorted-set v1 v2 ...)
	ssVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sorted-set"))
	ssVr.Value = Proc{Name: "procSortedSet", Fn: func(args []coretypes.Object) coretypes.Object {
		return sortedSetFrom(args, nil)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "sorted-set"), ssVr)

	// sorted-set-by — (sorted-set-by comparator v1 v2 ...)
	ssbVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sorted-set-by"))
	ssbVr.Value = Proc{Name: "procSortedSetBy", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 999)
		return sortedSetFrom(args[1:], coretypes.EnsureArgIsCallable(args, 0))
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "sorted-set-by"), ssbVr)

	// sorted? — (sorted? coll)
	sortedQVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "sorted?"))
	sortedQVr.Value = Proc{Name: "procSortedQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		if m, ok := args[0].(coretypes.Meta); ok {
			meta := m.GetMeta()
			if meta != nil {
				if ok, v := meta.Get(coretypes.MakeKeyword(STRINGS.Intern, "sorted")); ok {
					return coretypes.MakeBoolean(corert.ToBool(v))
				}
			}
		}
		return coretypes.Boolean{B: false}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "sorted?"), sortedQVr)

	// subseq/rsubseq — range queries over sorted coll API.
	subseqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "subseq"))
	subseqVr.Value = Proc{Name: "procSubseq", Fn: func(args []coretypes.Object) coretypes.Object {
		return sortedSubseq(args, false)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "subseq"), subseqVr)

	rsubseqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "rsubseq"))
	rsubseqVr.Value = Proc{Name: "procRsubseq", Fn: func(args []coretypes.Object) coretypes.Object {
		return sortedSubseq(args, true)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "rsubseq"), rsubseqVr)

	// comparator — (comparator pred) — wraps a boolean predicate into a comparator fn
	compVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "comparator"))
	compVr.Value = Proc{Name: "procComparator", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		pred := coretypes.EnsureArgIsCallable(args, 0)
		return Proc{Name: "procComparatorFn", Fn: func(cArgs []coretypes.Object) coretypes.Object {
			runtimeCheckArity(cArgs, 2, 2)
			if corert.ToBool(pred.Call(cArgs)) {
				return coretypes.Int{I: -1}
			}
			if corert.ToBool(call2(pred, cArgs[1], cArgs[0])) {
				return coretypes.Int{I: 1}
			}
			return coretypes.Int{I: 0}
		}}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "comparator"), compVr)
}

func sortedKeyValuePairs(keyvals []coretypes.Object, comp coretypes.Callable) []corecollections.KV[coretypes.Object, coretypes.Object] {
	pairs := corecollections.FlatToKVs(keyvals)
	corecollections.SortKVsBy(pairs, func(a, b coretypes.Object) bool {
		if comp != nil {
			return compareWith(comp, a, b) < 0
		}
		return compareObjects(a, b) < 0
	})
	return pairs
}

func addOrReplaceSortedMap(m *corecollections.ArrayMap, key coretypes.Object, val coretypes.Object, comp coretypes.Callable) {
	if comp != nil {
		for i := 0; i < len(m.Arr); i += 2 {
			if compareWith(comp, m.Arr[i], key) == 0 {
				m.Arr[i] = key
				m.Arr[i+1] = val
				return
			}
		}
		m.Add(key, val)
		return
	}
	if m.Add(key, val) {
		return
	}
	if i := corecollections.MapIndexOf(m.Arr, key); i != -1 {
		m.Arr[i+1] = val
	}
}

func sortedSetFrom(values []coretypes.Object, comp coretypes.Callable) coretypes.Object {
	sorted := make([]coretypes.Object, len(values))
	copy(sorted, values)
	corecollections.SortBy(sorted, func(a, b coretypes.Object) bool {
		if comp != nil {
			return compareWith(comp, a, b) < 0
		}
		return compareObjects(a, b) < 0
	})
	s := corecollections.EmptySet()
	for _, v := range sorted {
		s = s.Conj(v).(*corecollections.MapSet)
	}
	return s.WithMeta(sortedCollMeta())
}

func compareWith(comp coretypes.Callable, a, b coretypes.Object) int {
	return compare(comp, a, b)
}

func sortedSubseq(args []coretypes.Object, reverse bool) coretypes.Object {
	if len(args) != 3 && len(args) != 5 {
		coretypes.RuntimePanicArityMinMax(len(args), 3, 5)
	}
	coll := args[0]
	entries := sortedEntries(coll)
	if reverse {
		corecollections.Reverse(entries)
	}
	startPred := coretypes.EnsureObjectIsCallable(args[1], "subseq predicate must be callable, got %s")
	startKey := args[2]
	var endPred coretypes.Callable
	var endKey coretypes.Object
	if len(args) == 5 {
		endPred = coretypes.EnsureObjectIsCallable(args[3], "subseq predicate must be callable, got %s")
		endKey = args[4]
	}
	out := make([]coretypes.Object, 0)
	for _, e := range entries {
		k := rangeKey(e)
		if !rangePred(startPred, k, startKey) {
			continue
		}
		if endPred != nil && !rangePred(endPred, k, endKey) {
			continue
		}
		out = append(out, e)
	}
	if len(out) == 0 {
		return NIL
	}
	return &corecollections.ArraySeq{Arr: out, Index: 0}
}

func sortedEntries(coll coretypes.Object) []coretypes.Object {
	out := make([]coretypes.Object, 0)
	preserveOrder := isSortedColl(coll)
	if m, ok := coll.(coretypes.Map); ok {
		for it := m.Iter(); it.HasNext(); {
			p := it.Next()
			out = append(out, corecollections.NewArrayVectorFrom(p.Key, p.Value))
		}
		if !preserveOrder {
			corecollections.SortBy(out, func(a, b coretypes.Object) bool { return compareObjects(rangeKey(a), rangeKey(b)) < 0 })
		}
		return out
	}
	if s, ok := coll.(coretypes.Seqable); ok {
		for seq := s.Seq(); !seq.IsEmpty(); seq = seq.Rest() {
			out = append(out, seq.First())
		}
		if !preserveOrder {
			corecollections.SortBy(out, func(a, b coretypes.Object) bool { return compareObjects(a, b) < 0 })
		}
	}
	return out
}

func isSortedColl(coll coretypes.Object) bool {
	m, ok := coll.(coretypes.Meta)
	if !ok || m.GetMeta() == nil {
		return false
	}
	ok, v := m.GetMeta().Get(coretypes.MakeKeyword(STRINGS.Intern, "sorted"))
	return ok && corert.ToBool(v)
}

func rangeKey(entry coretypes.Object) coretypes.Object {
	if v, ok := entry.(coretypes.Vec); ok && v.Count() >= 1 {
		return v.Nth(0)
	}
	return entry
}

func rangePred(pred coretypes.Callable, a, b coretypes.Object) bool {
	if name := hotReducerName(pred); name != "" {
		switch name {
		case "procLt":
			return compareObjects(a, b) < 0
		case "procLte":
			return compareObjects(a, b) <= 0
		case "procGt":
			return compareObjects(a, b) > 0
		case "procGte":
			return compareObjects(a, b) >= 0
		}
	}
	return corert.ToBool(call2(pred, a, b))
}

// compareObjects provides a default ordering for Clojure values.
func compareObjects(a, b coretypes.Object) int {
	return corecollections.CompareObjectsDefault(a, b)
}

// ---- transient.go ----
var transientProcsOnce sync.Once

func init() {
	installTransientBridges()
	initTransientProcs()
}

func installTransientBridges() {
	if coretypes.TransientMutationError == nil {
		coretypes.TransientMutationError = func() any { return coretypes.RuntimeError("Cannot mutate a frozen transient") }
	}
	if coretypes.TransientVectorIndexTypeError == nil {
		coretypes.TransientVectorIndexTypeError = func(obj coretypes.Object) any { return RT.NewArgTypeError(1, obj, "Int") }
	}
	if coretypes.TransientVectorToPersistent == nil {
		coretypes.TransientVectorToPersistent = func(arr []coretypes.Object) coretypes.Object { return &corecollections.ArrayVector{Arr: arr} }
	}
	if coretypes.TransientMapToPersistent == nil {
		coretypes.TransientMapToPersistent = func(tm *coretypes.TransientMap) coretypes.Object {
			if tm.CountN <= int(corecollections.HASHMAP_THRESHOLD/2) {
				res := corecollections.EmptyArrayMap()
				for k, v := range tm.SM {
					res.Add(coretypes.String{S: k}, v)
				}
				for _, bucket := range tm.M {
					for _, e := range bucket {
						res.Add(e.Key, e.Val)
					}
				}
				return res
			}
			res := corecollections.EmptyHashMap
			for k, v := range tm.SM {
				res = res.Assoc(coretypes.String{S: k}, v).(*corecollections.HashMap)
			}
			for _, bucket := range tm.M {
				for _, e := range bucket {
					res = res.Assoc(e.Key, e.Val).(*corecollections.HashMap)
				}
			}
			return res
		}
	}
}

func initTransientProcs() {
	transientProcsOnce.Do(func() {
		ns := GLOBAL_ENV.CoreNamespace
		procs := []struct {
			name  string
			fn    func([]coretypes.Object) coretypes.Object
			pname string
		}{
			{"transient", procTransient, "procTransient"},
			{"assoc!", procAssocBang, "procAssocBang"},
			{"conj!", procConjBang, "procConjBang"},
			{"persistent!", procPersistentBang, "procPersistentBang"},
		}
		for _, p := range procs {
			sym := coretypes.MakeSymbol(STRINGS.Intern, p.name)
			vr := ns.Intern(sym)
			vr.Value = Proc{Fn: p.fn, Name: p.pname}
			referToUser(sym, vr)
		}

		tqSym := coretypes.MakeSymbol(STRINGS.Intern, "transient?")
		tqVr := ns.Intern(tqSym)
		tqVr.Value = Proc{Name: "procTransientQ", Fn: procIsTransient}
		referToUser(tqSym, tqVr)

		popSym := coretypes.MakeSymbol(STRINGS.Intern, "pop!")
		popVr := ns.Intern(popSym)
		popVr.Value = Proc{Name: "procPopBang", Fn: procPopBang}
		referToUser(popSym, popVr)
	})
}

var procTransient = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *corecollections.ArrayVector:
		return coretypes.ToTransient(coll.Arr)
	case coretypes.Map:
		return coretypes.MapToTransient(coll)
	default:
		panic(coretypes.RuntimeError("transient not supported on: " + coll.GetType().ToString(false)))
	}
}

var procAssocBang = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 3, 3)
	switch coll := args[0].(type) {
	case *coretypes.TransientVector:
		return coll.AssocInPlace(args[1], args[2])
	case *coretypes.TransientMap:
		return coll.AssocInPlace(args[1], args[2])
	default:
		panic(coretypes.RuntimeError("assoc! requires a transient, got: " + coll.GetType().ToString(false)))
	}
}

var procConjBang = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 2, 3)
	switch coll := args[0].(type) {
	case *coretypes.TransientVector:
		if len(args) != 2 {
			coretypes.RuntimePanicArityMinMax(len(args), 2, 2)
		}
		return coll.ConjInPlace(args[1])
	case *coretypes.TransientMap:
		if len(args) == 3 {
			return coll.AssocInPlace(args[1], args[2])
		}
		if k, v, ok := corecollections.TransientMapConjEntry(args[1]); ok {
			return coll.AssocInPlace(k, v)
		}
		panic(coretypes.RuntimeError("conj! on transient map requires a key/value pair"))
	default:
		panic(coretypes.RuntimeError("conj! requires a transient, got: " + coll.GetType().ToString(false)))
	}
}

var procPersistentBang = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *coretypes.TransientVector:
		return coll.ToPersistent()
	case *coretypes.TransientMap:
		return coll.ToPersistent()
	default:
		panic(coretypes.RuntimeError("persistent! requires a transient, got: " + coll.GetType().ToString(false)))
	}
}

var procIsTransient = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	return coretypes.MakeBoolean(corecollections.IsTransientObject(args[0]))
}

var procPopBang = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	switch coll := args[0].(type) {
	case *coretypes.TransientVector:
		return coll.PopInPlace()
	default:
		panic(coretypes.RuntimeError("pop! requires a transient vector, got: " + coll.GetType().ToString(false)))
	}
}

// ---- concurrency_ext.go ----
// concurrency_ext.go — Extended concurrency primitives: alts!, timeout, future, promise, pmap.
//
// These require the GIL-free runtime (goroutine_rt.go).

func checkedMillisecondDuration(ms int, context string) time.Duration {
	return corert.CheckedMillisecondDuration(ms, context, func(msg string) any { return coretypes.RuntimeError(msg) })
}

// installConcurrencyExt registers alts!, timeout, future, promise, deliver,
// future?, promise?, realized?, pmap, and pcalls.
func installConcurrencyExt() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// timeout — returns a channel that closes after ms milliseconds.
	// (timeout ms) -> Channel
	toVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "timeout"))
	toVr.Value = Proc{Name: "procTimeout", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		delay := checkedMillisecondDuration(coretypes.EnsureArgIsInt(args, 0).I, "timeout")
		ch := corert.NewObjectChannel(make(chan corert.FutureResult, 1))
		go func() {
			time.Sleep(delay)
			ch.Close()
		}()
		return ch
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "timeout"), toVr)

	// alts! — select-style multi-channel wait.
	// (alts! ports & opts) where ports is a vector of channels (take) or
	// [channel value] pairs (put).
	// Returns [value channel].
	// Options: :default val — return immediately if nothing ready.
	altsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "alts!"))
	altsVr.Value = Proc{Name: "procAlts", Fn: procAlts}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "alts!"), altsVr)

	// future — runs body in a goroutine, returns a deref-able object.
	// (future body...) is a macro defined in core.joke; the runtime primitive is future-call.
	fcVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "future-call"))
	fcVr.Value = Proc{Name: "procFutureCall", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		f := coretypes.EnsureArgIsCallable(args, 0)
		fut := corert.NewObjectFuture()
		go func() {
			registerGoroutineRT()
			defer unregisterGoroutineRT()
			var value coretypes.Object = NIL
			var err coretypes.Error
			defer func() {
				if r := recover(); r != nil {
					switch e := r.(type) {
					case coretypes.Error:
						err = e
					default:
						err = coretypes.RuntimeError("future panic").(coretypes.Error)
					}
				}
				fut.Complete(value, err)
			}()
			value = call0(f)
		}()
		return fut
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "future-call"), fcVr)

	// future — macro: (future body...) -> (future-call (fn [] body...))
	installMacro(ns, "future", func(args []coretypes.Object) coretypes.Object {
		// args: &form, &env, body...
		body := args[2:]
		fnForm := corecollections.NewListFrom(append([]coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "fn"), corecollections.NewVectorFrom()}, body...)...)
		return corecollections.NewListFrom(coretypes.MakeSymbol(STRINGS.Intern, "future-call"), fnForm)
	})

	// future? — true if obj is a Future.
	fqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "future?"))
	fqVr.Value = Proc{Name: "procFutureQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		_, ok := args[0].(*corert.ObjectFuture)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "future?"), fqVr)

	// promise — creates a promise that can be delivered once.
	// (promise) -> Promise
	prVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "promise"))
	prVr.Value = Proc{Name: "procPromise", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 0, 0)
		return corert.NewObjectPromise()
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "promise"), prVr)

	// deliver — delivers a value to a promise. Returns the promise.
	// (deliver p val) -> Promise
	dlVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "deliver"))
	dlVr.Value = Proc{Name: "procDeliver", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		p, ok := args[0].(*corert.ObjectPromise)
		if !ok {
			panic(coretypes.RuntimeError("deliver requires a promise"))
		}
		p.Deliver(args[1])
		return p
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "deliver"), dlVr)

	// promise? — true if obj is a Promise.
	pqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "promise?"))
	pqVr.Value = Proc{Name: "procPromiseQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		_, ok := args[0].(*corert.ObjectPromise)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "promise?"), pqVr)

	// realized? — true if a Future/Promise/coretypes.Delay has been realized.
	rzVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "realized?"))
	rzVr.Value = Proc{Name: "procRealizedQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		if p, ok := args[0].(coretypes.Pending); ok {
			return coretypes.MakeBoolean(p.IsRealized())
		}
		return coretypes.Boolean{B: false}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "realized?"), rzVr)

	// pmap — parallel map. (pmap f coll)
	// Applies f to each element in parallel goroutines, returns lazy seq of results in order.
	pmapVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "pmap"))
	pmapVr.Value = Proc{Name: "procPmap", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		f := coretypes.EnsureArgIsCallable(args, 0)
		coll := coretypes.EnsureObjectIsSeqable(args[1], "pmap requires a coretypes.Seqable collection").Seq()
		// Collect all elements first (pmap is not lazy in this impl).
		var elems []coretypes.Object
		for s := coll; !s.IsEmpty(); s = s.Rest() {
			elems = append(elems, s.First())
		}
		if len(elems) == 0 {
			return NIL
		}
		results := make([]coretypes.Object, len(elems))
		if r, panicked := corert.RunParallel(len(elems), func() { registerGoroutineRT() }, unregisterGoroutineRT, func(i int) {
			results[i] = call1(f, elems[i])
		}); panicked {
			panic(r)
		}
		return corecollections.NewListFrom(results...)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "pmap"), pmapVr)

	// pcalls — parallel calls. (pcalls & fns)
	// Calls each no-arg fn in parallel, returns list of results.
	pcVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "pcalls"))
	pcVr.Value = Proc{Name: "procPcalls", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 0 {
			return NIL
		}
		results := make([]coretypes.Object, len(args))
		fns := make([]coretypes.Callable, len(args))
		for i, arg := range args {
			fns[i] = coretypes.EnsureObjectIsCallable(arg, "pcalls requires callable arguments")
		}
		if r, panicked := corert.RunParallel(len(args), func() { registerGoroutineRT() }, unregisterGoroutineRT, func(i int) {
			results[i] = call0(fns[i])
		}); panicked {
			panic(r)
		}
		return corecollections.NewListFrom(results...)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "pcalls"), pcVr)
}

// procAlts implements (alts! ports & opts).
func procAlts(args []coretypes.Object) coretypes.Object {
	if len(args) < 1 {
		panic(coretypes.RuntimeError("alts! requires at least one argument (ports vector)"))
	}
	ports := coretypes.EnsureObjectIsSeqable(args[0], "alts! first arg must be a vector of ports").Seq()

	// Parse options.
	if len(args[1:])%2 != 0 {
		panic(coretypes.RuntimeError("alts! options must be key/value pairs"))
	}
	var defaultVal coretypes.Object
	hasDefault := false
	for i := 1; i+1 < len(args); i += 2 {
		if k, ok := args[i].(coretypes.Keyword); ok && k.ToString(false) == ":default" {
			defaultVal = args[i+1]
			hasDefault = true
		}
	}

	// Build reflect.Select cases.
	type portInfo struct {
		ch    *corert.ObjectChannel
		isPut bool
	}
	var cases []reflect.SelectCase
	var infos []portInfo

	for s := ports; !s.IsEmpty(); s = s.Rest() {
		item := s.First()
		switch v := item.(type) {
		case *corert.ObjectChannel:
			// Take operation.
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(v.Raw()),
			})
			infos = append(infos, portInfo{ch: v, isPut: false})
		default:
			// Check if it's a vector-like [channel value] for put.
			if ci, ok := item.(coretypes.CountedIndexed); ok && ci.Count() == 2 {
				ch := EnsureObjectIsChannel(ci.At(0), "alts! put port first element must be a channel")
				if ch.IsClosed() {
					// Clojure-like semantics: put on closed channel returns false immediately.
					return corecollections.NewVectorFrom(coretypes.MakeBoolean(false), ch)
				}
				val := ci.At(1)
				cases = append(cases, reflect.SelectCase{
					Dir:  reflect.SelectSend,
					Chan: reflect.ValueOf(ch.Raw()),
					Send: reflect.ValueOf(corert.NewFutureResult(val, nil)),
				})
				infos = append(infos, portInfo{ch: ch, isPut: true})
			} else {
				panic(coretypes.RuntimeError("alts! port must be a channel or [channel value] vector"))
			}
		}
	}

	if len(cases) == 0 {
		panic(coretypes.RuntimeError("alts! requires at least one port"))
	}

	// Add default case if :default option provided.
	if hasDefault {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectDefault})
	}

	// Select.
	chosen, recv, recvOK := reflect.Select(cases)

	// Default case.
	if hasDefault && chosen == len(cases)-1 {
		return corecollections.NewVectorFrom(defaultVal, coretypes.MakeKeyword(STRINGS.Intern, "default"))
	}

	info := infos[chosen]
	if info.isPut {
		// Put completed.
		return corecollections.NewVectorFrom(coretypes.MakeBoolean(true), info.ch)
	}
	// Take completed.
	if !recvOK {
		// Channel closed.
		return corecollections.NewVectorFrom(NIL, info.ch)
	}
	fr := recv.Interface().(corert.FutureResult)
	if fr.Err != nil {
		panic(fr.Err)
	}
	return corecollections.NewVectorFrom(fr.Value, info.ch)
}

func init() {
	corert.AgentRegisterGoroutine = func() { registerGoroutineRT() }
	corert.AgentUnregisterGoroutine = unregisterGoroutineRT
	installConcurrencyExt()
	installAgentExt()
}

func installAgentExt() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// agent — creates a new agent with initial value.
	agVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "agent"))
	agVr.Value = Proc{Name: "procAgent", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return corert.NewAgent(args[0])
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "agent"), agVr)

	// send — dispatches action to agent (returns agent immediately).
	sendVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "send"))
	sendVr.Value = Proc{Name: "procSend", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			panic(coretypes.RuntimeError("send requires at least 2 args: agent and fn"))
		}
		a, ok := args[0].(*corert.Agent)
		if !ok {
			panic(coretypes.RuntimeError("send first arg must be an agent"))
		}
		f := coretypes.EnsureObjectIsCallable(args[1], "send second arg must be a fn")
		a.Send(f, args[2:])
		return a
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "send"), sendVr)

	// send-off — same as send for this implementation (no thread pool distinction).
	soVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "send-off"))
	soVr.Value = Proc{Name: "procSendOff", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			panic(coretypes.RuntimeError("send-off requires at least 2 args: agent and fn"))
		}
		a, ok := args[0].(*corert.Agent)
		if !ok {
			panic(coretypes.RuntimeError("send-off first arg must be an agent"))
		}
		f := coretypes.EnsureObjectIsCallable(args[1], "send-off second arg must be a fn")
		a.Send(f, args[2:])
		return a
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "send-off"), soVr)

	// await — blocks until all actions dispatched to agents have completed.
	// Simple implementation: sends a sentinel and waits for it to be processed.
	awaitVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "await"))
	awaitVr.Value = Proc{Name: "procAwait", Fn: func(args []coretypes.Object) coretypes.Object {
		for _, arg := range args {
			a, ok := arg.(*corert.Agent)
			if !ok {
				panic(coretypes.RuntimeError("await requires agent arguments"))
			}
			a.Await()
		}
		return NIL
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "await"), awaitVr)

	// agent-error — returns any error that has occurred on the agent.
	aeVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "agent-error"))
	aeVr.Value = Proc{Name: "procAgentError", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		a, ok := args[0].(*corert.Agent)
		if !ok {
			panic(coretypes.RuntimeError("agent-error requires an agent"))
		}
		e := a.Error()
		if e == nil {
			return NIL
		}
		if eo, ok := e.(coretypes.Object); ok {
			return eo
		}
		return coretypes.MakeString(e.Error())
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "agent-error"), aeVr)
}

// ---- core_async_ext.go ----
// core_async_ext.go — clojure.core.async compatibility namespace.
//
// Joker's core already provides channels, go, alts!, timeout and blocking
// <!/>! operations. This file exposes a Clojure-shaped clojure.core.async
// namespace plus the most commonly used higher-level coordination helpers.

func init() {
	installCoreAsyncNamespace()
	fillNativeVarMetadata()
}

func installCoreAsyncNamespace() {
	if GLOBAL_ENV == nil || GLOBAL_ENV.CoreNamespace == nil {
		return
	}
	ns := GLOBAL_ENV.EnsureSymbolIsLib(coretypes.MakeSymbol(STRINGS.Intern, "clojure.core.async"))
	ns.Meta = MakeMeta(nil, "Clojure core.async-compatible channel helpers backed by Go goroutines.", "1.0")
	core := GLOBAL_ENV.CoreNamespace
	for _, name := range []string{"chan", "<!", ">!", "close!", "alts!", "timeout", "go"} {
		if vr := core.Resolve(name); vr != nil {
			ns.Refer(coretypes.MakeSymbol(STRINGS.Intern, name), vr)
		}
	}
	if vr := core.Resolve("<!"); vr != nil {
		ns.Refer(coretypes.MakeSymbol(STRINGS.Intern, "<!!"), vr)
	}
	if vr := core.Resolve(">!"); vr != nil {
		ns.Refer(coretypes.MakeSymbol(STRINGS.Intern, ">!!"), vr)
	}
	installAsyncMacro(ns, "go-loop", "Like core.async/go with an initial loop/recur binding vector.", macroCoreAsyncGoLoop)
	installAsyncMacro(ns, "thread", "Runs body asynchronously on a native goroutine and returns a future.", macroCoreAsyncThread)
	installAsyncMacro(ns, "thread-call", "Runs a zero-argument function asynchronously and returns a future.", macroCoreAsyncThreadCall)

	installAsyncProc(ns, "buffer", "Returns a fixed-size channel buffer descriptor.", procAsyncBuffer)
	installAsyncProc(ns, "dropping-buffer", "Returns a dropping channel buffer descriptor.", procAsyncBuffer)
	installAsyncProc(ns, "sliding-buffer", "Returns a sliding channel buffer descriptor.", procAsyncBuffer)
	installAsyncProc(ns, "promise-chan", "Returns a channel that accepts exactly one value then closes.", procAsyncPromiseChan)
	installAsyncProc(ns, "to-chan", "Copies a collection onto a new channel and closes it.", procAsyncToChan)
	installAsyncProc(ns, "to-chan!", "Alias for to-chan.", procAsyncToChan)
	installAsyncProc(ns, "onto-chan", "Copies a collection onto a channel, optionally closing it.", procAsyncOntoChan)
	installAsyncProc(ns, "onto-chan!", "Alias for onto-chan.", procAsyncOntoChan)
	installAsyncProc(ns, "put!", "Asynchronously puts a value on a channel and optionally invokes a callback.", procAsyncPutBang)
	installAsyncProc(ns, "take!", "Asynchronously takes a value from a channel and invokes a callback.", procAsyncTakeBang)
	installAsyncProc(ns, "pipe", "Pipes values from one channel to another.", procAsyncPipe)
	installAsyncProc(ns, "merge", "Merges multiple input channels onto one output channel.", procAsyncMerge)
	installAsyncProc(ns, "split", "Splits an input channel into true/false output channels by predicate.", procAsyncSplit)
	installAsyncProc(ns, "map<", "Maps a function over values taken from a channel.", procAsyncMapFrom)
	installAsyncProc(ns, "filter<", "Filters values taken from a channel by predicate.", procAsyncFilterFrom)
	installAsyncProc(ns, "map>", "Maps values before putting them on a channel.", procAsyncMapTo)
	installAsyncProc(ns, "filter>", "Filters values before putting them on a channel.", procAsyncFilterTo)
	installAsyncProc(ns, "reduce", "Reduces values from a channel and returns a result channel.", procAsyncReduce)
	installAsyncProc(ns, "into", "Collects values from a channel into a collection.", procAsyncInto)
	installAsyncProc(ns, "mult", "Creates a multicast source from a channel.", procAsyncMult)
	installAsyncProc(ns, "tap", "Adds a tap channel to a mult.", procAsyncTap)
	installAsyncProc(ns, "untap", "Removes a tap channel from a mult.", procAsyncUntap)
	installAsyncProc(ns, "untap-all", "Removes all tap channels from a mult.", procAsyncUntapAll)
	installAsyncProc(ns, "pub", "Creates a topic publication from a channel.", procAsyncPub)
	installAsyncProc(ns, "sub", "Subscribes a channel to a publication topic.", procAsyncSub)
	installAsyncProc(ns, "unsub", "Unsubscribes a channel from a publication topic.", procAsyncUnsub)
	installAsyncProc(ns, "unsub-all", "Unsubscribes channels from publication topics.", procAsyncUnsubAll)
}

func installAsyncProc(ns *Namespace, name, doc string, fn ProcFn) {
	ns.InternVar(name, Proc{Name: "procCoreAsync" + name, Fn: fn}, MakeMeta(nil, doc, "1.0"))
}

func installAsyncMacro(ns *Namespace, name, doc string, fn func([]coretypes.Object) coretypes.Object) {
	vr := ns.InternVar(name, Proc{Name: "macro" + name, Fn: fn}, MakeMeta(nil, doc, "1.0"))
	vr.isMacro = true
}

func macroCoreAsyncGoLoop(args []coretypes.Object) coretypes.Object {
	if len(args) < 3 {
		panic(coretypes.RuntimeError("go-loop requires bindings and body"))
	}
	return listObjs(coretypes.MakeSymbol(STRINGS.Intern, "go"), corecollections.NewListFrom(append([]coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "loop"), args[2]}, args[3:]...)...))
}
func macroCoreAsyncThread(args []coretypes.Object) coretypes.Object {
	if len(args) < 2 {
		panic(coretypes.RuntimeError("thread requires body"))
	}
	return listObjs(coretypes.MakeSymbol(STRINGS.Intern, "future"), doObj(args[2:]...))
}
func macroCoreAsyncThreadCall(args []coretypes.Object) coretypes.Object {
	if len(args) != 3 {
		panic(coretypes.RuntimeError("thread-call requires one fn"))
	}
	return listObjs(coretypes.MakeSymbol(STRINGS.Intern, "future-call"), args[2])
}

func asyncBufferSize(o coretypes.Object) int {
	if o == nil || o.Equals(NIL) {
		return 0
	}
	switch v := o.(type) {
	case coretypes.Int:
		return v.I
	default:
		panic(coretypes.RuntimeError("buffer size must be an integer"))
	}
}
func procAsyncBuffer(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	return coretypes.EnsureArgIsInt(args, 0)
}
func procAsyncPromiseChan(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 0, 0)
	return corert.NewObjectChannel(make(chan corert.FutureResult, 1))
}

func channelFromArg(args []coretypes.Object, i int) *corert.ObjectChannel {
	return EnsureObjectIsChannel(args[i], fmt.Sprintf("arg %d must be a channel", i))
}
func asyncSend(ch *corert.ObjectChannel, v coretypes.Object) bool {
	if v == nil || v.Equals(NIL) {
		panic(coretypes.RuntimeError("Can't put nil on channel"))
	}
	return ch.Send(v)
}
func asyncRecv(ch *corert.ObjectChannel) coretypes.Object {
	v, _, err := ch.Receive(nil)
	if err != nil {
		panic(coretypes.RuntimeError(err.Error()))
	}
	return v
}

func procAsyncPutBang(args []coretypes.Object) coretypes.Object {
	if len(args) != 2 && len(args) != 3 {
		panic(coretypes.RuntimeError("put! requires channel, value, optional callback"))
	}
	ch := channelFromArg(args, 0)
	v := args[1]
	var cb coretypes.Callable
	if len(args) == 3 {
		cb = coretypes.EnsureArgIsCallable(args, 2)
	}
	go func() {
		registerGoroutineRT()
		ok := asyncSend(ch, v)
		if cb != nil {
			call1(cb, coretypes.MakeBoolean(ok))
		}
	}()
	return coretypes.MakeBoolean(!ch.IsClosed())
}

func procAsyncTakeBang(args []coretypes.Object) coretypes.Object {
	if len(args) != 2 && len(args) != 3 {
		panic(coretypes.RuntimeError("take! requires channel, callback, optional on-caller?"))
	}
	ch := channelFromArg(args, 0)
	cb := coretypes.EnsureArgIsCallable(args, 1)
	go func() { registerGoroutineRT(); call1(cb, asyncRecv(ch)) }()
	return NIL
}

func procAsyncToChan(args []coretypes.Object) coretypes.Object {
	if len(args) < 1 || len(args) > 2 {
		panic(coretypes.RuntimeError("to-chan requires coll and optional close?"))
	}
	ch := corert.NewObjectChannel(make(chan corert.FutureResult, 0))
	closeOut := true
	if len(args) == 2 {
		closeOut = corert.ToBool(args[1])
	}
	seq := coretypes.EnsureObjectIsSeqable(args[0], "to-chan requires seqable").Seq()
	go func() {
		registerGoroutineRT()
		for !seq.IsEmpty() {
			asyncSend(ch, seq.First())
			seq = seq.Rest()
		}
		if closeOut {
			ch.Close()
		}
	}()
	return ch
}

func procAsyncOntoChan(args []coretypes.Object) coretypes.Object {
	if len(args) < 2 || len(args) > 3 {
		panic(coretypes.RuntimeError("onto-chan requires channel, coll, optional close?"))
	}
	ch := channelFromArg(args, 0)
	seq := coretypes.EnsureObjectIsSeqable(args[1], "onto-chan requires seqable").Seq()
	closeOut := true
	if len(args) == 3 {
		closeOut = corert.ToBool(args[2])
	}
	go func() {
		registerGoroutineRT()
		for !seq.IsEmpty() {
			asyncSend(ch, seq.First())
			seq = seq.Rest()
		}
		if closeOut {
			ch.Close()
		}
	}()
	return ch
}

func procAsyncPipe(args []coretypes.Object) coretypes.Object {
	if len(args) < 2 || len(args) > 3 {
		panic(coretypes.RuntimeError("pipe requires from, to, optional close?"))
	}
	from, to := channelFromArg(args, 0), channelFromArg(args, 1)
	closeOut := true
	if len(args) == 3 {
		closeOut = corert.ToBool(args[2])
	}
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(from)
			if v.Equals(NIL) {
				if closeOut {
					to.Close()
				}
				return
			}
			asyncSend(to, v)
		}
	}()
	return to
}

func procAsyncMerge(args []coretypes.Object) coretypes.Object {
	if len(args) < 1 || len(args) > 2 {
		panic(coretypes.RuntimeError("merge requires channels and optional buffer"))
	}
	chsSeq := coretypes.EnsureObjectIsSeqable(args[0], "merge requires seqable channels").Seq()
	out := corert.NewObjectChannel(make(chan corert.FutureResult, 0))
	var wg sync.WaitGroup
	for !chsSeq.IsEmpty() {
		ch := EnsureObjectIsChannel(chsSeq.First(), "merge element must be channel")
		wg.Add(1)
		go func(c *corert.ObjectChannel) {
			defer wg.Done()
			registerGoroutineRT()
			for {
				v := asyncRecv(c)
				if v.Equals(NIL) {
					return
				}
				asyncSend(out, v)
			}
		}(ch)
		chsSeq = chsSeq.Rest()
	}
	go func() { wg.Wait(); out.Close() }()
	return out
}

func procAsyncSplit(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 2, 2)
	pred := coretypes.EnsureArgIsCallable(args, 0)
	in := channelFromArg(args, 1)
	t := corert.NewObjectChannel(make(chan corert.FutureResult))
	f := corert.NewObjectChannel(make(chan corert.FutureResult))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(in)
			if v.Equals(NIL) {
				t.Close()
				f.Close()
				return
			}
			if corert.ToBool(call1(pred, v)) {
				asyncSend(t, v)
			} else {
				asyncSend(f, v)
			}
		}
	}()
	return corecollections.NewVectorFrom(t, f)
}

func procAsyncMapFrom(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 2, 2)
	xf := coretypes.EnsureArgIsCallable(args, 0)
	in := channelFromArg(args, 1)
	out := corert.NewObjectChannel(make(chan corert.FutureResult))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(in)
			if v.Equals(NIL) {
				out.Close()
				return
			}
			asyncSend(out, call1(xf, v))
		}
	}()
	return out
}
func procAsyncFilterFrom(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 2, 2)
	pred := coretypes.EnsureArgIsCallable(args, 0)
	in := channelFromArg(args, 1)
	out := corert.NewObjectChannel(make(chan corert.FutureResult))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(in)
			if v.Equals(NIL) {
				out.Close()
				return
			}
			if corert.ToBool(call1(pred, v)) {
				asyncSend(out, v)
			}
		}
	}()
	return out
}
func procAsyncMapTo(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 2, 2)
	xf := coretypes.EnsureArgIsCallable(args, 0)
	ch := channelFromArg(args, 1)
	out := corert.NewObjectChannel(make(chan corert.FutureResult))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(out)
			if v.Equals(NIL) {
				ch.Close()
				return
			}
			asyncSend(ch, call1(xf, v))
		}
	}()
	return out
}
func procAsyncFilterTo(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 2, 2)
	pred := coretypes.EnsureArgIsCallable(args, 0)
	ch := channelFromArg(args, 1)
	out := corert.NewObjectChannel(make(chan corert.FutureResult))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(out)
			if v.Equals(NIL) {
				ch.Close()
				return
			}
			if corert.ToBool(call1(pred, v)) {
				asyncSend(ch, v)
			}
		}
	}()
	return out
}

func procAsyncReduce(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 3, 3)
	f := coretypes.EnsureArgIsCallable(args, 0)
	acc := args[1]
	ch := channelFromArg(args, 2)
	out := corert.NewObjectChannel(make(chan corert.FutureResult, 1))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(ch)
			if v.Equals(NIL) {
				asyncSend(out, acc)
				out.Close()
				return
			}
			acc = call2(f, acc, v)
		}
	}()
	return out
}
func procAsyncInto(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 2, 2)
	init := args[0]
	ch := channelFromArg(args, 1)
	out := corert.NewObjectChannel(make(chan corert.FutureResult, 1))
	go func() {
		registerGoroutineRT()
		acc := init
		for {
			v := asyncRecv(ch)
			if v.Equals(NIL) {
				asyncSend(out, acc)
				out.Close()
				return
			}
			if c, ok := acc.(coretypes.Conjable); ok {
				acc = c.Conj(v).(coretypes.Object)
			} else {
				panic(coretypes.RuntimeError("into init is not conjable"))
			}
		}
	}()
	return out
}

type asyncMult struct {
	mu   sync.Mutex
	src  *corert.ObjectChannel
	taps map[*corert.ObjectChannel]bool
	hash uint32
}

func (m *asyncMult) ToString(bool) string                            { return "#object[core.async.Mult]" }
func (m *asyncMult) Print(w fmt.State, printReadably bool)           {}
func (m *asyncMult) Equals(o interface{}) bool                       { return m == o }
func (m *asyncMult) GetInfo() *coretypes.ObjectInfo                  { return nil }
func (m *asyncMult) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return m }
func (m *asyncMult) GetType() *coretypes.Type                        { return TYPE.Proc }
func (m *asyncMult) Hash() uint32                                    { return m.hash }

type asyncPub struct {
	mu      sync.Mutex
	src     *corert.ObjectChannel
	topicFn coretypes.Callable
	subs    map[string][]*corert.ObjectChannel
	hash    uint32
}

func (p *asyncPub) ToString(bool) string                            { return "#object[core.async.Pub]" }
func (p *asyncPub) Equals(o interface{}) bool                       { return p == o }
func (p *asyncPub) GetInfo() *coretypes.ObjectInfo                  { return nil }
func (p *asyncPub) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return p }
func (p *asyncPub) GetType() *coretypes.Type                        { return TYPE.Proc }
func (p *asyncPub) Hash() uint32                                    { return p.hash }

func procAsyncMult(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	src := channelFromArg(args, 0)
	m := &asyncMult{src: src, taps: map[*corert.ObjectChannel]bool{}}
	m.hash = hashutil.Ptr(uintptr(unsafe.Pointer(m)))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(src)
			m.mu.Lock()
			taps := make([]*corert.ObjectChannel, 0, len(m.taps))
			for t := range m.taps {
				taps = append(taps, t)
			}
			m.mu.Unlock()
			if v.Equals(NIL) {
				for _, t := range taps {
					t.Close()
				}
				return
			}
			for _, t := range taps {
				asyncSend(t, v)
			}
		}
	}()
	return m
}
func procAsyncTap(args []coretypes.Object) coretypes.Object {
	if len(args) < 2 || len(args) > 3 {
		panic(coretypes.RuntimeError("tap requires mult, channel, optional close?"))
	}
	m, ok := args[0].(*asyncMult)
	if !ok {
		panic(coretypes.RuntimeError("tap requires mult"))
	}
	ch := channelFromArg(args, 1)
	closep := true
	if len(args) == 3 {
		closep = corert.ToBool(args[2])
	}
	m.mu.Lock()
	m.taps[ch] = closep
	m.mu.Unlock()
	return ch
}
func procAsyncUntap(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 2, 2)
	m, ok := args[0].(*asyncMult)
	if !ok {
		panic(coretypes.RuntimeError("untap requires mult"))
	}
	ch := channelFromArg(args, 1)
	m.mu.Lock()
	delete(m.taps, ch)
	m.mu.Unlock()
	return NIL
}
func procAsyncUntapAll(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	m, ok := args[0].(*asyncMult)
	if !ok {
		panic(coretypes.RuntimeError("untap-all requires mult"))
	}
	m.mu.Lock()
	m.taps = map[*corert.ObjectChannel]bool{}
	m.mu.Unlock()
	return NIL
}

func procAsyncPub(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 2, 2)
	src := channelFromArg(args, 0)
	tf := coretypes.EnsureArgIsCallable(args, 1)
	p := &asyncPub{src: src, topicFn: tf, subs: map[string][]*corert.ObjectChannel{}}
	p.hash = hashutil.Ptr(uintptr(unsafe.Pointer(p)))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(src)
			p.mu.Lock()
			if v.Equals(NIL) {
				for _, ss := range p.subs {
					for _, ch := range ss {
						ch.Close()
					}
				}
				p.mu.Unlock()
				return
			}
			topic := call1(tf, v).ToString(false)
			ss := append([]*corert.ObjectChannel(nil), p.subs[topic]...)
			p.mu.Unlock()
			for _, ch := range ss {
				asyncSend(ch, v)
			}
		}
	}()
	return p
}
func procAsyncSub(args []coretypes.Object) coretypes.Object {
	if len(args) < 3 || len(args) > 4 {
		panic(coretypes.RuntimeError("sub requires pub, topic, channel, optional close?"))
	}
	p, ok := args[0].(*asyncPub)
	if !ok {
		panic(coretypes.RuntimeError("sub requires pub"))
	}
	topic := args[1].ToString(false)
	ch := channelFromArg(args, 2)
	p.mu.Lock()
	p.subs[topic] = append(p.subs[topic], ch)
	p.mu.Unlock()
	return ch
}
func procAsyncUnsub(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 3, 3)
	p, ok := args[0].(*asyncPub)
	if !ok {
		panic(coretypes.RuntimeError("unsub requires pub"))
	}
	topic := args[1].ToString(false)
	ch := channelFromArg(args, 2)
	p.mu.Lock()
	xs := p.subs[topic]
	ys := xs[:0]
	for _, c := range xs {
		if c != ch {
			ys = append(ys, c)
		}
	}
	if len(ys) == 0 {
		delete(p.subs, topic)
	} else {
		p.subs[topic] = ys
	}
	p.mu.Unlock()
	return NIL
}
func procAsyncUnsubAll(args []coretypes.Object) coretypes.Object {
	if len(args) < 1 || len(args) > 2 {
		panic(coretypes.RuntimeError("unsub-all requires pub and optional topic"))
	}
	p, ok := args[0].(*asyncPub)
	if !ok {
		panic(coretypes.RuntimeError("unsub-all requires pub"))
	}
	p.mu.Lock()
	if len(args) == 2 {
		delete(p.subs, args[1].ToString(false))
	} else {
		p.subs = map[string][]*corert.ObjectChannel{}
	}
	p.mu.Unlock()
	return NIL
}

// ---- go-spew default ----
var procGoSpew = func(args []coretypes.Object) (res coretypes.Object) {
	return coretypes.MakeBoolean(false)
}
