package core

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"maps"
	"slices"
	"sort"
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
