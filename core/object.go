//go:generate go run gen/gen_types.go assert coretypes.Comparable coretypes.Vec coretypes.Char coretypes.String coretypes.Symbol coretypes.Keyword *coretypes.Regex coretypes.Boolean coretypes.Time coretypes.Number coretypes.Seqable coretypes.Callable *coretypes.Type coretypes.Meta coretypes.Int coretypes.Double coretypes.Stack coretypes.Map coretypes.Set coretypes.Associative coretypes.Reversible coretypes.Named coretypes.Comparator *coretypes.Ratio *coretypes.BigFloat *coretypes.BigInt *Namespace *Var coretypes.Error *Fn coretypes.Deref *corert.Atom coretypes.Ref coretypes.KVReduce coretypes.Reduce coretypes.Pending *File io.Reader io.Writer coretypes.StringReader io.RuneReader *corert.ObjectChannel coretypes.CountedIndexed
//go:generate go run gen/gen_types.go info *corecollections.List *corecollections.ArrayMapSeq *corecollections.ArrayMap *corecollections.HashMap *ExInfo *Fn *Var Nil *corecollections.LazySeq *corecollections.MappingSeq *corecollections.ArraySeq *corecollections.ConsSeq *corecollections.NodeSeq *corecollections.ArrayNodeSeq *corecollections.MapSet *corecollections.Vector *corecollections.ArrayVector *corecollections.VectorSeq *corecollections.VectorRSeq
//go:generate go run -tags gen_code gen/codegen/main.go

package core

import (
	"fmt"
	coreir "github.com/rcarmo/go-joker/core/ir"
	coretrace "github.com/rcarmo/go-joker/core/trace"
	"math"
	"reflect"
	"time"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

type (
	Nil struct {
		coretypes.InfoHolder
		n struct{}
	}
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
		irProgOnce    uint32     // atomic: 0=not tried, 1=done
		defVar        *Var       // set when this fn is the value of a defn-created var
	}
	ExInfo struct {
		corecollections.ArrayMap
		rt *goroutineRT
	}
)

var NIL = Nil{}

func init() {
	coretypes.RuntimeNil = NIL
	coretypes.RuntimeError = func(msg string) any { return RT.NewError(msg) }
	coretypes.RuntimePanicArityMinMax = PanicArityMinMax
	coretypes.RuntimePprintObject = pprintObject
	coretypes.RuntimeFormatObject = formatObject
	coretypes.RuntimeMaybeNewLine = maybeNewLine
	coretypes.RuntimeWriteIndent = writeIndent
	coretypes.RuntimeIsComment = isComment
	coretypes.RuntimeIsReduced = IsReduced
	coretypes.RuntimeDerefReduced = DerefReduced
}

// stringSeq is a lazy seq over a string's runes; yields Chars on demand.
type stringSeq struct {
	s   string
	off int
}

func PanicArity(n int) {
	grt := currentGRT()
	name := "<unknown>"
	if grt.currentExpr != nil {
		if tr, ok := grt.currentExpr.(Traceable); ok {
			name = tr.Name()
		}
	}
	panic(RT.NewError(fmt.Sprintf("Wrong number of args (%d) passed to %s", n, name)))
}

func PanicArityMinMax(n, min, max int) {
	grt := currentGRT()
	name := "<unknown>"
	if grt.currentExpr != nil {
		if tr, ok := grt.currentExpr.(Traceable); ok {
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

func equalsNumbers(x coretypes.Number, y interface{}) bool {
	switch y := y.(type) {
	case coretypes.Number:
		return coretypes.Category(x) == coretypes.Category(y) && coretypes.NumbersEq(x, y)
	default:
		return false
	}
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
	if len(exInfo.rt.callstack.frames) > 0 && !LINTER_MODE {
		return fmt.Sprintf("%s:%d:%d: %s: %s\nStacktrace:\n%s", pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn, prefix, msg.(coretypes.String).S, exInfo.rt.stacktrace())
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
	res := *fn
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
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

func (n Nil) ToString(escape bool) string {
	return "nil"
}

func (n Nil) Equals(other interface{}) bool {
	switch other.(type) {
	case Nil:
		return true
	default:
		return false
	}
}

func (n Nil) GetType() *coretypes.Type {
	return TYPE.Nil
}

func (n Nil) Hash() uint32 {
	return 0
}

func (n Nil) Seq() coretypes.Seq {
	return n
}

func (n Nil) First() coretypes.Object {
	return NIL
}

func (n Nil) Rest() coretypes.Seq {
	return NIL
}

func (n Nil) IsEmpty() bool {
	return true
}

func (n Nil) Cons(obj coretypes.Object) coretypes.Seq {
	return corecollections.NewListFrom(obj)
}

func (n Nil) Conj(obj coretypes.Object) coretypes.Conjable {
	return corecollections.NewListFrom(obj)
}

func (n Nil) Without(key coretypes.Object) coretypes.Map {
	return n
}

func (n Nil) Count() int {
	return 0
}

func (n Nil) Iter() coretypes.MapIterator {
	return coretypes.EmptyMapIteratorInstance
}

func (n Nil) Merge(other coretypes.Map) coretypes.Map {
	return other
}

func (n Nil) Assoc(key, value coretypes.Object) coretypes.Associative {
	return corecollections.EmptyArrayMap().Assoc(key, value)
}

func (n Nil) EntryAt(key coretypes.Object) coretypes.Object {
	return nil
}

func (n Nil) Get(key coretypes.Object) (bool, coretypes.Object) {
	return false, NIL
}

func (n Nil) Disjoin(key coretypes.Object) coretypes.Set {
	return n
}

func (n Nil) Keys() coretypes.Seq {
	return NIL
}

func (n Nil) Vals() coretypes.Seq {
	return NIL
}

var asciiCharStringObjects = corestr.NewObjectCache(func(ch rune) coretypes.Object {
	return coretypes.String{S: corestr.String(ch)}
})

func charToStringFast(ch rune) string { return corestr.String(ch) }

func charToStringObjectFast(ch rune) coretypes.Object {
	if obj, ok := asciiCharStringObjects.Lookup(ch); ok {
		return obj
	}
	return coretypes.String{S: corestr.String(ch)}
}

func MakeStringVector(ss []string) *corecollections.ArrayVector {
	res := corecollections.EmptyArrayVector()
	for _, s := range ss {
		res.Append(coretypes.MakeString(s))
	}
	return res
}

func IsSymbol(obj coretypes.Object) bool {
	switch obj.(type) {
	case coretypes.Symbol:
		return true
	default:
		return false
	}
}

func IsKeyword(obj coretypes.Object) bool {
	_, ok := obj.(coretypes.Keyword)
	return ok
}

func IsVector(obj coretypes.Object) bool {
	switch obj.(type) {
	case coretypes.Vec:
		return true
	default:
		return false
	}
}

func IsSeq(obj coretypes.Object) bool {
	switch obj.(type) {
	case coretypes.Seq:
		return true
	default:
		return false
	}
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
