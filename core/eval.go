package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	corereader "github.com/rcarmo/go-joker/core/reader"
	"github.com/rcarmo/go-joker/core/types/numerical"
	"io"
	"maps"
	"math/big"
	"math/rand"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"sync"
	"unsafe"

	coretypes "github.com/rcarmo/go-joker/core/types"

	"github.com/rcarmo/go-joker/core/bufferpool"
	"github.com/rcarmo/go-joker/core/hashutil"
	corert "github.com/rcarmo/go-joker/core/runtime"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

type (
	Traceable interface {
		Name() string
		Pos() coretypes.Position
	}
	EvalError struct {
		msg  string
		pos  coretypes.Position
		rt   *goroutineRT
		hash uint32
	}
	Frame struct {
		traceable Traceable
	}
	Callstack struct {
		frames []Frame
	}
	Runtime struct{}
)

var RT *Runtime = &Runtime{}

// cloneGRT captures a snapshot of current goroutine state for error reporting.
func cloneGRT() *goroutineRT {
	grt := currentGRT()
	return &goroutineRT{
		callstack:   grt.callstack.clone(),
		currentExpr: grt.currentExpr,
	}
}

func (rt *Runtime) NewError(msg string) *EvalError {
	grt := cloneGRT()
	res := &EvalError{
		msg: msg,
		rt:  grt,
	}
	if grt.currentExpr != nil {
		res.pos = grt.currentExpr.Pos()
	}
	return res
}

func (rt *Runtime) NewArgTypeError(index int, obj coretypes.Object, expectedType string) *EvalError {
	grt := currentGRT()
	name := "<unknown>"
	if grt.currentExpr != nil {
		if tr, ok := grt.currentExpr.(Traceable); ok {
			name = tr.Name()
		}
	}
	return rt.NewError(fmt.Sprintf("Arg[%d] of %s must have type %s, got %s", index, name, expectedType, obj.GetType().ToString(false)))
}

func (rt *Runtime) NewErrorWithPos(msg string, pos coretypes.Position) *EvalError {
	grt := cloneGRT()
	return &EvalError{
		msg: msg,
		pos: pos,
		rt:  grt,
	}
}

func (rt *Runtime) stacktrace() string {
	grt := currentGRT()
	return grt.stacktrace()
}

func (grt *goroutineRT) stacktrace() string {
	b := bufferpool.Get()
	defer bufferpool.Put(b)
	pos := coretypes.Position{}
	if grt.currentExpr != nil {
		pos = grt.currentExpr.Pos()
	}
	name := "global"
	for _, f := range grt.callstack.frames {
		framePos := f.traceable.Pos()
		b.WriteString(fmt.Sprintf("  %s %s:%d:%d\n", name, framePos.FilenameOrUnknown(), framePos.StartLine, framePos.StartColumn))
		name = corestr.TrimVarQuotePrefix(f.traceable.Name())
	}
	b.WriteString(fmt.Sprintf("  %s %s:%d:%d", name, pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn))
	return b.String()
}

func (rt *Runtime) pushFrame() {
	grt := currentGRT()
	var tr Traceable
	if grt.currentExpr != nil {
		if t, ok := grt.currentExpr.(Traceable); ok {
			tr = t
		} else {
			tr = &CallExpr{}
		}
	} else {
		tr = &CallExpr{}
	}
	grt.callstack.pushFrame(Frame{traceable: tr})
}

func (rt *Runtime) popFrame() {
	grt := currentGRT()
	grt.callstack.popFrame()
}

func restoreCurrentExpr(expr Expr) {
	currentGRT().currentExpr = expr
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
	parentExpr := grt.currentExpr
	grt.currentExpr = expr
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
					defer func() {
						if r := recover(); r != nil {
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
				defer func() {
					if r := recover(); r != nil {
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

func (s *Callstack) pushFrame(frame Frame) {
	s.frames = append(s.frames, frame)
}

func (s *Callstack) popFrame() {
	s.frames = s.frames[:len(s.frames)-1]
}

func (s *Callstack) clone() *Callstack {
	res := &Callstack{frames: make([]Frame, len(s.frames))}
	copy(res.frames, s.frames)
	return res
}

func (s *Callstack) String() string {
	b := bufferpool.Get()
	defer bufferpool.Put(b)
	for _, f := range s.frames {
		pos := f.traceable.Pos()
		b.WriteString(fmt.Sprintf("%s %s:%d:%d\n", f.traceable.Name(), pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn))
	}
	if b.Len() > 0 {
		b.Truncate(b.Len() - 1)
	}
	return b.String()
}

func MakeEvalError(msg string, pos coretypes.Position, grt *goroutineRT) *EvalError {
	res := &EvalError{msg, pos, grt, 0}
	res.hash = hashutil.Ptr(uintptr(unsafe.Pointer(res)))
	return res
}

func (err *EvalError) ToString(escape bool) string {
	return err.Error()
}

func (err *EvalError) Equals(other interface{}) bool {
	return err == other
}

func (err *EvalError) GetInfo() *coretypes.ObjectInfo {
	return nil
}

func (err *EvalError) GetType() *coretypes.Type {
	return TYPE.EvalError
}

func (err *EvalError) Hash() uint32 {
	return err.hash
}

func (err *EvalError) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	return err
}

func (err *EvalError) Message() coretypes.Object {
	return coretypes.MakeString(err.msg)
}

func (err *EvalError) Error() string {
	pos := err.pos
	if len(err.rt.callstack.frames) > 0 && !LINTER_MODE {
		return fmt.Sprintf("%s:%d:%d: Eval error: %s\nStacktrace:\n%s", pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn, err.msg, err.rt.stacktrace())
	} else {
		if len(err.rt.callstack.frames) > 0 {
			pos = err.rt.callstack.frames[0].traceable.Pos()
		}
		return fmt.Sprintf("%s:%d:%d: Eval error: %s", pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn, err.msg)
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
			switch callable.Name {
			case "procInc":
				switch x := Eval(expr.args[0], env).(type) {
				case coretypes.Int:
					return coretypes.Int{I: x.I + 1}
				case coretypes.Double:
					return coretypes.Double{D: x.D + 1}
				}
			case "procDec":
				switch x := Eval(expr.args[0], env).(type) {
				case coretypes.Int:
					return coretypes.Int{I: x.I - 1}
				case coretypes.Double:
					return coretypes.Double{D: x.D - 1}
				}
			case "procIsZero":
				switch x := Eval(expr.args[0], env).(type) {
				case coretypes.Int:
					return coretypes.Boolean{B: x.I == 0}
				case coretypes.Double:
					return coretypes.Boolean{B: x.D == 0}
				}
			case "procSubtract":
				switch x := Eval(expr.args[0], env).(type) {
				case coretypes.Int:
					return coretypes.Int{I: -x.I}
				case coretypes.Double:
					return coretypes.Double{D: -x.D}
				}
			}
			var args [1]coretypes.Object
			args[0] = Eval(expr.args[0], env)
			return callable.Fn(args[:])
		case 2:
			switch callable.Name {
			case "procGet":
				coll := Eval(expr.args[0], env)
				key := Eval(expr.args[1], env)
				switch c := coll.(type) {
				case coretypes.Gettable:
					ok, v := c.Get(key)
					if ok {
						return v
					}
					return NIL
				}
			case "procAdd":
				ax := Eval(expr.args[0], env)
				bx := Eval(expr.args[1], env)
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Int{I: a.I + b.I}
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
				ax := Eval(expr.args[0], env)
				bx := Eval(expr.args[1], env)
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Int{I: a.I - b.I}
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
				ax := Eval(expr.args[0], env)
				bx := Eval(expr.args[1], env)
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						return coretypes.Int{I: a.I * b.I}
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
				ax := Eval(expr.args[0], env)
				bx := Eval(expr.args[1], env)
				if a, ok := ax.(coretypes.Int); ok {
					if b, ok := bx.(coretypes.Int); ok {
						if b.I == 0 {
							coretypes.PanicOnZero(coretypes.INT_OPS, b)
						}
						return coretypes.Int{I: a.I % b.I}
					}
				}
			case "procDivide":
				ax := Eval(expr.args[0], env)
				bx := Eval(expr.args[1], env)
				switch a := ax.(type) {
				case coretypes.Int:
					switch b := bx.(type) {
					case coretypes.Int:
						if b.I == 0 {
							coretypes.PanicOnZero(coretypes.INT_OPS, b)
						}
						return coretypes.Double{D: float64(a.I) / float64(b.I)}
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
				ax := Eval(expr.args[0], env)
				bx := Eval(expr.args[1], env)
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
				ax := Eval(expr.args[0], env)
				bx := Eval(expr.args[1], env)
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
				ax := Eval(expr.args[0], env)
				bx := Eval(expr.args[1], env)
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
				ax := Eval(expr.args[0], env)
				bx := Eval(expr.args[1], env)
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
				ax := Eval(expr.args[0], env)
				bx := Eval(expr.args[1], env)
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
			args[0] = Eval(expr.args[0], env)
			args[1] = Eval(expr.args[1], env)
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
	if ToBool(Eval(expr.cond, env)) {
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
				defer func() {
					if r := recover(); r != nil {
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
			defer func() {
				if r := recover(); r != nil {
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
			case *EvalError:
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

func PanicOnErr(err error) {
	if err != nil {
		panic(RT.NewError(err.Error()))
	}
}

// ---- tail_call.go ----
// tco.go — generic tail-call optimization via trampoline.
//
// When a function body's tail expression is a call to the same function,
// it returns a TailCall marker instead of actually recursing. Fn.Call
// detects this and loops with the new args, eliminating stack growth.
//
// This benefits any self-recursive function where the self-call is in
// tail position (e.g. accumulators, state machines, list traversals).
// It does NOT help tree-recursive patterns like naive fib where the
// recursive calls are not in tail position.

// TailCall is a marker returned by evalTailCall when a self-call in
// tail position is detected. It is NOT a valid Joker coretypes.Object — it is
// only used internally between evalLoop and Fn.Call.
type TailCall struct {
	fn   *Fn
	args []coretypes.Object
}

// coretypes.Object interface stubs — TailCall should never escape to user code.
func (tc *TailCall) ToString(escape bool) string                     { return "#<tail-call>" }
func (tc *TailCall) Equals(other interface{}) bool                   { return false }
func (tc *TailCall) GetInfo() *coretypes.ObjectInfo                  { return nil }
func (tc *TailCall) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return tc }
func (tc *TailCall) GetType() *coretypes.Type                        { return TYPE.Fn }
func (tc *TailCall) Hash() uint32                                    { return 0 }

// activeFn tracks the currently executing Fn for TCO detection.
// This is stored on the Runtime (single-threaded evaluator).
var activeFn *Fn

// evalBodyTCO evaluates a body and, for the last expression, checks
// if it's a self-call in tail position. If so, returns a *TailCall
// instead of actually calling.
func evalBodyTCO(body []Expr, env *LocalEnv, self *Fn) coretypes.Object {
	if len(body) == 0 {
		return NIL
	}
	// Evaluate all but the last expression normally
	for i := 0; i < len(body)-1; i++ {
		Eval(body[i], env)
	}
	// For the last expression, check for tail self-call
	last := body[len(body)-1]
	return evalTailExpr(last, env, self)
}

// evalLoopTCO is like evalLoop but with TCO awareness.
func evalLoopTCO(body []Expr, env *LocalEnv, self *Fn) coretypes.Object {
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

// evalTailExpr evaluates an expression in tail position with self-call detection.
func evalTailExpr(expr Expr, env *LocalEnv, self *Fn) coretypes.Object {
	switch e := expr.(type) {
	case *IfExpr:
		if ToBool(Eval(e.cond, env)) {
			return evalTailExpr(e.positive, env, self)
		}
		return evalTailExpr(e.negative, env, self)

	case *CallExpr:
		// Check if this is a self-call
		callable := Eval(e.callable, env)
		if fn, ok := callable.(*Fn); ok && fn == self {
			// This is a tail self-call — return TailCall marker
			args := evalSeq(e.args, env)
			return &TailCall{fn: fn, args: args}
		}
		// Not a self-call — evaluate normally
		switch c := callable.(type) {
		case coretypes.Callable:
			args := evalSeq(e.args, env)
			return c.Call(args)
		default:
			panic(RT.NewErrorWithPos(callable.ToString(false)+" is not a Fn", e.callable.Pos()))
		}

	case *DoExpr:
		return evalBodyTCO(e.body, env, self)

	case *LetExpr:
		childEnv := LocalEnv{bindings: make([]coretypes.Object, 0, len(e.names)), parent: env}
		if env != nil {
			childEnv.frame = env.frame + 1
		}
		for _, bindingExpr := range e.values {
			childEnv.bindings = append(childEnv.bindings, Eval(bindingExpr, &childEnv))
		}
		return evalBodyTCO(e.body, &childEnv, self)

	default:
		// Not a recognized tail form — evaluate normally
		return Eval(expr, env)
	}
}

// ---- tco_rewrite.go ----
// tco_rewrite.go — parse-time rewriting of tail-self-calls to recur.
//
// When a named fn (from letfn or named fn) has a tail-position call
// to itself, rewrite the fn body as a loop/recur. This eliminates
// the runtime trampoline overhead entirely.
//
// Before: (fn self [x] (if (= x 0) 1 (self (dec x))))
// After:  (fn self [x] (loop [x x] (if (= x 0) 1 (recur (dec x)))))
//
// The rewrite wraps the body in a LoopExpr with the fn args as bindings,
// and replaces tail self-calls with RecurExpr.

// rewriteTailCallsToRecur checks if a FnExpr with a self-binding
// has tail-position self-calls, and if so, rewrites them to recur.
func rewriteTailCallsToRecur(fnExpr *FnExpr, selfBinding *Binding) {
	if selfBinding == nil || fnExpr.self.NameKey() == nil {
		return
	}
	for i := range fnExpr.arities {
		arity := &fnExpr.arities[i]
		if len(arity.body) == 0 {
			continue
		}
		lastExpr := arity.body[len(arity.body)-1]
		has := hasTailSelfCall(lastExpr, selfBinding)
		if has {
			newBody := make([]Expr, len(arity.body))
			copy(newBody, arity.body)
			newBody[len(newBody)-1] = rewriteTailExpr(newBody[len(newBody)-1], selfBinding)
			arity.body = newBody
			fnExpr.tailRewritten = true
		}
	}
}

// hasTailSelfCall checks if an expression in tail position calls selfBinding.
func hasTailSelfCall(expr Expr, self *Binding) bool {
	switch e := expr.(type) {
	case *CallExpr:
		if bind, ok := e.callable.(*BindingExpr); ok {
			// The self-call may be through the letfn binding or the fn's own self binding.
			// Match by name since they may have different frame/index.
			if bind.binding.name.NameKey() != nil && self.name.NameKey() != nil &&
				*bind.binding.name.NameKey() == *self.name.NameKey() {
				return true
			}
		}
		return false
	case *IfExpr:
		return hasTailSelfCall(e.positive, self) || hasTailSelfCall(e.negative, self)
	case *DoExpr:
		if len(e.body) == 0 {
			return false
		}
		return hasTailSelfCall(e.body[len(e.body)-1], self)
	case *LetExpr:
		if len(e.body) == 0 {
			return false
		}
		return hasTailSelfCall(e.body[len(e.body)-1], self)
	default:
		return false
	}
}

// rewriteTailExpr replaces tail-position self-calls with RecurExpr.
func rewriteTailExpr(expr Expr, self *Binding) Expr {
	switch e := expr.(type) {
	case *CallExpr:
		if bind, ok := e.callable.(*BindingExpr); ok {
			if bind.binding.name.NameKey() != nil && self.name.NameKey() != nil &&
				*bind.binding.name.NameKey() == *self.name.NameKey() {
				return &RecurExpr{
					Position: e.Position,
					args:     e.args,
				}
			}
		}
		return e
	case *IfExpr:
		return &IfExpr{
			Position: e.Position,
			cond:     e.cond,
			positive: rewriteTailExpr(e.positive, self),
			negative: rewriteTailExpr(e.negative, self),
		}
	case *DoExpr:
		if len(e.body) == 0 {
			return e
		}
		newBody := make([]Expr, len(e.body))
		copy(newBody, e.body)
		newBody[len(newBody)-1] = rewriteTailExpr(newBody[len(newBody)-1], self)
		return &DoExpr{
			Position: e.Position,
			body:     newBody,
		}
	case *LetExpr:
		if len(e.body) == 0 {
			return e
		}
		newBody := make([]Expr, len(e.body))
		copy(newBody, e.body)
		newBody[len(newBody)-1] = rewriteTailExpr(newBody[len(newBody)-1], self)
		return &LetExpr{
			Position: e.Position,
			names:    e.names,
			values:   e.values,
			body:     newBody,
		}
	default:
		return expr
	}
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

// ---- reduce_fast.go ----

// ---- reduce_fast.go ----
// reduce_fast.go — coretypes.Seq-walking reduce fallback + IntRange creation at reduce time.

func seqReduceInit(s coretypes.Seq, f coretypes.Callable, init coretypes.Object) coretypes.Object {
	acc := init
	for !s.IsEmpty() {
		acc = call2(f, acc, s.First())
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
		s = s.Rest()
	}
	return acc
}

func seqReduce(s coretypes.Seq, f coretypes.Callable) coretypes.Object {
	if s.IsEmpty() {
		return f.Call(nil)
	}
	acc := s.First()
	s = s.Rest()
	for !s.IsEmpty() {
		acc = call2(f, acc, s.First())
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
		s = s.Rest()
	}
	return acc
}

// ---- map_filter_fast.go ----
// map_filter_fast.go — AST-level fused reducible pipelines.
//
// Recognizes reduce over a map/filter/take pipeline rooted at range and executes
// it in one loop, avoiding lazy seq wrapper churn. This replaces the earlier
// square/even/take/range-only special case with a small general pipeline compiler.

type reducibleStepKind byte

const (
	reducibleMap reducibleStepKind = iota
	reducibleFilter
	reducibleTake
)

type reducibleIntrinsic byte

const (
	reducibleGeneric reducibleIntrinsic = iota
	reducibleSquareInt
	reducibleEvenInt
)

type reducibleStep struct {
	kind      reducibleStepKind
	intrinsic reducibleIntrinsic
	fn        coretypes.Callable
	takeLimit int
}

type reducibleRangePipeline struct {
	start int
	end   int
	step  int
	steps []reducibleStep
}

func evalReducePipelineFast(expr *CallExpr, env *LocalEnv) (coretypes.Object, bool) {
	if !callableName(expr.callable, "reduce") || (len(expr.args) != 2 && len(expr.args) != 3) {
		return nil, false
	}

	reducerObj := Eval(expr.args[0], env)
	reducer, ok := reducerObj.(coretypes.Callable)
	if !ok {
		return nil, false
	}

	var init coretypes.Object
	var collExpr Expr
	if len(expr.args) == 3 {
		init = Eval(expr.args[1], env)
		collExpr = expr.args[2]
	} else {
		collExpr = expr.args[1]
	}

	pipeline, ok := compileReducibleRangePipeline(collExpr, env)
	if !ok || pipeline.step == 0 || len(pipeline.steps) == 0 {
		return nil, false
	}

	if len(expr.args) == 2 {
		return reducePipelineNoInit(reducer, pipeline)
	}
	return reducePipelineInit(reducer, init, pipeline), true
}

func compileReducibleRangePipeline(expr Expr, env *LocalEnv) (reducibleRangePipeline, bool) {
	call, ok := expr.(*CallExpr)
	if !ok {
		return reducibleRangePipeline{}, false
	}

	if callableName(call.callable, "range") {
		start, end, step, ok := evalRangeArgs(call.args, env)
		if !ok || step == 0 {
			return reducibleRangePipeline{}, false
		}
		return reducibleRangePipeline{start: start, end: end, step: step}, true
	}

	if callableName(call.callable, "map") && len(call.args) == 2 {
		inner, ok := compileReducibleRangePipeline(call.args[1], env)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		if isSquareMapperExpr(call.args[0]) {
			inner.steps = append(inner.steps, reducibleStep{kind: reducibleMap, intrinsic: reducibleSquareInt})
			return inner, true
		}
		fnObj := Eval(call.args[0], env)
		fn, ok := fnObj.(coretypes.Callable)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		inner.steps = append(inner.steps, reducibleStep{kind: reducibleMap, fn: fn})
		return inner, true
	}

	if callableName(call.callable, "filter") && len(call.args) == 2 {
		inner, ok := compileReducibleRangePipeline(call.args[1], env)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		if callableName(call.args[0], "even?") {
			inner.steps = append(inner.steps, reducibleStep{kind: reducibleFilter, intrinsic: reducibleEvenInt})
			return inner, true
		}
		fnObj := Eval(call.args[0], env)
		fn, ok := fnObj.(coretypes.Callable)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		inner.steps = append(inner.steps, reducibleStep{kind: reducibleFilter, fn: fn})
		return inner, true
	}

	if callableName(call.callable, "take") && len(call.args) == 2 {
		inner, ok := compileReducibleRangePipeline(call.args[1], env)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		nObj := Eval(call.args[0], env)
		n, ok := nObj.(coretypes.Int)
		if !ok {
			return reducibleRangePipeline{}, false
		}
		inner.steps = append(inner.steps, reducibleStep{kind: reducibleTake, takeLimit: n.I})
		return inner, true
	}

	return reducibleRangePipeline{}, false
}

func reducePipelineNoInit(reducer coretypes.Callable, p reducibleRangePipeline) (coretypes.Object, bool) {
	seen := false
	var acc coretypes.Object
	_, stopped := walkReducibleRangePipeline(p, func(v coretypes.Object) bool {
		if !seen {
			acc = v
			seen = true
			return false
		}
		acc = reduceStepFast(reducer, acc, v)
		return IsReduced(acc)
	})
	if !seen {
		return call0(reducer), true
	}
	if stopped && IsReduced(acc) {
		return DerefReduced(acc), true
	}
	return acc, true
}

func reducePipelineInit(reducer coretypes.Callable, init coretypes.Object, p reducibleRangePipeline) coretypes.Object {
	acc := init
	reducerName := hotReducerName(reducer)
	_, stopped := walkReducibleRangePipeline(p, func(v coretypes.Object) bool {
		acc = reduceStepFastByName(reducer, reducerName, acc, v)
		return IsReduced(acc)
	})
	if stopped && IsReduced(acc) {
		return DerefReduced(acc)
	}
	return acc
}

func walkReducibleRangePipeline(p reducibleRangePipeline, emit func(coretypes.Object) bool) (emitted int, stopped bool) {
	takeRemaining := make([]int, len(p.steps))
	for i, step := range p.steps {
		if step.kind == reducibleTake {
			takeRemaining[i] = step.takeLimit
		}
	}

	for i := p.start; (p.step > 0 && i < p.end) || (p.step < 0 && i > p.end); i += p.step {
		v := coretypes.Object(coretypes.Int{I: i})
		alive := true
		stopAfterCurrent := false

		for si, step := range p.steps {
			if !alive {
				break
			}
			switch step.kind {
			case reducibleMap:
				if step.intrinsic == reducibleSquareInt {
					if iv, ok := v.(coretypes.Int); ok {
						v = coretypes.Int{I: iv.I * iv.I}
					} else {
						v = call1(step.fn, v)
					}
				} else {
					v = call1(step.fn, v)
				}
			case reducibleFilter:
				if step.intrinsic == reducibleEvenInt {
					if iv, ok := v.(coretypes.Int); ok {
						alive = iv.I%2 == 0
					} else {
						alive = false
					}
				} else if !ToBool(call1(step.fn, v)) {
					alive = false
				}
			case reducibleTake:
				if takeRemaining[si] <= 0 {
					return emitted, true
				}
				takeRemaining[si]--
				if takeRemaining[si] == 0 {
					stopAfterCurrent = true
				}
			}
		}

		if alive {
			emitted++
			if emit(v) {
				return emitted, true
			}
		}
		if stopAfterCurrent {
			return emitted, true
		}
	}
	return emitted, false
}

func isSquareMapperExpr(expr Expr) bool {
	if fn, ok := expr.(*FnExpr); ok {
		return isSquareFnExpr(fn)
	}
	if le, ok := expr.(*LetExpr); ok {
		return isSquareFnExpr(extractFnExpr(le.body))
	}
	return false
}

func extractFnExpr(body []Expr) *FnExpr {
	if len(body) == 0 {
		return nil
	}
	switch e := body[len(body)-1].(type) {
	case *FnExpr:
		return e
	case *LetExpr:
		return extractFnExpr(e.body)
	case *DoExpr:
		return extractFnExpr(e.body)
	}
	return nil
}

func isSquareFnExpr(fn *FnExpr) bool {
	if fn == nil || len(fn.arities) != 1 || fn.variadic != nil {
		return false
	}
	arity := fn.arities[0]
	if len(arity.args) != 1 || len(arity.body) != 1 {
		return false
	}
	pf := guessFnParamFrame(arity.body, 1)
	if pf < 0 {
		pf = 1
	}
	call, ok := arity.body[0].(*CallExpr)
	if !ok || len(call.args) != 2 {
		return false
	}
	vref, ok := call.callable.(*VarRefExpr)
	if !ok || coreVarToProcName(vref.vr) != "procMultiply" {
		return false
	}
	lhs, lok := call.args[0].(*BindingExpr)
	rhs, rok := call.args[1].(*BindingExpr)
	return lok && rok && lhs.binding.frame == pf && rhs.binding.frame == pf && lhs.binding.index == 0 && rhs.binding.index == 0
}

func callableName(expr Expr, name string) bool {
	vref, ok := expr.(*VarRefExpr)
	return ok && vref.vr.name.ToString(false) == name
}

func evalRangeArgs(args []Expr, env *LocalEnv) (start, end, step int, ok bool) {
	switch len(args) {
	case 1:
		endObj := Eval(args[0], env)
		endInt, yes := endObj.(coretypes.Int)
		return 0, endInt.I, 1, yes
	case 2:
		startObj := Eval(args[0], env)
		endObj := Eval(args[1], env)
		startInt, sok := startObj.(coretypes.Int)
		endInt, eok := endObj.(coretypes.Int)
		return startInt.I, endInt.I, 1, sok && eok
	case 3:
		startObj := Eval(args[0], env)
		endObj := Eval(args[1], env)
		stepObj := Eval(args[2], env)
		startInt, sok := startObj.(coretypes.Int)
		endInt, eok := endObj.(coretypes.Int)
		stepInt, tok := stepObj.(coretypes.Int)
		return startInt.I, endInt.I, stepInt.I, sok && eok && tok
	}
	return 0, 0, 0, false
}

// ---- seq_ops_fast.go ----
// seq_ops_fast.go — Fast map/filter/take seq wrappers for reducible pipelines.

type FilteringSeq struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	seq  coretypes.Seq
	pred coretypes.Callable
}

type TakeSeq struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	seq coretypes.Seq
	n   int
}

func (s *FilteringSeq) ToString(escape bool) string {
	return corecollections.SeqToString(s, func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (s *FilteringSeq) Equals(other interface{}) bool { return coretypes.IsSeqEqual(s, other) }
func (s *FilteringSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *s
	res.Info = info
	return &res
}
func (s *FilteringSeq) GetType() *coretypes.Type { return TYPE.LazySeq }
func (s *FilteringSeq) Hash() uint32             { return corecollections.HashOrdered(s) }
func (s *FilteringSeq) WithMeta(m coretypes.Map) coretypes.Object {
	res := *s
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}
func (s *FilteringSeq) Seq() coretypes.Seq      { return s }
func (s *FilteringSeq) SequentialMarker()       {}
func (s *FilteringSeq) IsEmpty() bool           { return s.nextSeq().IsEmpty() }
func (s *FilteringSeq) First() coretypes.Object { return s.nextSeq().First() }
func (s *FilteringSeq) Rest() coretypes.Seq {
	ns := s.nextSeq()
	if ns.IsEmpty() {
		return corecollections.EmptyList
	}
	return &FilteringSeq{seq: ns.Rest(), pred: s.pred}
}
func (s *FilteringSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &corecollections.ConsSeq{FirstValue: obj, RestValue: s}
}

func (s *FilteringSeq) nextSeq() coretypes.Seq {
	cur := s.seq
	for !cur.IsEmpty() {
		if ToBool(call1(s.pred, cur.First())) {
			return cur
		}
		cur = cur.Rest()
	}
	return corecollections.EmptyList
}

func (s *FilteringSeq) Reduce(f coretypes.Callable) coretypes.Object {
	cur := s.seq
	for !cur.IsEmpty() {
		v := cur.First()
		if ToBool(call1(s.pred, v)) {
			acc := v
			cur = cur.Rest()
			for !cur.IsEmpty() {
				v = cur.First()
				if ToBool(call1(s.pred, v)) {
					acc = call2(f, acc, v)
					if IsReduced(acc) {
						return DerefReduced(acc)
					}
				}
				cur = cur.Rest()
			}
			return acc
		}
		cur = cur.Rest()
	}
	return call0(f)
}

func (s *FilteringSeq) ReduceInit(f coretypes.Callable, init coretypes.Object) coretypes.Object {
	acc := init
	cur := s.seq
	for !cur.IsEmpty() {
		v := cur.First()
		if ToBool(call1(s.pred, v)) {
			acc = call2(f, acc, v)
			if IsReduced(acc) {
				return DerefReduced(acc)
			}
		}
		cur = cur.Rest()
	}
	return acc
}

func (s *TakeSeq) ToString(escape bool) string {
	return corecollections.SeqToString(s, func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (s *TakeSeq) Equals(other interface{}) bool { return coretypes.IsSeqEqual(s, other) }
func (s *TakeSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *s
	res.Info = info
	return &res
}
func (s *TakeSeq) GetType() *coretypes.Type { return TYPE.LazySeq }
func (s *TakeSeq) Hash() uint32             { return corecollections.HashOrdered(s) }
func (s *TakeSeq) WithMeta(m coretypes.Map) coretypes.Object {
	res := *s
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}
func (s *TakeSeq) Seq() coretypes.Seq      { return s }
func (s *TakeSeq) SequentialMarker()       {}
func (s *TakeSeq) IsEmpty() bool           { return s.n <= 0 || s.seq.IsEmpty() }
func (s *TakeSeq) First() coretypes.Object { return s.seq.First() }
func (s *TakeSeq) Rest() coretypes.Seq {
	if s.n <= 1 {
		return corecollections.EmptyList
	}
	return &TakeSeq{seq: s.seq.Rest(), n: s.n - 1}
}
func (s *TakeSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &corecollections.ConsSeq{FirstValue: obj, RestValue: s}
}

func (s *TakeSeq) Reduce(f coretypes.Callable) coretypes.Object {
	if result, ok := s.reduceFused(f); ok {
		return result
	}
	if s.IsEmpty() {
		return call0(f)
	}
	acc := s.seq.First()
	cur := s.seq.Rest()
	for i := 1; i < s.n && !cur.IsEmpty(); i++ {
		acc = call2(f, acc, cur.First())
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
		cur = cur.Rest()
	}
	return acc
}

func (s *TakeSeq) reduceFused(f coretypes.Callable) (coretypes.Object, bool) {
	return nil, false
}

func (s *TakeSeq) ReduceInit(f coretypes.Callable, init coretypes.Object) coretypes.Object {
	acc := init
	cur := s.seq
	for i := 0; i < s.n && !cur.IsEmpty(); i++ {
		acc = call2(f, acc, cur.First())
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
		cur = cur.Rest()
	}
	return acc
}

func chunkedMapSeq(f coretypes.Callable, src coretypes.Seq) coretypes.Seq {
	if src == nil || src.IsEmpty() {
		return corecollections.EmptyList
	}
	buf := make([]coretypes.Object, 0, 32)
	cur := src
	for len(buf) < 32 && !cur.IsEmpty() {
		buf = append(buf, call1(f, cur.First()))
		cur = cur.Rest()
	}
	chunk := &corecollections.ArrayChunk{Arr: buf, Off: 0, End: len(buf)}
	var rest coretypes.Seq
	if !cur.IsEmpty() {
		restCur := cur
		rest = &corecollections.LazySeq{Fn: Proc{Name: "procChunkedMapRest", Fn: func(args []coretypes.Object) coretypes.Object {
			return chunkedMapSeq(f, restCur)
		}}}
	}
	return &corecollections.ChunkedCons{Chunk: chunk, RestSeq: rest, Idx: 0}
}

func chunkedFilterSeq(pred coretypes.Callable, src coretypes.Seq) coretypes.Seq {
	cur := src
	for {
		if cur == nil || cur.IsEmpty() {
			return corecollections.EmptyList
		}
		buf := make([]coretypes.Object, 0, 32)
		for len(buf) < 32 && !cur.IsEmpty() {
			v := cur.First()
			if ToBool(call1(pred, v)) {
				buf = append(buf, v)
			}
			cur = cur.Rest()
		}
		if len(buf) > 0 {
			chunk := &corecollections.ArrayChunk{Arr: buf, Off: 0, End: len(buf)}
			var rest coretypes.Seq
			if !cur.IsEmpty() {
				restCur := cur
				rest = &corecollections.LazySeq{Fn: Proc{Name: "procChunkedFilterRest", Fn: func(args []coretypes.Object) coretypes.Object {
					return chunkedFilterSeq(pred, restCur)
				}}}
			}
			return &corecollections.ChunkedCons{Chunk: chunk, RestSeq: rest, Idx: 0}
		}
	}
}

func maybeOverrideSeqOps() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}
	mapVr, filterVr, takeVr := ns.Resolve("map"), ns.Resolve("filter"), ns.Resolve("take")
	if mapVr == nil || filterVr == nil || takeVr == nil {
		return
	}
	mapOrig, mapOK := mapVr.Value.(coretypes.Callable)
	filterOrig, filterOK := filterVr.Value.(coretypes.Callable)
	takeOrig, takeOK := takeVr.Value.(coretypes.Callable)
	if !mapOK || !filterOK || !takeOK {
		return
	}
	if p, ok := mapVr.Value.(Proc); ok && p.Name == "procMapSeqFast" {
		return
	}

	mapVr.Value = Proc{Name: "procMapSeqFast", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			return makeMapTransducer(coretypes.EnsureArgIsCallable(args, 0))
		}
		if len(args) == 2 {
			f := coretypes.EnsureArgIsCallable(args, 0)
			s := coretypes.EnsureObjectIsSeqable(args[1], "map requires seqable").Seq()
			if _, ok := s.(*corecollections.ChunkedCons); ok {
				return chunkedMapSeq(f, s)
			}
			return &corecollections.MappingSeq{SeqValue: s, Fn: func(o coretypes.Object) coretypes.Object { return call1(f, o) }}
		}
		return mapOrig.Call(args)
	}}
	filterVr.Value = Proc{Name: "procFilterSeqFast", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			return makeFilterTransducer(coretypes.EnsureArgIsCallable(args, 0))
		}
		if len(args) == 2 {
			pred := coretypes.EnsureArgIsCallable(args, 0)
			s := coretypes.EnsureArgIsSeqable(args, 1).Seq()
			if _, ok := s.(*corecollections.ChunkedCons); ok {
				return chunkedFilterSeq(pred, s)
			}
			return &FilteringSeq{seq: s, pred: pred}
		}
		return filterOrig.Call(args)
	}}
	takeVr.Value = Proc{Name: "procTakeSeqFast", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			return makeTakeTransducer(coretypes.EnsureObjectIsNumber(args[0], "").Int().I)
		}
		if len(args) == 2 {
			return &TakeSeq{seq: coretypes.EnsureObjectIsSeqable(args[1], "take requires seqable").Seq(), n: coretypes.EnsureObjectIsNumber(args[0], "").Int().I}
		}
		return takeOrig.Call(args)
	}}
}

// ---- frequencies_fast.go ----
// frequencies_fast.go — native fast path for core/frequencies.

func init() {
	vr := GLOBAL_ENV.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "frequencies"))
	vr.Value = Proc{Name: "procFrequencies", Fn: procFrequencies}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "frequencies"), vr)

	sw := GLOBAL_ENV.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "split-whitespace__"))
	sw.Value = Proc{Name: "procSplitWhitespace", Fn: procSplitWhitespace}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "split-whitespace"), sw)
}

var procSplitWhitespace ProcFn = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	return splitWhitespaceVector(coretypes.EnsureArgIsString(args, 0).S)
}

func splitWhitespaceVector(s string) *corecollections.ArrayVector {
	res := corecollections.EmptyArrayVector()
	for _, token := range corestr.SplitWhitespace(s) {
		res.Append(coretypes.String{S: token})
	}
	return res
}

var procFrequencies ProcFn = func(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	seq := coretypes.EnsureObjectIsSeqable(args[0], "frequencies requires a seqable collection").Seq()
	if seq.IsEmpty() {
		return corecollections.EmptyArrayMap()
	}

	// Specialize the common text-token case: String keys and integer counts.
	// Avoids persistent map churn and repeated coretypes.Object hash calculation in the
	// hot loop, then emits a normal persistent map at the boundary.
	stringCounts := make(map[string]int)
	var tm *coretypes.TransientMap
	stringOnly := true
	for !seq.IsEmpty() {
		obj := seq.First()
		if stringOnly {
			if s, ok := obj.(coretypes.String); ok {
				stringCounts[s.S]++
				seq = seq.Rest()
				continue
			}
			stringOnly = false
			tm = coretypes.MapToTransient(nil)
			for k, v := range stringCounts {
				tm.AssocInPlace(coretypes.String{S: k}, coretypes.Int{I: v})
			}
			stringCounts = nil
		}
		_, old := tm.Get(obj)
		cnt := 0
		if i, ok := old.(coretypes.Int); ok {
			cnt = i.I
		}
		tm.AssocInPlace(obj, coretypes.Int{I: cnt + 1})
		seq = seq.Rest()
	}
	if stringOnly {
		if len(stringCounts) <= int(corecollections.HASHMAP_THRESHOLD/2) {
			res := corecollections.EmptyArrayMap()
			for k, v := range stringCounts {
				res.Add(coretypes.String{S: k}, coretypes.Int{I: v})
			}
			return res
		}
		res := corecollections.EmptyHashMap
		for k, v := range stringCounts {
			res = res.Assoc(coretypes.String{S: k}, coretypes.Int{I: v}).(*corecollections.HashMap)
		}
		return res
	}
	return tm.ToPersistent()
}

// ---- range_fast.go ----
// range_fast.go — Efficient Range type that implements coretypes.Reduce for fast numeric reduce.

var hotReducerFnCache sync.Map // *Fn -> reducer proc name string

// IntRange represents a range of integers [start, end) with step.
type IntRange struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	start, end, step int
}

func NewIntRange(start, end, step int) *IntRange {
	return &IntRange{start: start, end: end, step: step}
}

func (r *IntRange) ToString(escape bool) string {
	return corecollections.SeqToString(r.Seq(), func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (r *IntRange) Equals(other interface{}) bool { return coretypes.IsSeqEqual(r.Seq(), other) }
func (r *IntRange) WithInfo(i *coretypes.ObjectInfo) coretypes.Object {
	res := *r
	res.Info = i
	return &res
}
func (r *IntRange) GetType() *coretypes.Type { return TYPE.LazySeq }
func (r *IntRange) Hash() uint32             { return corecollections.HashOrdered(r.Seq()) }
func (r *IntRange) WithMeta(m coretypes.Map) coretypes.Object {
	res := *r
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}
func (r *IntRange) SequentialMarker() {}

func (r *IntRange) Seq() coretypes.Seq {
	if r.step > 0 && r.start >= r.end {
		return corecollections.EmptyList
	}
	if r.step < 0 && r.start <= r.end {
		return corecollections.EmptyList
	}
	return r.chunkedSeqFrom(r.start)
}

func (r *IntRange) chunkedSeqFrom(cur int) coretypes.Seq {
	if !r.contains(cur) {
		return corecollections.EmptyList
	}
	buf := make([]coretypes.Object, 0, 32)
	v := cur
	for len(buf) < 32 && r.contains(v) {
		buf = append(buf, coretypes.Int{I: v})
		v += r.step
	}
	chunk := &corecollections.ArrayChunk{Arr: buf, Off: 0, End: len(buf)}
	var rest coretypes.Seq
	if r.contains(v) {
		rest = &corecollections.LazySeq{Fn: Proc{Name: "procIntRangeChunkRest", Fn: func(args []coretypes.Object) coretypes.Object {
			return r.chunkedSeqFrom(v)
		}}}
	}
	return &corecollections.ChunkedCons{Chunk: chunk, RestSeq: rest, Idx: 0}
}

func (r *IntRange) Count() int {
	if r.step > 0 {
		n := (r.end - r.start + r.step - 1) / r.step
		if n < 0 {
			return 0
		}
		return n
	}
	if r.step < 0 {
		n := (r.start - r.end - r.step - 1) / (-r.step)
		if n < 0 {
			return 0
		}
		return n
	}
	if r.step == 0 {
		panic(coretypes.RuntimeError("range: step must not be 0"))
	}
	return 0
}

func (r *IntRange) Reduce(f coretypes.Callable) coretypes.Object {
	if r.isEmpty() {
		return f.Call(nil)
	}
	if result, ok := r.reduceFast(f); ok {
		return result
	}
	acc := coretypes.Object(coretypes.Int{I: r.start})
	for i := r.start + r.step; r.contains(i); i += r.step {
		acc = call2(f, acc, coretypes.Int{I: i})
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
	}
	return acc
}

func (r *IntRange) ReduceInit(f coretypes.Callable, init coretypes.Object) coretypes.Object {
	if result, ok := r.reduceInitFast(f, init); ok {
		return result
	}
	acc := init
	for i := r.start; r.contains(i); i += r.step {
		acc = call2(f, acc, coretypes.Int{I: i})
		if IsReduced(acc) {
			return DerefReduced(acc)
		}
	}
	return acc
}

func (r *IntRange) isEmpty() bool {
	return (r.step > 0 && r.start >= r.end) || (r.step < 0 && r.start <= r.end)
}

func (r *IntRange) contains(i int) bool {
	return (r.step > 0 && i < r.end) || (r.step < 0 && i > r.end)
}

func (r *IntRange) reduceFast(f coretypes.Callable) (coretypes.Object, bool) {
	name := hotReducerName(f)
	switch name {
	case "procAdd", "procunchecked-add", "procunchecked-add-int":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			acc += i
		}
		return coretypes.Int{I: acc}, true
	case "procMultiply", "procunchecked-multiply", "procunchecked-multiply-int":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			acc *= i
		}
		return coretypes.Int{I: acc}, true
	case "procMax":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			if i > acc {
				acc = i
			}
		}
		return coretypes.Int{I: acc}, true
	case "procMin":
		acc := r.start
		for i := r.start + r.step; r.contains(i); i += r.step {
			if i < acc {
				acc = i
			}
		}
		return coretypes.Int{I: acc}, true
	}
	return nil, false
}

func (r *IntRange) reduceInitFast(f coretypes.Callable, init coretypes.Object) (coretypes.Object, bool) {
	if result, ok := r.reduceMapAssocFast(f, init); ok {
		return result, true
	}
	name := hotReducerName(f)
	switch acc := init.(type) {
	case coretypes.Int:
		v := acc.I
		switch name {
		case "procAdd", "procunchecked-add", "procunchecked-add-int":
			for i := r.start; r.contains(i); i += r.step {
				v += i
			}
			return coretypes.Int{I: v}, true
		case "procMultiply", "procunchecked-multiply", "procunchecked-multiply-int":
			for i := r.start; r.contains(i); i += r.step {
				v *= i
			}
			return coretypes.Int{I: v}, true
		case "procMax":
			for i := r.start; r.contains(i); i += r.step {
				if i > v {
					v = i
				}
			}
			return coretypes.Int{I: v}, true
		case "procMin":
			for i := r.start; r.contains(i); i += r.step {
				if i < v {
					v = i
				}
			}
			return coretypes.Int{I: v}, true
		}
	case coretypes.Double:
		v := acc.D
		switch name {
		case "procAdd":
			for i := r.start; r.contains(i); i += r.step {
				v += float64(i)
			}
			return coretypes.Double{D: v}, true
		case "procMultiply":
			for i := r.start; r.contains(i); i += r.step {
				v *= float64(i)
			}
			return coretypes.Double{D: v}, true
		case "procMax":
			for i := r.start; r.contains(i); i += r.step {
				fi := float64(i)
				if fi > v {
					v = fi
				}
			}
			return coretypes.Double{D: v}, true
		case "procMin":
			for i := r.start; r.contains(i); i += r.step {
				fi := float64(i)
				if fi < v {
					v = fi
				}
			}
			return coretypes.Double{D: v}, true
		}
	}
	return nil, false
}

func (r *IntRange) reduceMapAssocFast(f coretypes.Callable, init coretypes.Object) (coretypes.Object, bool) {
	fn, ok := f.(*Fn)
	if !ok || fn == nil || fn.fnExpr == nil || len(fn.fnExpr.arities) != 1 || fn.fnExpr.variadic != nil {
		return nil, false
	}
	m, ok := init.(coretypes.Map)
	if !ok {
		return nil, false
	}
	arity := fn.fnExpr.arities[0]
	if len(arity.args) != 2 || len(arity.body) != 1 {
		return nil, false
	}
	pf := guessFnParamFrame(arity.body, 2)
	if pf < 0 {
		pf = 1
	}
	call, ok := arity.body[0].(*CallExpr)
	if !ok || len(call.args) != 3 {
		return nil, false
	}
	vref, ok := call.callable.(*VarRefExpr)
	if !ok || coreVarToProcName(vref.vr) != "procAssoc" {
		return nil, false
	}
	base, ok := call.args[0].(*BindingExpr)
	if !ok || base.binding.frame != pf || base.binding.index != 0 {
		return nil, false
	}
	keyFn := compileIntExpr2(call.args[1], nil, pf, &nativeRecursiveEntry{arity: 2})
	valFn := compileIntExpr2(call.args[2], nil, pf, &nativeRecursiveEntry{arity: 2})
	if keyFn == nil || valFn == nil {
		return nil, false
	}
	tm := coretypes.MapToTransient(m)
	for i := r.start; r.contains(i); i += r.step {
		tm.AssocInPlace(coretypes.Int{I: keyFn(0, i)}, coretypes.Int{I: valFn(0, i)})
	}
	return tm.ToPersistent(), true
}

func hotReducerName(f coretypes.Callable) string {
	switch c := f.(type) {
	case Proc:
		return c.Name
	case *Fn:
		if c.defVar != nil {
			if proc := hotReducerSymbol(c.defVar.name.ToString(false)); proc != "" {
				return proc
			}
		}
		if cached, ok := hotReducerFnCache.Load(c); ok {
			return cached.(string)
		}
		if proc := bindHotReducerDefVar(c); proc != "" {
			hotReducerFnCache.Store(c, proc)
			return proc
		}
	case *Var:
		return hotReducerSymbol(c.name.ToString(false))
	}
	return ""
}

func findFnVarNameCallable(c coretypes.Callable) string {
	switch f := c.(type) {
	case *Fn:
		return findFnVarName(f)
	case *Var:
		return f.name.ToString(false)
	}
	return ""
}

func findFnVarName(fn *Fn) string {
	if fn != nil && fn.defVar != nil {
		return fn.defVar.name.ToString(false)
	}
	if fn == nil {
		return ""
	}
	if ns := GLOBAL_ENV.CurrentNamespace(); ns != nil {
		for _, vr := range ns.Mappings() {
			if vr.Value == fn {
				return vr.name.ToString(false)
			}
		}
	}
	if ns := GLOBAL_ENV.CoreNamespace; ns != nil {
		for _, vr := range ns.Mappings() {
			if vr.Value == fn {
				return vr.name.ToString(false)
			}
		}
	}
	return ""
}

func bindHotReducerDefVar(fn *Fn) string {
	if fn == nil {
		return ""
	}
	for _, ns := range []*Namespace{GLOBAL_ENV.CurrentNamespace(), GLOBAL_ENV.CoreNamespace} {
		if ns == nil {
			continue
		}
		for _, vr := range ns.Mappings() {
			if vr.Value == fn {
				proc := hotReducerSymbol(vr.name.ToString(false))
				if proc != "" {
					fn.defVar = vr
					return proc
				}
			}
		}
	}
	return ""
}

func hotReducerSymbol(sym string) string {
	switch sym {
	case "+":
		return "procAdd"
	case "*":
		return "procMultiply"
	case "max":
		return "procMax"
	case "min":
		return "procMin"
	case "unchecked-add", "unchecked-add-int":
		return "procunchecked-add"
	case "unchecked-multiply", "unchecked-multiply-int":
		return "procunchecked-multiply"
	case "<":
		return "procLt"
	case "<=":
		return "procLte"
	case ">":
		return "procGt"
	case ">=":
		return "procGte"
	}
	return ""
}

// intRangeSeq is the lazy seq view of an IntRange
type intRangeSeq struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	r   *IntRange
	cur int
}

func (s *intRangeSeq) ToString(escape bool) string {
	return corecollections.SeqToString(s, func(obj coretypes.Object) string { return obj.ToString(escape) })
}
func (s *intRangeSeq) Equals(other interface{}) bool { return coretypes.IsSeqEqual(s, other) }
func (s *intRangeSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *s
	res.Info = info
	return &res
}
func (s *intRangeSeq) GetType() *coretypes.Type { return TYPE.LazySeq }
func (s *intRangeSeq) Hash() uint32             { return corecollections.HashOrdered(s) }
func (s *intRangeSeq) WithMeta(m coretypes.Map) coretypes.Object {
	res := *s
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}
func (s *intRangeSeq) Seq() coretypes.Seq      { return s }
func (s *intRangeSeq) SequentialMarker()       {}
func (s *intRangeSeq) First() coretypes.Object { return coretypes.Int{I: s.cur} }
func (s *intRangeSeq) Rest() coretypes.Seq {
	next := s.cur + s.r.step
	if s.r.step > 0 && next >= s.r.end {
		return corecollections.EmptyList
	}
	if s.r.step < 0 && next <= s.r.end {
		return corecollections.EmptyList
	}
	return &intRangeSeq{r: s.r, cur: next}
}
func (s *intRangeSeq) IsEmpty() bool {
	if s.r.step == 0 {
		return false // infinite range
	}
	if s.r.step > 0 {
		return s.cur >= s.r.end
	}
	return s.cur <= s.r.end
}
func (s *intRangeSeq) Cons(obj coretypes.Object) coretypes.Seq {
	return &corecollections.ConsSeq{FirstValue: obj, RestValue: s}
}

// maybeOverrideRange installs the IntRange-backed range wrapper after core.joke is loaded.
// It may be called multiple times; it only wraps the original range once.
func maybeOverrideRange() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}
	rangeVr := ns.Resolve("range")
	if rangeVr == nil {
		return
	}
	if p, ok := rangeVr.Value.(Proc); ok && p.Name == "procRangeFast" {
		return
	}
	origRange, ok := rangeVr.Value.(coretypes.Callable)
	if !ok {
		return
	}

	rangeVr.Value = Proc{Name: "procRangeFast", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 1:
			if end, ok := args[0].(coretypes.Int); ok {
				return NewIntRange(0, end.I, 1)
			}
		case 2:
			if start, ok := args[0].(coretypes.Int); ok {
				if end, ok := args[1].(coretypes.Int); ok {
					return NewIntRange(start.I, end.I, 1)
				}
			}
		case 3:
			if start, ok := args[0].(coretypes.Int); ok {
				if end, ok := args[1].(coretypes.Int); ok {
					if step, ok := args[2].(coretypes.Int); ok && step.I != 0 {
						return NewIntRange(start.I, end.I, step.I)
					}
				}
			}
		}
		return origRange.Call(args)
	}}
}

// ---- transducer_compat.go ----

// transducer_compat.go — Transducer runtime support with proper Reduced type.
//
// Provides full Clojure transducer semantics:
// - transducer arities for map/filter/take
// - transduce (3-arity and 4-arity)
// - reduced, reduced?, ensure-reduced, unreduced
// - completing (1 and 2-arity)
// - eduction (materialized vector-backed)
// - sequence 2-arity via eduction

// xformKind describes one compiled transducer step.
type xformKind byte

const (
	xformMap xformKind = iota
	xformFilter
	xformTake
)

type xformStep struct {
	kind      xformKind
	intrinsic reducibleIntrinsic
	fn        coretypes.Callable
	n         int
}

// XForm is an internal transducer pipeline representation.
// It is also coretypes.Callable, so it remains compatible with generic transducer use.
type XForm struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	steps []xformStep
}

func (xf *XForm) ToString(escape bool) string   { return "#object[XForm]" }
func (xf *XForm) Equals(other interface{}) bool { return xf == other }
func (xf *XForm) GetType() *coretypes.Type      { return TYPE.Fn }
func (xf *XForm) Hash() uint32                  { return hashutil.Ptr(uintptr(unsafe.Pointer(xf))) }
func (xf *XForm) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *xf
	res.Info = info
	return &res
}
func (xf *XForm) WithMeta(m coretypes.Map) coretypes.Object {
	res := *xf
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}

func (xf *XForm) Call(args []coretypes.Object) coretypes.Object {
	runtimeCheckArity(args, 1, 1)
	rf := coretypes.EnsureArgIsCallable(args, 0)
	return buildXFormRF(xf.steps, rf).(coretypes.Object)
}

func buildXFormRF(steps []xformStep, rf coretypes.Callable) coretypes.Callable {
	wrapped := rf
	for i := len(steps) - 1; i >= 0; i-- {
		step := steps[i]
		switch step.kind {
		case xformMap:
			f := step.fn
			downstream := wrapped
			wrapped = Proc{Name: "procMapXfRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
				switch len(callArgs) {
				case 0:
					return call0(downstream)
				case 1:
					return downstream.Call(callArgs)
				case 2:
					return call2(downstream, callArgs[0], call1(f, callArgs[1]))
				default:
					coretypes.RuntimePanicArityMinMax(len(callArgs), 0, 2)
					return NIL
				}
			}}
		case xformFilter:
			pred := step.fn
			downstream := wrapped
			wrapped = Proc{Name: "procFilterXfRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
				switch len(callArgs) {
				case 0:
					return call0(downstream)
				case 1:
					return downstream.Call(callArgs)
				case 2:
					if ToBool(call1(pred, callArgs[1])) {
						return downstream.Call(callArgs)
					}
					return callArgs[0]
				default:
					coretypes.RuntimePanicArityMinMax(len(callArgs), 0, 2)
					return NIL
				}
			}}
		case xformTake:
			limit := step.n
			downstream := wrapped
			remaining := limit
			wrapped = Proc{Name: "procTakeXfRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
				switch len(callArgs) {
				case 0:
					return call0(downstream)
				case 1:
					return downstream.Call(callArgs)
				case 2:
					if remaining <= 0 {
						return EnsureReduced(callArgs[0])
					}
					out := downstream.Call(callArgs)
					remaining--
					if remaining <= 0 {
						return EnsureReduced(out)
					}
					return out
				default:
					coretypes.RuntimePanicArityMinMax(len(callArgs), 0, 2)
					return NIL
				}
			}}
		}
	}
	return wrapped
}

func completeReducingFn(rf coretypes.Callable, res coretypes.Object) coretypes.Object {
	completed := res
	func() {
		defer func() {
			if recover() != nil {
				completed = res
			}
		}()
		completed = call1(rf, res)
	}()
	return DerefReduced(completed)
}

func transduceInternal(xform coretypes.Callable, reducingFnObj coretypes.Object, init coretypes.Object, collObj coretypes.Object) coretypes.Object {
	if xf, ok := xform.(*XForm); ok {
		return transducePipeline(xf, reducingFnObj, init, collObj)
	}
	rfObj := call1(xform, reducingFnObj)
	rf := coretypes.EnsureObjectIsCallable(rfObj, "transduce xform must produce a reducing function, got %s")

	s := coretypes.EnsureObjectIsSeqable(collObj, "Arg of core/transduce must be coretypes.Seqable, got %s").Seq()
	res := init
	for !s.IsEmpty() {
		step := call2(rf, res, s.First())
		if IsReduced(step) {
			res = DerefReduced(step)
			return completeReducingFn(rf, res)
		}
		res = step
		s = s.Rest()
	}
	return completeReducingFn(rf, res)
}

func transducePipeline(xf *XForm, reducingFnObj coretypes.Object, init coretypes.Object, collObj coretypes.Object) coretypes.Object {
	rf := coretypes.EnsureObjectIsCallable(reducingFnObj, "transduce reducing function must be coretypes.Callable, got %s")
	if r, ok := collObj.(*IntRange); ok {
		return transducePipelineRange(xf, rf, init, r)
	}
	s := coretypes.EnsureObjectIsSeqable(collObj, "Arg of core/transduce must be coretypes.Seqable, got %s").Seq()
	res := init
	reducerName := hotReducerName(rf)
	takeRemaining := -1
	for _, step := range xf.steps {
		if step.kind == xformTake {
			takeRemaining = step.n
			break
		}
	}
	for !s.IsEmpty() {
		val := s.First()
		include := true
		stopAfter := false
		for _, step := range xf.steps {
			switch step.kind {
			case xformMap:
				val = applyXFormMapStep(step, val)
			case xformFilter:
				if !applyXFormFilterStep(step, val) {
					include = false
				}
			case xformTake:
				if takeRemaining <= 0 {
					return completeReducingFn(rf, res)
				}
				takeRemaining--
				if takeRemaining == 0 {
					stopAfter = true
				}
			}
			if !include {
				break
			}
		}
		if include {
			step := reduceStepFastByName(rf, reducerName, res, val)
			if IsReduced(step) {
				return completeReducingFn(rf, DerefReduced(step))
			}
			res = step
			if stopAfter {
				return completeReducingFn(rf, res)
			}
		}
		s = s.Rest()
	}
	return completeReducingFn(rf, res)
}

func transducePipelineRange(xf *XForm, rf coretypes.Callable, init coretypes.Object, r *IntRange) coretypes.Object {
	res := init
	reducerName := hotReducerName(rf)
	takeRemaining := -1
	for _, step := range xf.steps {
		if step.kind == xformTake {
			takeRemaining = step.n
			break
		}
	}
	for i := r.start; r.contains(i); i += r.step {
		val := coretypes.Object(coretypes.Int{I: i})
		include := true
		stopAfter := false
		for _, step := range xf.steps {
			switch step.kind {
			case xformMap:
				val = applyXFormMapStep(step, val)
			case xformFilter:
				if !applyXFormFilterStep(step, val) {
					include = false
				}
			case xformTake:
				if takeRemaining <= 0 {
					return completeReducingFn(rf, res)
				}
				takeRemaining--
				if takeRemaining == 0 {
					stopAfter = true
				}
			}
			if !include {
				break
			}
		}
		if include {
			step := reduceStepFastByName(rf, reducerName, res, val)
			if IsReduced(step) {
				return completeReducingFn(rf, DerefReduced(step))
			}
			res = step
			if stopAfter {
				return completeReducingFn(rf, res)
			}
		}
	}
	return completeReducingFn(rf, res)
}

func applyXFormMapStep(step xformStep, val coretypes.Object) coretypes.Object {
	if step.intrinsic == reducibleSquareInt {
		if iv, ok := val.(coretypes.Int); ok {
			return coretypes.Int{I: iv.I * iv.I}
		}
	}
	return call1(step.fn, val)
}

func applyXFormFilterStep(step xformStep, val coretypes.Object) bool {
	if step.intrinsic == reducibleEvenInt {
		if iv, ok := val.(coretypes.Int); ok {
			return iv.I%2 == 0
		}
		return false
	}
	return ToBool(call1(step.fn, val))
}

func reduceStepFast(rf coretypes.Callable, acc coretypes.Object, val coretypes.Object) coretypes.Object {
	return reduceStepFastByName(rf, hotReducerName(rf), acc, val)
}

func reduceStepFastByName(rf coretypes.Callable, name string, acc coretypes.Object, val coretypes.Object) coretypes.Object {
	switch name {
	case "procAdd", "procunchecked-add", "procunchecked-add-int":
		if a, ok := acc.(coretypes.Int); ok {
			if b, ok := val.(coretypes.Int); ok {
				return coretypes.Int{I: a.I + b.I}
			}
		}
	case "procMultiply", "procunchecked-multiply", "procunchecked-multiply-int":
		if a, ok := acc.(coretypes.Int); ok {
			if b, ok := val.(coretypes.Int); ok {
				return coretypes.Int{I: a.I * b.I}
			}
		}
	}
	return call2(rf, acc, val)
}

func makeMapTransducer(f coretypes.Callable) coretypes.Object {
	step := xformStep{kind: xformMap, fn: f}
	if fn, ok := f.(*Fn); ok && isSquareFnExpr(fn.fnExpr) {
		step.intrinsic = reducibleSquareInt
	}
	return &XForm{steps: []xformStep{step}}
}

func makeFilterTransducer(pred coretypes.Callable) coretypes.Object {
	step := xformStep{kind: xformFilter, fn: pred}
	if findFnVarNameCallable(pred) == "even?" {
		step.intrinsic = reducibleEvenInt
	}
	return &XForm{steps: []xformStep{step}}
}

func makeTakeTransducer(n int) coretypes.Object {
	if n < 0 {
		n = 0
	}
	return &XForm{steps: []xformStep{{kind: xformTake, n: n}}}
}

func referToUser(sym coretypes.Symbol, vr *Var) {
	userNs := GLOBAL_ENV.FindNamespace(coretypes.MakeSymbol(STRINGS.Intern, "user"))
	if userNs != nil {
		userNs.Refer(sym, vr)
	}
}

func installTransducerCompat() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// Fix reduce-kv to handle nil coll (returns init)
	rkvVr := ns.Resolve("reduce-kv")
	if rkvVr != nil {
		origRKV, ok := rkvVr.Value.(coretypes.Callable)
		if ok {
			rkvVr.Value = Proc{Name: "procReduceKvNilSafe", Fn: func(args []coretypes.Object) coretypes.Object {
				if len(args) >= 3 {
					coll := args[2]
					if coll == nil {
						return args[1]
					}
					if _, ok := coll.(Nil); ok {
						return args[1]
					}
				}
				return origRKV.Call(args)
			}}
		}
	}

	// reduced — wraps value in Reduced box
	reducedVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "reduced"))
	reducedVr.Value = Proc{Name: "procReduced", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return MakeReduced(args[0])
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "reduced"), reducedVr)

	// reduced? — type check, no map lookup
	reducedQVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "reduced?"))
	reducedQVr.Value = Proc{Name: "procReducedQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return coretypes.MakeBoolean(IsReduced(args[0]))
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "reduced?"), reducedQVr)

	// ensure-reduced
	ensureReducedVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "ensure-reduced"))
	ensureReducedVr.Value = Proc{Name: "procEnsureReduced", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return EnsureReduced(args[0])
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "ensure-reduced"), ensureReducedVr)

	// unreduced — deref a Reduced box (identity if not reduced)
	unreducedVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "unreduced"))
	unreducedVr.Value = Proc{Name: "procUnreduced", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return DerefReduced(args[0])
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "unreduced"), unreducedVr)

	// completing — wraps a reducing fn with optional completion step
	completingVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "completing"))
	completingVr.Value = Proc{Name: "procCompleting", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) != 1 && len(args) != 2 {
			coretypes.RuntimePanicArityMinMax(len(args), 1, 2)
		}
		f := coretypes.EnsureArgIsCallable(args, 0)
		var cf coretypes.Callable
		if len(args) == 2 {
			cf = coretypes.EnsureArgIsCallable(args, 1)
		} else {
			cf = Proc{Name: "procCompletingIdentity", Fn: func(callArgs []coretypes.Object) coretypes.Object {
				runtimeCheckArity(callArgs, 1, 1)
				return callArgs[0]
			}}
		}
		return Proc{Name: "procCompletingRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
			switch len(callArgs) {
			case 0:
				return f.Call(nil)
			case 1:
				return cf.Call(callArgs)
			case 2:
				return f.Call(callArgs)
			default:
				coretypes.RuntimePanicArityMinMax(len(callArgs), 0, 2)
				return NIL
			}
		}}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "completing"), completingVr)

	mapVr := ns.Resolve("map")
	filterVr := ns.Resolve("filter")
	takeVr := ns.Resolve("take")
	sequenceVr := ns.Resolve("sequence")
	compVr := ns.Resolve("comp")
	if mapVr == nil || filterVr == nil || takeVr == nil || sequenceVr == nil || compVr == nil {
		return
	}

	mapOrig, mapOK := mapVr.Value.(coretypes.Callable)
	filterOrig, filterOK := filterVr.Value.(coretypes.Callable)
	takeOrig, takeOK := takeVr.Value.(coretypes.Callable)
	sequenceOrig, sequenceOK := sequenceVr.Value.(coretypes.Callable)
	compOrig, compOK := compVr.Value.(coretypes.Callable)
	if !mapOK || !filterOK || !takeOK || !sequenceOK || !compOK {
		return
	}

	// map transducer arity: (map f) returns a transducer
	mapVr.Value = Proc{Name: "procMapXfCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			f := coretypes.EnsureArgIsCallable(args, 0)
			return makeMapTransducer(f)
		}
		return mapOrig.Call(args)
	}}

	// filter transducer arity: (filter pred) returns a transducer
	filterVr.Value = Proc{Name: "procFilterXfCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			pred := coretypes.EnsureArgIsCallable(args, 0)
			return makeFilterTransducer(pred)
		}
		return filterOrig.Call(args)
	}}

	// take transducer arity: (take n) returns a transducer when used with transduce
	takeVr.Value = Proc{Name: "procTakeXfCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 1 {
			n := coretypes.EnsureArgIsNumber(args, 0).Int().I
			return makeTakeTransducer(n)
		}
		return takeOrig.Call(args)
	}}

	// comp of internal xforms returns a fused pipeline.
	compVr.Value = Proc{Name: "procCompXfCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) > 0 {
			steps := make([]xformStep, 0)
			for _, arg := range args {
				xf, ok := arg.(*XForm)
				if !ok {
					return compOrig.Call(args)
				}
				steps = append(steps, xf.steps...)
			}
			return &XForm{steps: steps}
		}
		return compOrig.Call(args)
	}}

	// transduce — full 3 and 4-arity support
	transduceProc := Proc{Name: "procTransduce", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) != 3 && len(args) != 4 {
			coretypes.RuntimePanicArityMinMax(len(args), 3, 4)
		}

		xform := coretypes.EnsureArgIsCallable(args, 0)
		reducingFnObj := args[1]
		f := coretypes.EnsureArgIsCallable(args, 1)

		var init coretypes.Object
		var collObj coretypes.Object
		if len(args) == 4 {
			init = args[2]
			collObj = args[3]
		} else {
			init = f.Call(nil)
			collObj = args[2]
		}

		return transduceInternal(xform, reducingFnObj, init, collObj)
	}}
	transduceVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "transduce"))
	transduceVr.Value = transduceProc
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "transduce"), transduceVr)

	// eduction — materializes transducer pipeline into a vector
	eductionVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "eduction"))
	eductionVr.Value = Proc{Name: "procEduction", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			coretypes.RuntimePanicArityMinMax(len(args), 2, 999)
		}
		collObj := args[len(args)-1]
		var xformObj coretypes.Object
		if len(args) == 2 {
			xformObj = args[0]
		} else {
			compVr := ns.Resolve("comp")
			if compVr == nil {
				panic(coretypes.RuntimeError("Unable to resolve core/comp for eduction"))
			}
			compFn := coretypes.EnsureObjectIsCallable(compVr.Value, "comp must be callable, got %s")
			xformObj = compFn.Call(args[:len(args)-1])
		}
		xform := coretypes.EnsureObjectIsCallable(xformObj, "eduction expected callable xform, got %s")

		conjRF := Proc{Name: "procEductionConjRF", Fn: func(callArgs []coretypes.Object) coretypes.Object {
			switch len(callArgs) {
			case 0:
				return corecollections.EmptyArrayVector()
			case 1:
				return callArgs[0]
			case 2:
				acc, ok := callArgs[0].(coretypes.Conjable)
				if !ok {
					panic(FailArg(callArgs[0], "coretypes.Conjable", 0))
				}
				return acc.Conj(callArgs[1]).(coretypes.Object)
			default:
				coretypes.RuntimePanicArityMinMax(len(callArgs), 0, 2)
				return NIL
			}
		}}

		return transduceInternal(xform, conjRF, corecollections.EmptyArrayVector(), collObj)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "eduction"), eductionVr)

	// sequence 2-arity: (sequence xform coll) → lazy seq of eduction result
	sequenceVr.Value = Proc{Name: "procSequenceCompat", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 2 {
			res := eductionVr.Value.(coretypes.Callable).Call(args)
			if s, ok := res.(coretypes.Seqable); ok {
				return s.Seq()
			}
			return NIL
		}
		return sequenceOrig.Call(args)
	}}
}

func init() {
	installTransducerCompat()
	maybeOverrideRange()
}

// ---- reduced.go ----
// reduced.go — Proper Reduced type for transducer early termination.
//
// In Clojure, (reduced x) wraps x in a Reduced box that signals
// early termination to reduce/transduce. This replaces the corecollections.ArrayMap-based
// shim with a proper type that's fast to create, check, and unwrap.

// Reduced wraps a value to signal early termination in reduce/transduce.
type Reduced struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	Val coretypes.Object
}

func (r *Reduced) ToString(escape bool) string {
	return "#object[Reduced " + r.Val.ToString(escape) + "]"
}

func (r *Reduced) Equals(other interface{}) bool {
	if o, ok := other.(*Reduced); ok {
		return r.Val.Equals(o.Val)
	}
	return false
}

func (r *Reduced) GetType() *coretypes.Type {
	return TYPE.Fn // reuse Fn type slot for now
}

func (r *Reduced) Hash() uint32 {
	return r.Val.Hash() ^ 0xDEADBEEF
}

func (r *Reduced) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *r
	res.Info = info
	return &res
}

func (r *Reduced) WithMeta(m coretypes.Map) coretypes.Object {
	res := *r
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}

// MakeReduced wraps a value in a Reduced box.
func MakeReduced(val coretypes.Object) *Reduced {
	return &Reduced{Val: val}
}

// IsReduced checks if an object is a Reduced box (type assertion, no map lookup).
func IsReduced(obj coretypes.Object) bool {
	_, ok := obj.(*Reduced)
	return ok
}

// DerefReduced unwraps a Reduced box, returning the inner value.
// If not reduced, returns the value as-is.
func DerefReduced(obj coretypes.Object) coretypes.Object {
	if r, ok := obj.(*Reduced); ok {
		return r.Val
	}
	return obj
}

// EnsureReduced wraps a value in Reduced if it isn't already.
func EnsureReduced(obj coretypes.Object) *Reduced {
	if r, ok := obj.(*Reduced); ok {
		return r
	}
	return MakeReduced(obj)
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
	info := err.obj.GetInfo()
	if info != nil {
		line, column, filename = info.StartLine, info.StartColumn, info.FilenameOrUnknown()
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
			vr.isPrivate = ToBool(p)
		}
		if ok, p := meta.Get(KEYWORDS.dynamic); ok {
			vr.isDynamic = ToBool(p)
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
		if !IsSymbol(sym) {
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
				if !IsSymbol(variadic) {
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
	if IsSymbol(p) { // self reference
		res.self = p.(coretypes.Symbol)
		res.traceName = res.self.ToString(false)
		bodies = bodies.Rest()
		p = bodies.First()
		ctx.PushLocalFrame([]coretypes.Symbol{res.self})
		defer ctx.PopLocalFrame()
	}
	if IsVector(p) { // single arity
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
			if !IsVector(params) {
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
	return IsSeq(obj) && obj.(coretypes.Seq).First().Equals(SYMBOLS.catch)
}

func isFinally(obj coretypes.Object) bool {
	return IsSeq(obj) && obj.(coretypes.Seq).First().Equals(SYMBOLS.finally)
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
	if !IsSymbol(excSymbol) {
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
			return ToBool(v)
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
			case *EvalError:
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
	}
)

func NewReader(runeReader io.RuneReader, filename string) *Reader {
	return &Reader{
		RuneStream: corereader.NewRuneStream(runeReader, func(err error) {
			panic(RT.NewError(err.Error()))
		}),
		filename: STRINGS.Intern(filename),
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

var posStack = corereader.NewPositionStack(8)

func pushPos(reader *Reader) {
	posStack.Push(corereader.Position{Line: reader.Line(), Column: reader.Column()})
}

func popPos() corereader.Position {
	p, ok := posStack.Pop()
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
	p := popPos()
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
			if !IsKeyword(feature) {
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
	if corereader.ShouldAppendMapCommentSurrogate(FORMAT_MODE, isComment(obj)) {
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
	if vr == nil || vr.Value == nil || IsNil(vr.Value) {
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
		popPos()
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
		popPos()
		nextObj := readFirst(reader)
		prefix, _ := corereader.DispatchFormatPrefix(kind)
		addPrefix(nextObj, prefix)
		return nextObj, false
	case corereader.DispatchMeta:
		popPos()
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
		popPos()
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
	popPos()
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
		popPos()
		nextObj := readFirst(reader)
		if FORMAT_MODE {
			prefix, _ := corereader.ReaderMacroPrefix(r)
			addPrefix(nextObj, prefix)
			return nextObj, false
		}
		return makeQuote(nextObj, SYMBOLS.quote), false
	case corereader.ReadFormDeref:
		popPos()
		nextObj := readFirst(reader)
		if FORMAT_MODE {
			prefix, _ := corereader.ReaderMacroPrefix(r)
			addPrefix(nextObj, prefix)
			return nextObj, false
		}
		return DeriveReadObject(nextObj, readerConstruction.ListFrom([]coretypes.Object{DeriveReadObject(nextObj, SYMBOLS.deref), nextObj})), false
	case corereader.ReadFormUnquote:
		popPos()
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
		popPos()
		nextObj := readFirst(reader)
		if FORMAT_MODE {
			prefix, _ := corereader.ReaderMacroPrefix(r)
			addPrefix(nextObj, prefix)
			return nextObj, false
		}
		return makeSyntaxQuote(nextObj, make(map[*string]coretypes.Symbol), reader), false
	case corereader.ReadFormMeta:
		popPos()
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
			case *EvalError:
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

// ---- tagged_literals.go ----
// tagged_literals.go — Built-in tagged literal readers (#inst, #uuid).

func init() {
	registerTaggedLiterals()
}

func registerTaggedLiterals() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// Register #inst reader — parses ISO 8601 date strings to Time
	instReaderVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "__read-inst"))
	instReaderVr.Value = Proc{Name: "procReadInst", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		s := coretypes.EnsureObjectIsString(args[0], "#inst argument must be a string, got %s")
		t, err := coretypes.ParseInstString(s.S)
		if err != nil {
			panic(RT.NewError(err.Error()))
		}
		return t
	}}
	instReaderVr.isPrivate = true

	// Register #uuid reader — stores as string (no java.util.UUID equivalent)
	uuidReaderVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "__read-uuid"))
	uuidReaderVr.Value = Proc{Name: "procReadUuid", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		s := coretypes.EnsureObjectIsString(args[0], "#uuid argument must be a string, got %s")
		if err := coretypes.ValidateUUIDString(s.S); err != nil {
			panic(RT.NewError(err.Error()))
		}
		return s
	}}
	uuidReaderVr.isPrivate = true

	// Install into default-data-readers
	readersVr := ns.Resolve("default-data-readers")
	if readersVr == nil {
		readersVr = ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "default-data-readers"))
	}

	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeSymbol(STRINGS.Intern, "inst"), instReaderVr)
	m.Add(coretypes.MakeSymbol(STRINGS.Intern, "uuid"), uuidReaderVr)
	readersVr.Value = m

	// Also install *data-readers* dynamic var
	dataReadersVr := ns.Resolve("*data-readers*")
	if dataReadersVr == nil {
		dataReadersVr = ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*data-readers*"))
	}
	dataReadersVr.Value = m
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "*data-readers*"), dataReadersVr)

	// Clojure-compatible fallback hook. If non-nil, called as (f tag value)
	// when a reader tag is not present in *data-readers* or default-data-readers.
	fallbackVr := ns.Resolve("*default-data-reader-fn*")
	if fallbackVr == nil {
		fallbackVr = ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*default-data-reader-fn*"))
	}
	fallbackVr.Value = NIL
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "*default-data-reader-fn*"), fallbackVr)

	// Convenience alias used by some lightweight compatibility tests/docs.
	fallbackAliasVr := ns.Resolve("default-data-reader-fn")
	if fallbackAliasVr == nil {
		fallbackAliasVr = ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "default-data-reader-fn"))
	}
	fallbackAliasVr.Value = fallbackVr
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "default-data-reader-fn"), fallbackAliasVr)
}

// ---- reader_construction.go ----

// ReaderConstructionAdapter is the narrow root-owned construction surface for
// reader/parser objects and expression nodes. Future core/reader extraction
// should route construction through this surface before moving implementation
// files across package boundaries.
type ReaderConstructionAdapter struct{}

var readerConstruction ReaderConstructionAdapter

func (ReaderConstructionAdapter) NewReader(runeReader io.RuneReader, filename string) *Reader {
	return NewReader(runeReader, filename)
}

func (ReaderConstructionAdapter) Read(reader *Reader) (coretypes.Object, bool) {
	return Read(reader)
}

func (ReaderConstructionAdapter) TryRead(reader *Reader) (coretypes.Object, error) {
	return TryRead(reader)
}

func (ReaderConstructionAdapter) ReadError(reader *Reader, msg string) ReadError {
	return makeReadError(reader, msg)
}

func (ReaderConstructionAdapter) ReadObject(reader *Reader, obj coretypes.Object) coretypes.Object {
	return makeReadObject(reader, obj)
}

func (ReaderConstructionAdapter) DeriveReadObject(base coretypes.Object, obj coretypes.Object) coretypes.Object {
	return deriveReadObject(base, obj)
}

func (ReaderConstructionAdapter) Nil() coretypes.Object { return NIL }

func (ReaderConstructionAdapter) Bool(v bool) coretypes.Object { return coretypes.Boolean{B: v} }

func (ReaderConstructionAdapter) Char(v rune) coretypes.Object { return coretypes.Char{Ch: v} }

func (ReaderConstructionAdapter) Int(v int) coretypes.Object { return coretypes.Int{I: v} }

func (ReaderConstructionAdapter) String(v string) coretypes.Object { return coretypes.MakeString(v) }

func (ReaderConstructionAdapter) Symbol(v string) coretypes.Object {
	return coretypes.MakeSymbol(STRINGS.Intern, v)
}

func (ReaderConstructionAdapter) Keyword(v string) coretypes.Object {
	return coretypes.MakeKeyword(STRINGS.Intern, v)
}

func (ReaderConstructionAdapter) ListFrom(values []coretypes.Object) coretypes.Object {
	return corecollections.NewListFrom(values...)
}

func (ReaderConstructionAdapter) VectorFrom(values []coretypes.Object) coretypes.Object {
	return corecollections.NewArrayVectorFrom(values...)
}

func (ReaderConstructionAdapter) PersistentVectorFromSeq(seq coretypes.Seq) coretypes.Object {
	return corecollections.PersistentVectorFrom(corecollections.SeqToSlice(seq))
}

func (ReaderConstructionAdapter) MapLiteral(reader *Reader, values []coretypes.Object, nsname string) coretypes.Object {
	if int64(len(values)) >= corecollections.HASHMAP_THRESHOLD {
		hashMap := corecollections.NewHashMap()
		for i := 0; i < len(values); i += 2 {
			key := resolveKey(values[i], nsname)
			if hashMap.ContainsKey(key) {
				panic(MakeReadError(reader, "Duplicate key "+key.ToString(false)))
			}
			hashMap = hashMap.Assoc(key, values[i+1]).(*corecollections.HashMap)
		}
		return hashMap
	}
	m := corecollections.EmptyArrayMap()
	for i := 0; i < len(values); i += 2 {
		key := resolveKey(values[i], nsname)
		if !m.Add(key, values[i+1]) {
			panic(MakeReadError(reader, "Duplicate key "+key.ToString(false)))
		}
	}
	return m
}

func (ReaderConstructionAdapter) SetLiteral(reader *Reader, values []coretypes.Object) coretypes.Object {
	set := corecollections.EmptySet()
	for _, obj := range values {
		if !set.Add(obj) {
			panic(MakeReadError(reader, "Duplicate set element "+obj.ToString(false)))
		}
	}
	return set
}

func (ReaderConstructionAdapter) Double(v float64) coretypes.Object { return coretypes.MakeDouble(v) }

func (ReaderConstructionAdapter) BigInt(v *big.Int, original string) coretypes.Object {
	return &coretypes.BigInt{B: v, Original: original}
}

func (ReaderConstructionAdapter) BigFloatFromString(value string, original string) (coretypes.Object, bool) {
	return coretypes.MakeBigFloatWithOrig(value, original)
}

func (ReaderConstructionAdapter) RatioOrInt(value string, ratio *big.Rat) coretypes.Object {
	return coretypes.RatioOrIntWithOriginal(value, ratio)
}

func (ReaderConstructionAdapter) Comment(v string) coretypes.Object { return coretypes.Comment{C: v} }

func (ReaderConstructionAdapter) Regex(v *regexp.Regexp) coretypes.Object {
	return coretypes.MakeRegex(v)
}

func (ReaderConstructionAdapter) NumberFromToken(reader *Reader, token corereader.NumberToken) coretypes.Object {
	return numberFromToken(reader, token)
}

func (ReaderConstructionAdapter) MetadataFromObject(obj coretypes.Object) (*corecollections.ArrayMap, bool) {
	return metadataFromObject(obj)
}

func (ReaderConstructionAdapter) WithMeta(obj coretypes.Object, meta *corecollections.ArrayMap) (coretypes.Object, bool) {
	v, ok := obj.(coretypes.Meta)
	if !ok {
		return nil, false
	}
	return deriveReadObject(obj, v.WithMeta(meta)), true
}

func (ReaderConstructionAdapter) SkipRedundantDoMeta() *corecollections.ArrayMap {
	return corecollections.EmptyArrayMap().Plus(coretypes.MakeKeyword(STRINGS.Intern, "skip-redundant-do"), coretypes.Boolean{B: true})
}

func (ReaderConstructionAdapter) LiteralExpr(obj coretypes.Object) *LiteralExpr {
	return NewLiteralExpr(obj)
}

func (ReaderConstructionAdapter) SurrogateExpr(obj coretypes.Object) *LiteralExpr {
	return NewSurrogateExpr(obj)
}

func (ReaderConstructionAdapter) VectorExpr(elements []Expr, pos coretypes.Position) *VectorExpr {
	return &VectorExpr{v: elements, Position: pos}
}

func (ReaderConstructionAdapter) MapExpr(size int, pos coretypes.Position) *MapExpr {
	return &MapExpr{keys: make([]Expr, size), values: make([]Expr, size), Position: pos}
}

func (ReaderConstructionAdapter) SetExpr(size int, pos coretypes.Position) *SetExpr {
	return &SetExpr{elements: make([]Expr, size), Position: pos}
}

func (ReaderConstructionAdapter) SetExprFrom(elements []Expr, pos coretypes.Position) *SetExpr {
	return &SetExpr{elements: elements, Position: pos}
}

func (ReaderConstructionAdapter) MapExprFrom(keys []Expr, values []Expr, pos coretypes.Position) *MapExpr {
	return &MapExpr{keys: keys, values: values, Position: pos}
}
