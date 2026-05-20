package core

import (
	"fmt"
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
