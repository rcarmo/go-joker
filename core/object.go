//go:generate go run gen/gen_types.go assert coretypes.Comparable coretypes.Vec coretypes.Char coretypes.String coretypes.Symbol coretypes.Keyword *coretypes.Regex coretypes.Boolean coretypes.Time coretypes.Number coretypes.Seqable coretypes.Callable *coretypes.Type coretypes.Meta coretypes.Int coretypes.Double coretypes.Stack coretypes.Map coretypes.Set coretypes.Associative coretypes.Reversible coretypes.Named coretypes.Comparator *coretypes.Ratio *coretypes.BigFloat *coretypes.BigInt *Namespace *Var coretypes.Error *Fn coretypes.Deref *corert.Atom coretypes.Ref coretypes.KVReduce coretypes.Reduce coretypes.Pending *File io.Reader io.Writer coretypes.StringReader io.RuneReader *corert.ObjectChannel coretypes.CountedIndexed
//go:generate go run gen/gen_types.go info *corecollections.List *corecollections.ArrayMapSeq *corecollections.ArrayMap *corecollections.HashMap *ExInfo *Fn *Var Nil *corecollections.LazySeq *corecollections.MappingSeq *corecollections.ArraySeq *corecollections.ConsSeq *corecollections.NodeSeq *corecollections.ArrayNodeSeq *corecollections.MapSet *corecollections.Vector *corecollections.ArrayVector *corecollections.VectorSeq *corecollections.VectorRSeq
//go:generate go run -tags gen_code gen/codegen/main.go

package core

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/rcarmo/go-joker/core/deps"
	coregenerated "github.com/rcarmo/go-joker/core/generated"
	coreir "github.com/rcarmo/go-joker/core/ir"
	"github.com/rcarmo/go-joker/core/osutil"
	corereader "github.com/rcarmo/go-joker/core/reader"
	coretrace "github.com/rcarmo/go-joker/core/trace"
	"github.com/rcarmo/go-joker/core/types/numerical"
	"io"
	"math"
	"math/big"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
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

// ---- procs.go ----

const VERSION = "v42.8.2"

func ExtractCallable(args []coretypes.Object, index int) coretypes.Callable {
	return coretypes.EnsureArgIsCallable(args, index)
}

func ExtractObject(args []coretypes.Object, index int) coretypes.Object {
	return args[index]
}

func ExtractString(args []coretypes.Object, index int) string {
	return coretypes.EnsureArgIsString(args, index).S
}

func ExtractKeyword(args []coretypes.Object, index int) string {
	return coretypes.EnsureArgIsKeyword(args, index).ToString(false)
}

func ExtractStringable(args []coretypes.Object, index int) string {
	return coretypes.EnsureArgIsStringable(args, index).S
}

func ExtractStrings(args []coretypes.Object, index int) []string {
	strs := make([]string, 0)
	for i := index; i < len(args); i++ {
		strs = append(strs, coretypes.EnsureArgIsString(args, i).S)
	}
	return strs
}

func ExtractInt(args []coretypes.Object, index int) int {
	return coretypes.EnsureArgIsInt(args, index).I
}

func ExtractInteger(args []coretypes.Object, index int) int {
	switch c := args[index].(type) {
	case coretypes.Number:
		return c.Int().I
	default:
		panic(RT.NewArgTypeError(index, c, "coretypes.Number"))
	}
}

func ExtractBoolean(args []coretypes.Object, index int) bool {
	return coretypes.EnsureArgIsBoolean(args, index).B
}

func FailArg(obj coretypes.Object, typeName string, index int) *EvalError {
	return RT.NewArgTypeError(index, obj, typeName)
}

func FailObject(obj coretypes.Object, typeName, pattern string) *EvalError {
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

func ExtractChar(args []coretypes.Object, index int) rune {
	return coretypes.EnsureArgIsChar(args, index).Ch
}

func ExtractTime(args []coretypes.Object, index int) time.Time {
	return coretypes.EnsureArgIsTime(args, index).T
}

func ExtractDouble(args []coretypes.Object, index int) float64 {
	return coretypes.EnsureArgIsDouble(args, index).D
}

func ExtractNumber(args []coretypes.Object, index int) coretypes.Number {
	return coretypes.EnsureArgIsNumber(args, index)
}

func ExtractBigInt(args []coretypes.Object, index int) *big.Int {
	return coretypes.EnsureArgIsBigInt(args, index).B
}

func ExtractBigFloat(args []coretypes.Object, index int) *big.Float {
	return coretypes.EnsureArgIsBigFloat(args, index).B
}

func ExtractRegex(args []coretypes.Object, index int) *regexp.Regexp {
	return coretypes.EnsureArgIsRegex(args, index).R
}

func ExtractSeqable(args []coretypes.Object, index int) coretypes.Seqable {
	return coretypes.EnsureArgIsSeqable(args, index)
}

func ExtractMap(args []coretypes.Object, index int) coretypes.Map {
	return coretypes.EnsureArgIsMap(args, index)
}

func ExtractIOReader(args []coretypes.Object, index int) io.Reader {
	return coretypes.EnsureArgIsio_Reader(args, index)
}

func ExtractIOWriter(args []coretypes.Object, index int) io.Writer {
	return coretypes.EnsureArgIsio_Writer(args, index)
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
				return coretypes.String{S: as.S + charToStringFast(bc.Ch)}
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
	return coretypes.Boolean{B: ToBool(args[0])}
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
	pprintObject(obj, 0, w)
	fmt.Fprint(w, "\n")
	return NIL
}

func PrintObject(obj coretypes.Object, w io.Writer) {
	printReadably := ToBool(GLOBAL_ENV.printReadably.Value)
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
	case *File:
		f.Sync()
	}
	return NIL
}

func readFromReader(reader io.RuneReader) coretypes.Object {
	r := readerConstruction.NewReader(reader, "<>")
	obj, err := readerConstruction.TryRead(r)
	PanicOnErr(err)
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
		return MakeBuffer(bytes.NewBufferString(s.S))
	}
	return MakeBuffer(&bytes.Buffer{})
}

var procBufferedReader = func(args []coretypes.Object) coretypes.Object {
	switch rdr := args[0].(type) {
	case io.Reader:
		return MakeBufferedReader(rdr)
	default:
		panic(RT.NewArgTypeError(0, args[0], "IOReader"))
	}
}

var procSlurp = func(args []coretypes.Object) coretypes.Object {
	switch f := args[0].(type) {
	case coretypes.String:
		s, err := osutil.ReadFileString(f.S)
		PanicOnErr(err)
		return coretypes.String{S: s}
	case io.Reader:
		s, err := osutil.ReadAllString(f)
		PanicOnErr(err)
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
		appendFile = ToBool(append)
	}
	switch f := f.(type) {
	case coretypes.String:
		err := osutil.WriteFileString(f.S, str(content), appendFile)
		PanicOnErr(err)
	case io.Writer:
		err := osutil.WriteString(f, str(content))
		PanicOnErr(err)
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
	return coretypes.String{S: VERSION[1:]}
}

var procHash = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Int{I: int(args[0].Hash())}
}

func loadFile(filename string) coretypes.Object {
	var reader *Reader
	f, rr, err := osutil.OpenRuneFile(filename)
	PanicOnErr(err)
	defer func() { PanicOnErr(f.Close()) }()
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
	PanicOnErr(canonicalErr)
	PanicOnErr(err)
	defer func() { PanicOnErr(f.Close()) }()
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
			PanicOnErr(err)
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
			PanicOnErr(err)
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
	isMacro := ToBool(args[2])
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
	ExitJoker(coretypes.EnsureArgIsInt(args, 0).I)
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
		PanicOnErr(err)
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
		PanicOnErr(err)
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
				cnt := newLineCount(prevObj, obj)
				for i := 0; i < cnt; i++ {
					fmt.Fprint(Stdout, "\n")
				}
				if cnt == 0 {
					fmt.Fprint(Stdout, " ")
				}
			}
			formatObject(obj, 0, Stdout)
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
		PanicOnErr(err)
		parseContext.GlobalEnv.SetFilename(coretypes.MakeString(s))
	}
	for {
		obj, err := readerConstruction.TryRead(reader)
		if err == io.EOF {
			return
		}
		PanicOnErr(err)
		expr, err := TryParse(obj, parseContext)
		PanicOnErr(err)
		_, err = TryEval(expr)
		PanicOnErr(err)
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
		PanicOnErr(err)
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
			WARNINGS.ifWithoutElse = ToBool(v)
		}
		if ok, v := m.Get(KEYWORDS.unusedFnParameters); ok {
			WARNINGS.unusedFnParameters = ToBool(v)
		}
		if ok, v := m.Get(KEYWORDS.fnWithEmptyBody); ok {
			WARNINGS.fnWithEmptyBody = ToBool(v)
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
type atomExtras struct {
	validator coretypes.Callable
	watches   map[string]struct {
		key coretypes.Object
		fn  coretypes.Callable
	} // key.ToString → watch
}

var atomExtrasMap sync.Map // *corert.Atom → *atomExtras

func getAtomExtras(a *corert.Atom) *atomExtras {
	if v, ok := atomExtrasMap.Load(a); ok {
		return v.(*atomExtras)
	}
	return nil
}

func getOrCreateAtomExtras(a *corert.Atom) *atomExtras {
	if v, ok := atomExtrasMap.Load(a); ok {
		return v.(*atomExtras)
	}
	ext := &atomExtras{watches: make(map[string]struct {
		key coretypes.Object
		fn  coretypes.Callable
	})}
	atomExtrasMap.Store(a, ext)
	return ext
}

// notifyWatches calls all watch functions with (key atom old-val new-val).
func notifyWatches(a *corert.Atom, oldVal, newVal coretypes.Object) {
	ext := getAtomExtras(a)
	if ext == nil || len(ext.watches) == 0 {
		return
	}
	for _, w := range ext.watches {
		call4(w.fn, w.key, a, oldVal, newVal)
	}
}

// validateAtom checks the validator, panics if invalid.
func validateAtom(a *corert.Atom, newVal coretypes.Object) {
	ext := getAtomExtras(a)
	if ext == nil || ext.validator == nil {
		return
	}
	result := call1(ext.validator, newVal)
	if !ToBool(result) {
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
		if args[1] == nil || IsNil(args[1]) {
			ext.validator = nil
		} else {
			fn := coretypes.EnsureObjectIsCallable(args[1], "validator must be a function, got %s")
			// Validate current value
			result := call1(fn, a.Deref())
			if !ToBool(result) {
				panic(coretypes.RuntimeError("Invalid reference state"))
			}
			ext.validator = fn
		}
		return NIL
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "set-validator!"), svVr)

	// get-validator — (get-validator atom)
	gvVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "get-validator"))
	gvVr.Value = Proc{Name: "procGetValidator", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		a := EnsureObjectIsAtom(args[0], "get-validator requires an atom, got %s")
		ext := getAtomExtras(a)
		if ext == nil || ext.validator == nil {
			return NIL
		}
		return ext.validator.(coretypes.Object)
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
		ext.watches[key.ToString(false)] = struct {
			key coretypes.Object
			fn  coretypes.Callable
		}{key, fn}
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
			delete(ext.watches, key.ToString(false))
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

// IsNil checks if an coretypes.Object is nil or Nil.
func IsNil(obj coretypes.Object) bool {
	if obj == nil {
		return true
	}
	_, ok := obj.(Nil)
	return ok
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
			if args[1] == nil || IsNil(args[1]) {
				return corecollections.EmptyList
			}
			if s, ok := args[1].(coretypes.Seqable); ok {
				return s.Seq()
			}
			return corecollections.EmptyList
		}
		rest := corecollections.ChunkConsRest(args[1], IsNil)
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

// ---- io_objects.go ----
type File struct{ *corereader.File }

func MakeFile(f *os.File) *File { return &File{File: corereader.NewFile(f)} }

func (f *File) ToString(escape bool) string                          { return "#object[File]" }
func (f *File) Equals(other interface{}) bool                        { return f == other }
func (f *File) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (f *File) GetType() *coretypes.Type                             { return TYPE.File }
func (f *File) Hash() uint32                                         { return f.File.Hash() }
func (f *File) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return f }
func (f *File) Namespace() string                                    { return "" }
func ExtractFile(args []coretypes.Object, index int) *File           { return EnsureArgIsFile(args, index) }

type Buffer struct{ *corereader.Buffer }

func MakeBuffer(b *bytes.Buffer) *Buffer { return &Buffer{Buffer: corereader.NewBuffer(b)} }

func (b *Buffer) ToString(escape bool) string                          { return b.String() }
func (b *Buffer) Equals(other interface{}) bool                        { return b == other }
func (b *Buffer) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (b *Buffer) GetType() *coretypes.Type                             { return TYPE.Buffer }
func (b *Buffer) Hash() uint32                                         { return b.Buffer.Hash() }
func (b *Buffer) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return b }

type BufferedReader struct{ *corereader.Buffered }

func MakeBufferedReader(rd io.Reader) *BufferedReader {
	return &BufferedReader{Buffered: corereader.NewBuffered(rd)}
}

func (br *BufferedReader) ToString(escape bool) string                          { return "#object[BufferedReader]" }
func (br *BufferedReader) Equals(other interface{}) bool                        { return br == other }
func (br *BufferedReader) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (br *BufferedReader) GetType() *coretypes.Type                             { return TYPE.BufferedReader }
func (br *BufferedReader) Hash() uint32                                         { return br.Buffered.Hash() }
func (br *BufferedReader) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return br }

type IOReader struct{ *corereader.IOReader }

func MakeIOReader(r io.Reader) *IOReader { return &IOReader{IOReader: corereader.NewIOReader(r)} }

func (ior *IOReader) ToString(escape bool) string                          { return "#object[IOReader]" }
func (ior *IOReader) Equals(other interface{}) bool                        { return ior == other }
func (ior *IOReader) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (ior *IOReader) GetType() *coretypes.Type                             { return TYPE.IOReader }
func (ior *IOReader) Hash() uint32                                         { return ior.IOReader.Hash() }
func (ior *IOReader) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return ior }
func (ior *IOReader) Close() error {
	if err := ior.IOReader.Close(); err != nil {
		if errors.Is(err, corereader.ErrNotClosable) {
			return RT.NewError("coretypes.Object is not closable: " + ior.ToString(false))
		}
		return err
	}
	return nil
}

type IOWriter struct{ *corereader.IOWriter }

func MakeIOWriter(w io.Writer) *IOWriter { return &IOWriter{IOWriter: corereader.NewIOWriter(w)} }

func (iow *IOWriter) ToString(escape bool) string                          { return "#object[IOWriter]" }
func (iow *IOWriter) Equals(other interface{}) bool                        { return iow == other }
func (iow *IOWriter) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (iow *IOWriter) GetType() *coretypes.Type                             { return TYPE.IOWriter }
func (iow *IOWriter) Hash() uint32                                         { return iow.IOWriter.Hash() }
func (iow *IOWriter) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return iow }
func (iow *IOWriter) Close() error {
	if err := iow.IOWriter.Close(); err != nil {
		if errors.Is(err, corereader.ErrNotClosable) {
			return RT.NewError("coretypes.Object is not closable: " + iow.ToString(false))
		}
		return err
	}
	return nil
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
					return coretypes.MakeBoolean(ToBool(v))
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
			if ToBool(pred.Call(cArgs)) {
				return coretypes.Int{I: -1}
			}
			if ToBool(call2(pred, cArgs[1], cArgs[0])) {
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
	return ok && ToBool(v)
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
	return ToBool(call2(pred, a, b))
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

func init() { installCoreAsyncNamespace() }

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
		closeOut = ToBool(args[1])
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
		closeOut = ToBool(args[2])
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
		closeOut = ToBool(args[2])
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
			if ToBool(call1(pred, v)) {
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
			if ToBool(call1(pred, v)) {
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
			if ToBool(call1(pred, v)) {
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
		closep = ToBool(args[2])
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
