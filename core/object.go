//go:generate go run gen/gen_types.go assert coretypes.Comparable coretypes.Vec coretypes.Char coretypes.String coretypes.Symbol coretypes.Keyword *coretypes.Regex coretypes.Boolean coretypes.Time coretypes.Number coretypes.Seqable coretypes.Callable *coretypes.Type coretypes.Meta coretypes.Int coretypes.Double coretypes.Stack coretypes.Map coretypes.Set coretypes.Associative coretypes.Reversible coretypes.Named coretypes.Comparator *coretypes.Ratio *coretypes.BigFloat *coretypes.BigInt *Namespace *Var coretypes.Error *Fn coretypes.Deref *corert.Atom coretypes.Ref coretypes.KVReduce coretypes.Reduce coretypes.Pending *File io.Reader io.Writer coretypes.StringReader io.RuneReader *corert.ObjectChannel coretypes.CountedIndexed
//go:generate go run gen/gen_types.go info *corecollections.List *corecollections.ArrayMapSeq *corecollections.ArrayMap *corecollections.HashMap *ExInfo *Fn *Var Nil *corecollections.LazySeq *corecollections.MappingSeq *corecollections.ArraySeq *corecollections.ConsSeq *corecollections.NodeSeq *corecollections.ArrayNodeSeq *corecollections.MapSet *corecollections.Vector *corecollections.ArrayVector *corecollections.VectorSeq *corecollections.VectorRSeq
//go:generate go run -tags gen_code gen/codegen/main.go

package core

import (
	"fmt"
	coreir "github.com/rcarmo/go-joker/core/ir"
	"github.com/rcarmo/go-joker/core/osutil"
	coretrace "github.com/rcarmo/go-joker/core/trace"
	"io"
	"math"
	"math/big"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	corert "github.com/rcarmo/go-joker/core/runtime"
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

// ---- common.go ----
var exitCallbacks []func()

func ExitJoker(rc int) {
	for _, f := range exitCallbacks {
		f()
	}
	os.Exit(rc)
}

func OnExit(f func()) {
	exitCallbacks = append(exitCallbacks, f)
}

func writeIndent(w io.Writer, n int) {
	space := []byte(" ")
	for i := 0; i < n; i++ {
		w.Write(space)
	}
}

func pprintObject(obj coretypes.Object, indent int, w io.Writer) int {
	switch obj := obj.(type) {
	case coretypes.Pprinter:
		return obj.Pprint(w, indent)
	default:
		s := obj.ToString(true)
		fmt.Fprint(w, s)
		return indent + len(s)
	}
}

func formatObject(obj coretypes.Object, indent int, w io.Writer) int {
	if info := obj.GetInfo(); info != nil {
		fmt.Fprint(w, info.Prefix)
		indent += utf8.RuneCountInString(info.Prefix)
	}
	switch obj := obj.(type) {
	case coretypes.Formatter:
		return obj.Format(w, indent)
	default:
		s := obj.ToString(true)
		fmt.Fprint(w, s)
		return indent + utf8.RuneCountInString(s)
	}
}

func isComment(obj coretypes.Object) bool {
	if _, ok := obj.(coretypes.Comment); ok {
		return true
	}
	info := obj.GetInfo()
	if info == nil {
		return false
	}
	return info.Prefix == "^" || info.Prefix == "#^" || info.Prefix == "#_"
}

func isComma(obj coretypes.Object) bool {
	if c, ok := obj.(coretypes.Comment); ok && c.C == "," {
		return true
	}
	return false
}

func maybeNewLine(w io.Writer, obj, nextObj coretypes.Object, baseIndent, currentIndent int) int {
	if writeNewLines(w, obj, nextObj) > 0 {
		writeIndent(w, baseIndent)
		return baseIndent
	}
	if !isComma(nextObj) {
		fmt.Fprint(w, " ")
	}
	return currentIndent + 1
}

func FileInfoMap(name string, info os.FileInfo) coretypes.Map {
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString(name))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "size"), coretypes.IntOrBigInt(big.NewInt(info.Size())))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "mode"), coretypes.MakeInt(int(info.Mode())))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "modtime"), coretypes.MakeTime(info.ModTime()))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "dir?"), coretypes.MakeBoolean(info.IsDir()))
	return m
}

func ToBool(obj coretypes.Object) bool {
	switch obj := obj.(type) {
	case Nil:
		return false
	case coretypes.Boolean:
		return obj.B
	default:
		return true
	}
}

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

func EnsureObjectIsFile(obj coretypes.Object, pattern string) *File {
	if c, yes := obj.(*File); yes {
		return c
	}
	panic(FailObject(obj, "File", pattern))
}

func EnsureArgIsFile(args []coretypes.Object, index int) *File {
	obj := args[index]
	if c, yes := obj.(*File); yes {
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

func (x Nil) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	x.Info = info
	return x
}

// ---- string_runtime.go ----
type StringCursor struct {
	coretypes.InfoHolder
	rt *corestr.CursorRuntime
}

func NewStringCursor(s string) *StringCursor { return &StringCursor{rt: corestr.NewCursorRuntime(s)} }
func (c *StringCursor) Done() bool           { return c.rt.Done() }
func (c *StringCursor) Char() rune           { return c.rt.Char() }
func (c *StringCursor) Index() int           { return c.rt.Index() }
func (c *StringCursor) Next() *StringCursor {
	next := c.rt.Next()
	if next == c.rt {
		return c
	}
	return &StringCursor{rt: next}
}
func (c *StringCursor) ToString(escape bool) string { return c.rt.String() }
func (c *StringCursor) Equals(other interface{}) bool {
	o, ok := other.(*StringCursor)
	return ok && c.rt.Equal(o.rt)
}
func (c *StringCursor) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (c *StringCursor) Hash() uint32                                         { return c.rt.Hash() }
func (c *StringCursor) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return c }
func (c *StringCursor) GetType() *coretypes.Type                             { return typeStringCursor }

var typeStringCursor = &coretypes.Type{Name: "StringCursor"}

type TransientString struct {
	rt *corestr.RuntimeTransientString
}

func NewTransientString(s coretypes.String) coretypes.Object {
	return &TransientString{rt: corestr.NewRuntimeTransientString(s.S)}
}

func (ts *TransientString) ToString(escape bool) string { return ts.rt.String() }
func (ts *TransientString) Equals(other interface{}) bool {
	switch v := other.(type) {
	case *TransientString:
		return ts.rt.String() == v.rt.String()
	case coretypes.String:
		return ts.rt.String() == v.S
	default:
		return false
	}
}
func (ts *TransientString) GetInfo() *coretypes.ObjectInfo                  { return nil }
func (ts *TransientString) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return ts }
func (ts *TransientString) GetType() *coretypes.Type                        { return TYPE.String }
func (ts *TransientString) Hash() uint32                                    { return coretypes.String{S: ts.rt.String()}.Hash() }
func (ts *TransientString) Count() int                                      { return ts.rt.Count() }
func (ts *TransientString) AppendChar(ch rune) *TransientString             { ts.rt.AppendChar(ch); return ts }
func (ts *TransientString) AppendString(s string) *TransientString          { ts.rt.AppendString(s); return ts }
func (ts *TransientString) PrependChar(ch rune) *TransientString            { ts.rt.PrependChar(ch); return ts }
func (ts *TransientString) PrependString(s string) *TransientString {
	ts.rt.PrependString(s)
	return ts
}
func (ts *TransientString) ToPersistent() coretypes.String {
	return coretypes.String{S: ts.rt.Freeze()}
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
	return NewStringCursor(s.S)
}

func procCursorChar(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*StringCursor)
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
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-next expects a StringCursor"))
	}
	return c.Next()
}

func procCursorDone(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-done? expects a StringCursor"))
	}
	return coretypes.Boolean{B: c.Done()}
}

func procCursorIndex(args []coretypes.Object) coretypes.Object {
	c, ok := args[0].(*StringCursor)
	if !ok {
		panic(RT.NewError("cursor-index expects a StringCursor"))
	}
	return coretypes.Int{I: c.Index()}
}

// ---- format.go ----

func seqFirst(seq coretypes.Seq, w io.Writer, indent int) (coretypes.Seq, int) {
	if !seq.IsEmpty() {
		indent = formatObject(seq.First(), indent, w)
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
			indent = formatObject(obj, indent+1, w)
		}
		seq = seq.Rest()
	}
	return seq, obj, indent
}

func writeNewLines(w io.Writer, prevObj coretypes.Object, obj coretypes.Object) int {
	cnt := newLineCount(prevObj, obj)
	for i := 0; i < cnt; i++ {
		fmt.Fprint(w, "\n")
	}
	return cnt
}

func seqFirstAfterBreak(prevObj coretypes.Object, seq coretypes.Seq, w io.Writer, indent int, insideDefRecord bool) (coretypes.Seq, coretypes.Object, int) {
	var obj coretypes.Object
	if !seq.IsEmpty() {
		obj = seq.First()
		writeNewLines(w, prevObj, obj)
		writeIndent(w, indent)
		// coretypes.Seq handling here is needed to properly format methods
		// inside defrecord
		if s, ok := obj.(coretypes.Seq); ok && !obj.Equals(NIL) {
			if info := obj.GetInfo(); info != nil {
				fmt.Fprint(w, info.Prefix)
				indent += utf8.RuneCountInString(info.Prefix)
			}
			indent = formatSeqEx(s, w, indent, insideDefRecord)
		} else {
			indent = formatObject(obj, indent, w)
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
		writeIndent(w, indent)
		indent = formatObject(obj, indent, w)
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
		newIndent = formatObject(v.At(i), indent+1, w)
		if i+1 < v.Count() {
			fmt.Fprint(w, "\n")
			writeIndent(w, indent+1)
		}
	}
	if v.Count() > 0 {
		if isComment(v.At(v.Count() - 1)) {
			fmt.Fprint(w, "\n")
			writeIndent(w, indent+1)
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

func isNewLine(obj, nextObj coretypes.Object) bool {
	info, nextInfo := obj.GetInfo(), nextObj.GetInfo()
	return !(info == nil || nextInfo == nil || info.EndLine == nextInfo.StartLine)
}

func newLineCount(obj, nextObj coretypes.Object) int {
	info, nextInfo := obj.GetInfo(), nextObj.GetInfo()
	if info == nil || nextInfo == nil {
		return 0
	}
	return nextInfo.StartLine - info.EndLine
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
			ind = maybeNewLine(w, prevObj, obj, indent+1, ind)
		}
		ind = formatObject(obj, ind, w)
		prevObj = obj
		seq = seq.Rest()
	}

	if prevObj != nil {
		if isComment(prevObj) {
			fmt.Fprint(w, "\n")
			writeIndent(w, indent+1)
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
				if !isNewLine(obj, seq.First()) {
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
		if !seq.IsEmpty() && !isNewLine(obj, seq.First()) {
			restIndent = i + 1
		}
	} else if formatAsDef {
	} else if isBodyIndent(obj) {
		restIndent = indent + 2
	} else {
		// Indent function call arguments.
		restIndent = indent + 1
		if !seq.IsEmpty() && !isNewLine(obj, seq.First()) {
			restIndent = i + 1
		}
	}

	for !seq.IsEmpty() {
		nextObj := seq.First()
		if isNewLine(obj, nextObj) {
			seq, prevObj, i = seqFirstAfterBreak(prevObj, seq, w, restIndent, isDefRecord)
		} else {
			seq, prevObj, i = seqFirstAfterSpace(seq, w, i, isDefRecord)
		}
		obj = nextObj
	}

	if isComment(obj) {
		fmt.Fprint(w, "\n")
		writeIndent(w, restIndent)
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

func versionMap() coretypes.Map {
	res := corecollections.EmptyArrayMap()
	major, minor, incremental := corestr.ParseVersionTriplet(VERSION)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "major"), coretypes.Int{I: int(major)})
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "minor"), coretypes.Int{I: int(minor)})
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "incremental"), coretypes.Int{I: int(incremental)})
	return res
}

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
	env.stdin.Value = MakeBufferedReader(stdin)
	env.stdout.Value = MakeIOWriter(stdout)
	env.stderr.Value = MakeIOWriter(stderr)
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
	nsRWMu.RLock()
	ns := env.Namespaces[nameKey]
	nsRWMu.RUnlock()
	if ns != nil {
		return ns
	}
	nsRWMu.Lock()
	if env.Namespaces[nameKey] == nil {
		env.Namespaces[nameKey] = NewNamespace(sym)
	}
	ns = env.Namespaces[nameKey]
	nsRWMu.Unlock()
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
			nsRWMu.RLock()
			res = env.Namespaces[nsKey]
			nsRWMu.RUnlock()
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
	nsRWMu.RLock()
	ns := env.Namespaces[s.NameKey()]
	nsRWMu.RUnlock()
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
	nsRWMu.Lock()
	ns := env.Namespaces[nameKey]
	delete(env.Namespaces, nameKey)
	nsRWMu.Unlock()
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
// - Namespace map mutations are protected by nsRWMu.

// goroutineRT holds per-goroutine interpreter state.
type goroutineRT struct {
	callstack   *Callstack
	currentExpr Expr
}

var (
	// mainRT is the default runtime for the main goroutine (hot path).
	mainRT = goroutineRT{
		callstack: &Callstack{frames: make([]Frame, 0, 50)},
	}
	goroutineState *corert.GoRTPool

	// nsRWMu protects GLOBAL_ENV.Namespaces map mutations.
	nsRWMu sync.RWMutex
)

func init() {
	goroutineState = corert.NewGoRTPool(corert.GoID, &mainRT)
}

// currentGRT returns the goroutineRT for the current goroutine.
// HOT PATH: When no spawned goroutines exist (the common case for
// single-threaded execution), returns &mainRT with a single atomic load.
func currentGRT() *goroutineRT {
	return goroutineState.Current().(*goroutineRT)
}

// registerGoroutineRT sets up a new goroutineRT for the current goroutine.
// Called once at goroutine start.
func registerGoroutineRT() *goroutineRT {
	grt := &goroutineRT{callstack: &Callstack{frames: make([]Frame, 0, 20)}}
	goroutineState.Register(grt)
	return grt
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

func init() {
	fillNativeVarMetadata()
}

func fillNativeVarMetadata() {
	if GLOBAL_ENV == nil {
		return
	}
	nsRWMu.RLock()
	namespaces := make([]*Namespace, 0, len(GLOBAL_ENV.Namespaces))
	for _, ns := range GLOBAL_ENV.Namespaces {
		namespaces = append(namespaces, ns)
	}
	nsRWMu.RUnlock()
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
