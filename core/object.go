//go:generate go run gen/gen_types.go assert coretypes.Comparable Vec Char String Symbol Keyword *Regex Boolean Time Number Seqable Callable *coretypes.Type Meta Int Double Stack Map Set Associative Reversible coretypes.Named coretypes.Comparator *Ratio *BigFloat *BigInt *Namespace *Var Error *Fn Deref *Atom Ref KVReduce Reduce coretypes.Pending *File io.Reader io.Writer coretypes.StringReader io.RuneReader *Channel CountedIndexed
//go:generate go run gen/gen_types.go info *List *ArrayMapSeq *ArrayMap *HashMap *ExInfo *Fn *Var Nil *Ratio *BigInt *BigFloat Char Double Int Boolean Time Keyword *Regex Symbol String Comment *LazySeq *MappingSeq *ArraySeq *ConsSeq *NodeSeq *ArrayNodeSeq *MapSet *Vector *ArrayVector *VectorSeq *VectorRSeq
//go:generate go run -tags gen_code gen/codegen/main.go

package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"reflect"
	"regexp"
	"sync"
	"time"
	"unicode/utf8"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	"github.com/rcarmo/go-joker/core/numutil"
	corert "github.com/rcarmo/go-joker/core/runtime"
	corestr "github.com/rcarmo/go-joker/core/string"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

type (
	Object interface {
		coretypes.Object
		GetType() *coretypes.Type
	}
	Conjable interface {
		Object
		Conj(obj Object) Conjable
	}
	CountedIndexed interface {
		coretypes.Counted
		At(int) Object
	}
	Error interface {
		error
		Object
		Message() Object
	}
	Meta interface {
		GetMeta() Map
		WithMeta(Map) Object
	}
	Ref interface {
		AlterMeta(fn *Fn, args []Object) Map
		ResetMeta(m Map) Map
	}
	MetaHolder struct {
		meta Map
	}
	Char struct {
		coretypes.InfoHolder
		Ch rune
	}
	Double struct {
		D float64
	}
	Int struct {
		I int
	}
	BigInt struct {
		coretypes.InfoHolder
		b        *big.Int
		Original string
	}
	BigFloat struct {
		coretypes.InfoHolder
		b        *big.Float
		Original string
	}
	Ratio struct {
		coretypes.InfoHolder
		r        *big.Rat
		Original string
	}
	Boolean struct {
		coretypes.InfoHolder
		B bool
	}
	Nil struct {
		coretypes.InfoHolder
		n struct{}
	}
	Keyword struct {
		coretypes.InfoHolder
		ns   *string
		name *string
		hash uint32
	}
	Symbol struct {
		coretypes.InfoHolder
		MetaHolder
		ns   *string
		name *string
		hash uint32
	}
	String struct {
		coretypes.InfoHolder
		S string
	}
	Comment struct {
		coretypes.InfoHolder
		C string
	}
	Regex struct {
		coretypes.InfoHolder
		R *regexp.Regexp
	}
	Time struct {
		coretypes.InfoHolder
		T time.Time
	}
	Var struct {
		coretypes.InfoHolder
		MetaHolder
		ns             *Namespace
		name           Symbol
		Value          Object
		expr           Expr
		isMacro        bool
		isPrivate      bool
		isDynamic      bool
		isUsed         bool
		isGloballyUsed bool
		isFake         bool
		taggedType     *coretypes.Type
	}
	ProcFn func([]Object) Object
	Proc   struct {
		Fn      ProcFn
		Name    string
		Package string // "" for core (this package), else e.g. "std/string"
	}
	Fn struct {
		coretypes.InfoHolder
		MetaHolder
		isMacro       bool
		fnExpr        *FnExpr
		env           *LocalEnv
		tailRewritten bool       // tail-self-calls rewritten to recur
		irProg        *IRProgram // cached IR compilation (nil = not attempted, irCompileFailed = failed)
		irProgOnce    uint32     // atomic: 0=not tried, 1=done
		defVar        *Var       // set when this fn is the value of a defn-created var
	}
	ExInfo struct {
		ArrayMap
		rt *goroutineRT
	}
	RecurBindings []Object
	Delay         struct {
		fn      Callable
		runtime *corert.Promise[Object]
	}
	Indexed interface {
		Nth(i int) Object
		TryNth(i int, d Object) Object
	}
	Stack interface {
		Peek() Object
		Pop() Stack
	}
	Gettable interface {
		Get(key Object) (bool, Object)
	}
	Associative interface {
		Conjable
		Gettable
		EntryAt(key Object) *ArrayVector
		Assoc(key, val Object) Associative
	}
	Reversible interface {
		Rseq() Seq
	}
	SortableSlice struct {
		s   []Object
		cmp coretypes.Comparator
	}
	Collection interface {
		Object
		coretypes.Counted
		Seqable
		Empty() Collection
	}
	Atom struct {
		MetaHolder
		mu    sync.Mutex
		value Object
	}
	Deref interface {
		Deref() Object
	}
	KVReduce interface {
		kvreduce(c Callable, init Object) Object
	}
	Reduce interface {
		reduceInit(c Callable, init Object) Object
		reduce(c Callable) Object
	}
)

// stringSeq is a lazy seq over a string's runes; yields Chars on demand.
type stringSeq struct {
	s   string
	off int
}

func newIteratorError() error {
	return errors.New("Iterator reached the end of collection")
}

func getHash() interface {
	Write([]byte) (int, error)
	Sum32() uint32
} {
	return hashutil.New32()
}

func MakeSymbol(nsname string) Symbol {
	ns, local, ok := corestr.SplitQualified(nsname)
	if !ok {
		return Symbol{
			ns:   nil,
			name: STRINGS.Intern(local),
		}
	}
	return Symbol{
		ns:   STRINGS.Intern(ns),
		name: STRINGS.Intern(local),
	}
}

type BySymbolName []Symbol

func (s BySymbolName) Len() int {
	return len(s)
}
func (s BySymbolName) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s BySymbolName) Less(i, j int) bool {
	return s[i].ToString(false) < s[j].ToString(false)
}

const KeywordHashMask uint32 = 0x7334c790

func MakeKeyword(nsname string) Keyword {
	nsName, local, ok := corestr.SplitQualified(nsname)
	if !ok {
		name := STRINGS.Intern(local)
		return Keyword{
			ns:   nil,
			name: name,
			hash: hashutil.Symbol(nil, name) ^ KeywordHashMask,
		}
	}
	ns := STRINGS.Intern(nsName)
	name := STRINGS.Intern(local)
	return Keyword{
		ns:   ns,
		name: name,
		hash: hashutil.Symbol(ns, name) ^ KeywordHashMask,
	}
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

func CheckArity(args []Object, min int, max int) {
	n := len(args)
	if n < min || n > max {
		PanicArityMinMax(n, min, max)
	}
}

func getMap(k Object, args []Object) Object {
	CheckArity(args, 1, 2)
	switch m := args[0].(type) {
	case Map:
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

func (s SortableSlice) Len() int {
	return len(s.s)
}

func (s SortableSlice) Swap(i, j int) {
	s.s[i], s.s[j] = s.s[j], s.s[i]
}

func (s SortableSlice) Less(i, j int) bool {
	return s.cmp.Compare(s.s[i], s.s[j]) == -1
}

func equalsNumbers(x Number, y interface{}) bool {
	switch y := y.(type) {
	case Number:
		return category(x) == category(y) && numbersEq(x, y)
	default:
		return false
	}
}

func (a *Atom) ToString(escape bool) string {
	return "#object[Atom {:val " + a.value.ToString(escape) + "}]"
}

func (a *Atom) Equals(other interface{}) bool {
	return a == other
}

func (a *Atom) GetInfo() *coretypes.ObjectInfo {
	return nil
}

func (a *Atom) GetType() *coretypes.Type {
	return TYPE.Atom
}

func (a *Atom) Hash() uint32 {
	return hashutil.Ptr(uintptr(unsafe.Pointer(a)))
}

func (a *Atom) WithInfo(info *coretypes.ObjectInfo) Object {
	return a
}

func (a *Atom) WithMeta(meta Map) Object {
	res := &Atom{
		value: a.value,
	}
	res.meta = SafeMerge(a.meta, meta)
	return res
}

func (a *Atom) ResetMeta(newMeta Map) Map {
	a.meta = newMeta
	return a.meta
}

func (a *Atom) AlterMeta(fn *Fn, args []Object) Map {
	return AlterMeta(&a.MetaHolder, fn, args)
}

func (a *Atom) Deref() Object {
	a.mu.Lock()
	v := a.value
	a.mu.Unlock()
	return v
}

func (d *Delay) ToString(escape bool) string {
	return "#object[Delay]"
}

func (d *Delay) Equals(other interface{}) bool {
	return d == other
}

func (d *Delay) GetInfo() *coretypes.ObjectInfo {
	return nil
}

func (d *Delay) GetType() *coretypes.Type {
	return TYPE.Delay
}

func (d *Delay) Hash() uint32 {
	return hashutil.Ptr(uintptr(unsafe.Pointer(d)))
}

func (d *Delay) WithInfo(info *coretypes.ObjectInfo) Object {
	return d
}

func (d *Delay) Force() Object {
	if d.runtime == nil {
		d.runtime = corert.NewPromise[Object]()
	}
	if d.runtime.IsRealized() {
		return d.runtime.Await()
	}
	value := call0(d.fn)
	d.runtime.Deliver(value)
	return value
}

func (d *Delay) Deref() Object {
	return d.Force()
}

func (d *Delay) IsRealized() bool {
	return d.runtime != nil && d.runtime.IsRealized()
}

func (rb RecurBindings) ToString(escape bool) string {
	return "#object[RecurBindings]"
}

func (rb RecurBindings) Equals(other interface{}) bool {
	return false
}

func (rb RecurBindings) GetInfo() *coretypes.ObjectInfo {
	return nil
}

func (rb RecurBindings) GetType() *coretypes.Type {
	return TYPE.RecurBindings
}

func (rb RecurBindings) Hash() uint32 {
	return 0
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

func (exInfo *ExInfo) Message() Object {
	if ok, res := exInfo.Get(KEYWORDS.message); ok {
		return res
	}
	return NIL
}

func (exInfo *ExInfo) Error() string {
	var pos coretypes.Position
	_, data := exInfo.Get(KEYWORDS.data)
	ok, form := data.(Map).Get(KEYWORDS.form)
	if ok {
		if form.GetInfo() != nil {
			pos = form.GetInfo().Pos()
		}
	}
	prefix := "Exception"
	if ok, pr := data.(Map).Get(KEYWORDS._prefix); ok {
		prefix = pr.ToString(false)
	}
	_, msg := exInfo.Get(KEYWORDS.message)
	if len(exInfo.rt.callstack.frames) > 0 && !LINTER_MODE {
		return fmt.Sprintf("%s:%d:%d: %s: %s\nStacktrace:\n%s", pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn, prefix, msg.(String).S, exInfo.rt.stacktrace())
	} else {
		return fmt.Sprintf("%s:%d:%d: %s: %s", pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn, prefix, msg.(String).S)
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

func (fn *Fn) WithMeta(meta Map) Object {
	res := *fn
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func (fn *Fn) GetType() *coretypes.Type {
	return TYPE.Fn
}

// clearArgs nils out an args slice to release references for GC.
// This prevents retention of large objects across recursive call chains.
func clearArgs(args []Object) {
	for i := range args {
		args[i] = nil
	}
}

func (fn *Fn) Hash() uint32 {
	return hashutil.Ptr(uintptr(unsafe.Pointer(fn)))
}

func (fn *Fn) Call(args []Object) Object {
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
			if fn.fnExpr.self.name != nil {
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
	var restArgs Object = NIL
	if len(v.args)-1 < len(args) {
		restArgs = &ArraySeq{arr: args, index: len(v.args) - 1}
	}
	vargs := make([]Object, len(v.args))
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

func compare(c Callable, a, b Object) int {
	switch r := call2(c, a, b).(type) {
	case Boolean:
		if r.B {
			return -1
		}
		if EnsureObjectIsBoolean(call2(c, b, a), "").B {
			return 1
		}
		return 0
	default:
		return EnsureObjectIsNumber(r, "Function is not a comparator since it returned a non-integer value%.s").Int().I
	}
}

func (fn *Fn) Compare(a, b coretypes.Object) int {
	return compare(fn, a.(Object), b.(Object))
}

func (p Proc) Call(args []Object) Object {
	defer traceProcCall(p, len(args))()
	return p.Fn(args)
}

func (p Proc) Compare(a, b coretypes.Object) int {
	return compare(p, a.(Object), b.(Object))
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

func (p Proc) WithInfo(*coretypes.ObjectInfo) Object {
	return p
}

func (p Proc) GetType() *coretypes.Type {
	return TYPE.Proc
}

func (p Proc) Hash() uint32 {
	return hashutil.Ptr(reflect.ValueOf(p.Fn).Pointer())
}

func (m MetaHolder) GetMeta() Map {
	return m.meta
}

func AlterMeta(m *MetaHolder, fn *Fn, args []Object) Map {
	meta := m.meta
	if meta == nil {
		meta = NIL
	}
	fargs := append([]Object{meta}, args...)
	m.meta = EnsureObjectIsMap(fn.Call(fargs), "")
	return m.meta
}

func (sym Symbol) WithMeta(meta Map) Object {
	res := sym
	res.meta = SafeMerge(res.meta, meta)
	return res
}

func (v *Var) Name() string {
	return v.ns.Name.ToString(false) + "/" + v.name.ToString(false)
}

func (v *Var) ToString(escape bool) string {
	return "#'" + v.Name()
}

func (v *Var) Equals(other interface{}) bool {
	// TODO: revisit this
	return v == other
}

func (v *Var) WithMeta(meta Map) Object {
	res := *v
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func (v *Var) ResetMeta(newMeta Map) Map {
	v.meta = newMeta
	return v.meta
}

func (v *Var) AlterMeta(fn *Fn, args []Object) Map {
	return AlterMeta(&v.MetaHolder, fn, args)
}

func (v *Var) GetType() *coretypes.Type {
	return TYPE.Var
}

func (v *Var) Hash() uint32 {
	return hashutil.Ptr(uintptr(unsafe.Pointer(v)))
}

func (v *Var) Resolve() Object {
	traceVarDeref(v)
	defer symbolTraceMaybeWrite()
	if v.Value == nil {
		return NIL
	}
	return v.Value
}

func (v *Var) Call(args []Object) Object {
	vl := v.Resolve()
	return EnsureObjectIsCallable(
		vl,
		"Var "+v.ToString(false)+" resolves to "+vl.ToString(false)+", which is not a Fn").Call(args)
}

func (v *Var) Deref() Object {
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

func (n Nil) Seq() Seq {
	return n
}

func (n Nil) First() Object {
	return NIL
}

func (n Nil) Rest() Seq {
	return NIL
}

func (n Nil) IsEmpty() bool {
	return true
}

func (n Nil) Cons(obj Object) Seq {
	return NewListFrom(obj)
}

func (n Nil) Conj(obj Object) Conjable {
	return NewListFrom(obj)
}

func (n Nil) Without(key Object) Map {
	return n
}

func (n Nil) Count() int {
	return 0
}

func (n Nil) Iter() MapIterator {
	return emptyMapIterator
}

func (n Nil) Merge(other Map) Map {
	return other
}

func (n Nil) Assoc(key, value Object) Associative {
	return EmptyArrayMap().Assoc(key, value)
}

func (n Nil) EntryAt(key Object) *ArrayVector {
	return nil
}

func (n Nil) Get(key Object) (bool, Object) {
	return false, NIL
}

func (n Nil) Disjoin(key Object) Set {
	return n
}

func (n Nil) Keys() Seq {
	return NIL
}

func (n Nil) Vals() Seq {
	return NIL
}

func (rat *Ratio) ToString(escape bool) string {
	return rat.r.String()
}

func (rat *Ratio) Equals(other interface{}) bool {
	return equalsNumbers(rat, other)
}

func (rat *Ratio) GetType() *coretypes.Type {
	return TYPE.Ratio
}

func (rat *Ratio) Hash() uint32 {
	return hashutil.GobEncoder(rat.r)
}

func (rat *Ratio) Compare(other coretypes.Object) int {
	return CompareNumbers(rat, EnsureObjectIsNumber(rootObject(other), "Cannot compare Ratio: %s"))
}

func MakeBigInt(b *big.Int) *BigInt {
	return &BigInt{b: b}
}

func MakeRatio(r *big.Rat) *Ratio {
	return &Ratio{r: r}
}

// Helper function that returns a math/big.Int given an int.
func MakeMathBigIntFromInt(i int) *big.Int {
	return MakeMathBigIntFromInt64(int64(i))
}

// Helper function that returns a math/big.Int given an int64.
func MakeMathBigIntFromInt64(i int64) *big.Int {
	return big.NewInt(i)
}

// Helper function that returns a math/big.Int given a uint.
func MakeMathBigIntFromUint(b uint) *big.Int {
	return MakeMathBigIntFromUint64(uint64(b))
}

// Helper function that returns a math/big.Int given a uint64.
func MakeMathBigIntFromUint64(b uint64) *big.Int {
	bigint := big.NewInt(0)
	bigint.SetUint64(b)
	return bigint
}

func (bi *BigInt) ToString(escape bool) string {
	if FORMAT_MODE && bi.Original != "" {
		return bi.Original
	}
	return bi.b.String() + "N"
}

func (bi *BigInt) Equals(other interface{}) bool {
	return equalsNumbers(bi, other)
}

func (bi *BigInt) GetType() *coretypes.Type {
	return TYPE.BigInt
}

func (bi *BigInt) Hash() uint32 {
	return hashutil.GobEncoder(bi.b)
}

func (bi *BigInt) Compare(other coretypes.Object) int {
	return CompareNumbers(bi, EnsureObjectIsNumber(rootObject(other), "Cannot compare BigInt: %s"))
}

func MakeBigFloat(b *big.Float) *BigFloat {
	return &BigFloat{b: b}
}

// Helper function that returns a BigFloat given a string, remembering
// any original string provided, and true if the string had the proper
// format; nil and false otherwise.
func MakeBigFloatWithOrig(s, orig string) (*BigFloat, bool) {
	prec := numutil.ComputeFloatPrecision(s)
	f := new(big.Float)
	f.SetPrec(uint(prec))

	if _, ok := f.SetString(s); ok {
		return &BigFloat{b: f, Original: orig}, true
	}

	return nil, false
}

func (bf *BigFloat) ToString(escape bool) string {
	if FORMAT_MODE && bf.Original != "" {
		return bf.Original
	}
	b := bf.b
	if b.IsInf() {
		if b.Signbit() {
			return "##-Inf"
		}
		return "##Inf"
	}
	return b.Text('g', -1) + "M"
}

func (bf *BigFloat) Equals(other interface{}) bool {
	return equalsNumbers(bf, other)
}

func (bf *BigFloat) GetType() *coretypes.Type {
	return TYPE.BigFloat
}

func (bf *BigFloat) Hash() uint32 {
	return hashutil.GobEncoder(bf.b)
}

func (bf *BigFloat) Compare(other coretypes.Object) int {
	return CompareNumbers(bf, EnsureObjectIsNumber(rootObject(other), "Cannot compare BigFloat: %s"))
}

var asciiCharStringObjects = corestr.NewObjectCache(func(ch rune) Object {
	return String{S: corestr.String(ch)}
})

func charToStringFast(ch rune) string { return corestr.String(ch) }

func charToStringObjectFast(ch rune) Object {
	if obj, ok := asciiCharStringObjects.Lookup(ch); ok {
		return obj
	}
	return String{S: corestr.String(ch)}
}

func EnsureObjectIsStringable(obj Object, pattern string) String {
	switch c := obj.(type) {
	case String:
		return c
	case Char:
		return String{S: string(c.Ch)}
	default:
		panic(FailObject(c, "Stringable", pattern))
	}
}

func EnsureArgIsStringable(args []Object, index int) String {
	switch c := args[index].(type) {
	case String:
		return c
	case Char:
		return String{S: string(c.Ch)}
	default:
		panic(FailArg(c, "Stringable", index))
	}
}

func (c Char) ToString(escape bool) string {
	if escape {
		return corestr.EscapeRune(c.Ch)
	}
	return charToStringFast(c.Ch)
}

func (c Char) Equals(other interface{}) bool {
	switch other := other.(type) {
	case Char:
		return c.Ch == other.Ch
	default:
		return false
	}
}

func (c Char) GetType() *coretypes.Type {
	return TYPE.Char
}

func (c Char) Native() interface{} {
	return c.Ch
}

func (c Char) Hash() uint32 {
	h := getHash()
	h.Write([]byte(string(c.Ch)))
	return h.Sum32()
}

func (c Char) Compare(other coretypes.Object) int {
	c2 := EnsureObjectIsChar(rootObject(other), "Cannot compare Char: %s")
	if c.Ch < c2.Ch {
		return -1
	}
	if c2.Ch < c.Ch {
		return 1
	}
	return 0
}

func MakeBoolean(b bool) Boolean {
	return Boolean{B: b}
}

func MakeTime(t time.Time) Time {
	return Time{T: t}
}

func MakeDouble(d float64) Double {
	return Double{D: d}
}

func (d Double) GetInfo() *coretypes.ObjectInfo { return nil }

func (d Double) ToString(escape bool) string {
	dbl := d.D
	if math.IsInf(dbl, 1) {
		return "##Inf"
	}
	if math.IsInf(dbl, -1) {
		return "##-Inf"
	}
	if math.IsNaN(dbl) {
		return "##NaN"
	}
	res := fmt.Sprintf("%g", dbl)
	if numutil.NeedsDecimalSuffix(res) {
		return res + ".0"
	}
	return res
}

func (d Double) Equals(other interface{}) bool {
	return equalsNumbers(d, other)
}

func (d Double) GetType() *coretypes.Type {
	return TYPE.Double
}

func (d Double) Native() interface{} {
	return d.D
}

func (d Double) Hash() uint32 {
	h := getHash()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, math.Float64bits(d.D))
	h.Write(b)
	return h.Sum32()
}

func (d Double) Compare(other coretypes.Object) int {
	return CompareNumbers(d, EnsureObjectIsNumber(rootObject(other), "Cannot compare Double: %s"))
}

func (i Int) GetInfo() *coretypes.ObjectInfo { return nil }

func (i Int) ToString(escape bool) string {
	return corestr.Int(i.I)
}

func MakeInt(i int) Int {
	return Int{I: i}
}

func MakeIntVector(ii []int) *ArrayVector {
	res := EmptyArrayVector()
	for _, i := range ii {
		res.Append(MakeInt(i))
	}
	return res
}

func MakeIntWithOriginal(orig string, i int) Int {
	return Int{I: i}
}

func (i Int) Equals(other interface{}) bool {
	return equalsNumbers(i, other)
}

func (i Int) GetType() *coretypes.Type {
	return TYPE.Int
}

func (i Int) Native() interface{} {
	return i.I
}

func (i Int) Hash() uint32 {
	h := getHash()
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(i.I))
	h.Write(b)
	return h.Sum32()
}

func (i Int) Compare(other coretypes.Object) int {
	return CompareNumbers(i, EnsureObjectIsNumber(rootObject(other), "Cannot compare Int: %s"))
}

func (b Boolean) ToString(escape bool) string {
	return fmt.Sprintf("%t", b.B)
}

func (b Boolean) Equals(other interface{}) bool {
	switch other := other.(type) {
	case Boolean:
		return b.B == other.B
	default:
		return false
	}
}

func (b Boolean) GetType() *coretypes.Type {
	return TYPE.Boolean
}

func (b Boolean) Native() interface{} {
	return b.B
}

func (b Boolean) Hash() uint32 {
	h := getHash()
	var bs = make([]byte, 1)
	if b.B {
		bs[0] = 1
	} else {
		bs[0] = 0
	}
	h.Write(bs)
	return h.Sum32()
}

func (b Boolean) Compare(other coretypes.Object) int {
	b2 := EnsureObjectIsBoolean(rootObject(other), "Cannot compare Boolean: %s")
	if b.B == b2.B {
		return 0
	}
	if b.B {
		return 1
	}
	return -1
}

func (t Time) ToString(escape bool) string {
	return t.T.String()
}

func (t Time) Equals(other interface{}) bool {
	switch other := other.(type) {
	case Time:
		return t.T.Equal(other.T)
	default:
		return false
	}
}

func (t Time) GetType() *coretypes.Type {
	return TYPE.Time
}

func (t Time) Native() interface{} {
	return t.T
}

func (t Time) Hash() uint32 {
	return hashutil.GobEncoder(t.T)
}

func (t Time) Compare(other coretypes.Object) int {
	t2 := EnsureObjectIsTime(rootObject(other), "Cannot compare Time: %s")
	if t.T.Equal(t2.T) {
		return 0
	}
	if t2.T.Before(t.T) {
		return 1
	}
	return -1
}

func (k Keyword) ToString(escape bool) string {
	if k.ns != nil {
		return ":" + *k.ns + "/" + *k.name
	}
	return ":" + *k.name
}

func (k Keyword) Name() string {
	return *k.name
}

func (k Keyword) Namespace() string {
	if k.ns != nil {
		return *k.ns
	}
	return ""
}

func (k Keyword) Equals(other interface{}) bool {
	switch other := other.(type) {
	case Keyword:
		return k.ns == other.ns && k.name == other.name
	default:
		return false
	}
}

func (k Keyword) GetType() *coretypes.Type {
	return TYPE.Keyword
}

func (k Keyword) Hash() uint32 {
	return k.hash
}

func (k Keyword) Compare(other coretypes.Object) int {
	k2 := EnsureObjectIsKeyword(rootObject(other), "Cannot compare Keyword: %s")
	return corestr.Compare(k.ToString(false), k2.ToString(false))
}

func (k Keyword) Call(args []Object) Object {
	return getMap(k, args)
}

func MakeRegex(r *regexp.Regexp) *Regex {
	return &Regex{R: r}
}

func (rx *Regex) ToString(escape bool) string {
	if escape {
		return "#\"" + rx.R.String() + "\""
	}
	return rx.R.String()
}

func (rx *Regex) Print(w io.Writer, printReadably bool) {
	fmt.Fprint(w, rx.ToString(true))
}

func (rx *Regex) Equals(other interface{}) bool {
	switch other := other.(type) {
	case *Regex:
		return rx.R == other.R
	default:
		return false
	}
}

func (rx *Regex) GetType() *coretypes.Type {
	return TYPE.Regex
}

func (rx *Regex) Hash() uint32 {
	return hashutil.Ptr(uintptr(unsafe.Pointer(rx.R)))
}

func (s Symbol) ToString(escape bool) string {
	if s.ns != nil {
		return *s.ns + "/" + *s.name
	}
	return *s.name
}

func (s Symbol) Name() string {
	return *s.name
}

func (s Symbol) Namespace() string {
	if s.ns != nil {
		return *s.ns
	}
	return ""
}

func (s Symbol) Equals(other interface{}) bool {
	switch other := other.(type) {
	case Symbol:
		return s.ns == other.ns && s.name == other.name
	default:
		return false
	}
}

func (s Symbol) GetType() *coretypes.Type {
	return TYPE.Symbol
}

func (s Symbol) Hash() uint32 {
	return hashutil.Symbol(s.ns, s.name) + 0x9e3779b9
}

func (s Symbol) Compare(other coretypes.Object) int {
	s2 := EnsureObjectIsSymbol(rootObject(other), "Cannot compare Symbol: %s")
	return corestr.Compare(s.ToString(false), s2.ToString(false))
}

func (s Symbol) Call(args []Object) Object {
	return getMap(s, args)
}

func (c Comment) ToString(escape bool) string {
	return c.C
}

func (c Comment) Equals(other interface{}) bool {
	return false
}

func (c Comment) GetType() *coretypes.Type {
	// Comments don't deserve their own type
	// since they are only used in FORMAT mode.
	return TYPE.String
}

func (c Comment) Hash() uint32 {
	h := getHash()
	h.Write([]byte(c.C))
	return h.Sum32()
}

func (s String) ToString(escape bool) string {
	if escape {
		return corestr.EscapeString(s.S)
	}
	return s.S
}

func (s String) Format(w io.Writer, indent int) int {
	fmt.Fprint(w, "\"", s.S, "\"")
	return indent + utf8.RuneCountInString(s.S) + 2
}

func MakeString(s string) String {
	return String{S: s}
}

func MakeChar(r rune) Char {
	return Char{Ch: r}
}

func MakeStringVector(ss []string) *ArrayVector {
	res := EmptyArrayVector()
	for _, s := range ss {
		res.Append(MakeString(s))
	}
	return res
}

func (s String) Equals(other interface{}) bool {
	switch other := other.(type) {
	case String:
		return s.S == other.S
	default:
		return false
	}
}

func (s String) GetType() *coretypes.Type {
	return TYPE.String
}

func (s String) Native() interface{} {
	return s.S
}

func (s String) Hash() uint32 {
	h := getHash()
	h.Write([]byte(s.S))
	return h.Sum32()
}

func (s String) Count() int {
	if stringIsASCII(s.S) {
		return len(s.S)
	}
	return utf8.RuneCountInString(s.S)
}

func (s String) Seq() Seq {
	return &stringSeq{s: s.S, off: 0}
}

func (seq *stringSeq) Seq() Seq          { return seq }
func (seq *stringSeq) SequentialMarker() {}

func (seq *stringSeq) First() Object {
	if seq.off >= len(seq.s) {
		return NIL
	}
	r, _ := utf8.DecodeRuneInString(seq.s[seq.off:])
	return Char{Ch: r}
}

func (seq *stringSeq) Rest() Seq {
	if seq.off >= len(seq.s) {
		return EmptyList
	}
	_, size := utf8.DecodeRuneInString(seq.s[seq.off:])
	return &stringSeq{s: seq.s, off: seq.off + size}
}

func (seq *stringSeq) IsEmpty() bool {
	return seq.off >= len(seq.s)
}

func (seq *stringSeq) Cons(obj Object) Seq {
	return &ConsSeq{first: obj, rest: seq}
}

func (seq *stringSeq) Equals(other interface{}) bool {
	return IsSeqEqual(seq, other)
}

func (seq *stringSeq) ToString(escape bool) string {
	return SeqToString(seq, escape)
}

func (seq *stringSeq) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (seq *stringSeq) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return seq }
func (seq *stringSeq) GetType() *coretypes.Type                             { return TYPE.StringSeq }
func (seq *stringSeq) Hash() uint32                                         { return hashOrdered(seq) }
func (seq *stringSeq) WithMeta(meta Map) Object {
	// stringSeq has no meta; return as-is like other minimal seqs
	return seq
}
func (seq *stringSeq) Pprint(w io.Writer, indent int) int { return pprintSeq(seq, w, indent) }
func (seq *stringSeq) Format(w io.Writer, indent int) int { return formatSeq(seq, w, indent) }

func (s String) Nth(i int) Object {
	if i < 0 {
		panic(RT.NewError(fmt.Sprintf("Negative index: %d", i)))
	}
	// Fast path: for pure ASCII strings, byte index == rune index.
	// Check: if len(s) matches byte count, all chars are single-byte.
	if i < len(s.S) && stringIsASCII(s.S) {
		return Char{Ch: rune(s.S[i])}
	}
	// Slow path: UTF-8 rune iteration
	n := 0
	for _, r := range s.S {
		if n == i {
			return Char{Ch: r}
		}
		n++
	}
	panic(RT.NewError(fmt.Sprintf("Index %d exceeds string's length %d", i, n)))
}

// stringIsASCII returns true if all bytes in s are < 0x80.
// Caches results for strings > 8 bytes to avoid repeated scans.
var asciiCache sync.Map // string -> bool

func stringIsASCII(s string) bool {
	if len(s) <= 8 {
		for i := 0; i < len(s); i++ {
			if s[i] >= 0x80 {
				return false
			}
		}
		return true
	}
	if v, ok := asciiCache.Load(s); ok {
		return v.(bool)
	}
	result := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			result = false
			break
		}
	}
	asciiCache.Store(s, result)
	return result
}

func (s String) TryNth(i int, d Object) Object {
	if i < 0 {
		return d
	}
	if i < len(s.S) && stringIsASCII(s.S) {
		return Char{Ch: rune(s.S[i])}
	}
	n := 0
	for _, r := range s.S {
		if n == i {
			return Char{Ch: r}
		}
		n++
	}
	return d
}

func (s String) Compare(other coretypes.Object) int {
	s2 := EnsureObjectIsString(rootObject(other), "Cannot compare String: %s")
	return corestr.Compare(s.S, s2.S)
}

func IsSymbol(obj Object) bool {
	switch obj.(type) {
	case Symbol:
		return true
	default:
		return false
	}
}

func IsKeyword(obj Object) bool {
	_, ok := obj.(Keyword)
	return ok
}

func IsVector(obj Object) bool {
	switch obj.(type) {
	case Vec:
		return true
	default:
		return false
	}
}

func IsSeq(obj Object) bool {
	switch obj.(type) {
	case Seq:
		return true
	default:
		return false
	}
}

func (x RecurBindings) WithInfo(info *coretypes.ObjectInfo) Object {
	return x
}

func IsEqualOrImplements(abstractType *coretypes.Type, concreteType *coretypes.Type) bool {
	if abstractType.ReflectType.Kind() == reflect.Interface {
		return concreteType.ReflectType.Implements(abstractType.ReflectType)
	} else {
		return concreteType.ReflectType == abstractType.ReflectType
	}
}

func IsInstance(t *coretypes.Type, obj Object) bool {
	if obj.Equals(NIL) {
		return false
	}
	// Interface-typed extension objects may report a concrete sequence/map type
	// from GetType while still satisfying runtime interfaces such as Reduce.
	// Check the actual Go interface first for hot extension paths.
	if t == TYPE.Reduce {
		_, ok := obj.(Reduce)
		return ok
	}
	if t == TYPE.KVReduce {
		_, ok := obj.(KVReduce)
		return ok
	}
	return IsEqualOrImplements(t, obj.GetType())
}

func IsSpecialSymbol(obj Object) bool {
	switch obj := obj.(type) {
	case Symbol:
		return obj.ns == nil && SPECIAL_SYMBOLS[obj.name]
	default:
		return false
	}
}

func MakeMeta(arglists Seq, docstring string, added string) *ArrayMap {
	res := EmptyArrayMap()
	if arglists != nil {
		res.Add(KEYWORDS.arglist, arglists)
	}
	res.Add(KEYWORDS.doc, String{S: docstring})
	res.Add(KEYWORDS.added, String{S: added})
	return res
}

func RegRefType(name string, inst interface{}, doc string) *coretypes.Type {
	if doc != "" {
		doc = "\n  " + doc
	}
	meta := MakeMeta(nil, "(Concrete reference type)"+doc, "1.0")
	meta.Add(KEYWORDS.name, MakeString(name))
	return TYPES.Register(STRINGS.Intern(name), coretypes.NewType(name, reflect.TypeOf(inst), MetaHolder{meta}))
}

func RegType(name string, inst interface{}, doc string) *coretypes.Type {
	if doc != "" {
		doc = "\n  " + doc
	}
	meta := MakeMeta(nil, "(Concrete type)"+doc, "1.0")
	meta.Add(KEYWORDS.name, MakeString(name))
	return TYPES.Register(STRINGS.Intern(name), coretypes.NewType(name, reflect.TypeOf(inst).Elem(), MetaHolder{meta}))
}

func RegInterface(name string, inst interface{}, doc string) *coretypes.Type {
	if doc != "" {
		doc = "\n  " + doc
	}
	meta := MakeMeta(nil, "(Interface type)"+doc, "1.0")
	meta.Add(KEYWORDS.name, MakeString(name))
	return TYPES.Register(STRINGS.Intern(name), coretypes.NewType(name, reflect.TypeOf(inst).Elem(), MetaHolder{meta}))
}

func CountedIndexedToString(v CountedIndexed, escape bool) string {
	var b bytes.Buffer
	b.WriteRune('[')
	cnt := v.Count()
	if cnt > 0 {
		for i := 0; i < cnt-1; i++ {
			b.WriteString(v.At(i).ToString(escape))
			b.WriteRune(' ')
		}
		b.WriteString(v.At(cnt - 1).ToString(escape))
	}
	b.WriteRune(']')
	return b.String()
}

func AreCountedIndexedEqual(v1, v2 CountedIndexed) bool {
	if v1.Count() != v2.Count() {
		return false
	}
	for i := 0; i < v1.Count(); i++ {
		if !v1.At(i).Equals(v2.At(i)) {
			return false
		}
	}
	return true
}

func CountedIndexedHash(v CountedIndexed) uint32 {
	h := getHash()
	for i := 0; i < v.Count(); i++ {
		h.Write(hashutil.Uint32Bytes(v.At(i).Hash()))
	}
	return h.Sum32()
}

func CountedIndexedGet(v CountedIndexed, key Object) (bool, Object) {
	switch key := key.(type) {
	case Int:
		if key.I >= 0 && key.I < v.Count() {
			return true, v.At(key.I)
		}
	}
	return false, nil
}

func CountedIndexedCompare(v1, v2 CountedIndexed) int {
	if v1.Count() > v2.Count() {
		return 1
	}
	if v1.Count() < v2.Count() {
		return -1
	}
	for i := 0; i < v1.Count(); i++ {
		c := EnsureObjectIsComparable(v1.At(i), "").Compare(v2.At(i))
		if c != 0 {
			return c
		}
	}
	return 0
}

func CountedIndexedKvreduce(v CountedIndexed, c Callable, init Object) Object {
	res := init
	for i := 0; i < v.Count(); i++ {
		res = call3(c, res, Int{I: i}, v.At(i))
	}
	return res
}

func CountedIndexedPprint(v CountedIndexed, w io.Writer, indent int) int {
	ind := indent + 1
	fmt.Fprint(w, "[")
	if v.Count() > 0 {
		for i := 0; i < v.Count()-1; i++ {
			pprintObject(v.At(i), indent+1, w)
			fmt.Fprint(w, "\n")
			writeIndent(w, indent+1)
		}
		ind = pprintObject(v.At(v.Count()-1), indent+1, w)
	}
	fmt.Fprint(w, "]")
	return ind + 1
}

func CountedIndexedFormat(v CountedIndexed, w io.Writer, indent int) int {
	ind := indent + 1
	fmt.Fprint(w, "[")
	if v.Count() > 0 {
		for i := 0; i < v.Count()-1; i++ {
			ind = formatObject(v.At(i), ind, w)

			ind = maybeNewLine(w, v.At(i), v.At(i+1), indent+1, ind)
		}
		ind = formatObject(v.At(v.Count()-1), ind, w)
	}
	if v.Count() > 0 {
		if isComment(v.At(v.Count() - 1)) {
			fmt.Fprint(w, "\n")
			writeIndent(w, indent+1)
			ind = indent + 1
		}
	}
	fmt.Fprint(w, "]")
	return ind + 1
}

func CountedIndexedReduce(v CountedIndexed, c Callable) Object {
	switch v.Count() {
	case 0:
		return call0(c)
	case 1:
		return v.At(0)
	default:
		args := make([]Object, 2)
		args[0] = v.At(0)
		args[1] = v.At(1)
		acc := c.Call(args)
		for i := 2; i < v.Count(); i++ {
			args[0] = acc
			args[1] = v.At(i)
			acc = c.Call(args)
		}
		return acc
	}
}

func CountedIndexedReduceInit(v CountedIndexed, c Callable, init Object) Object {
	switch v.Count() {
	case 0:
		return init
	default:
		args := make([]Object, 2)
		args[0] = init
		args[1] = v.At(0)
		acc := c.Call(args)
		for i := 1; i < v.Count(); i++ {
			args[0] = acc
			args[1] = v.At(i)
			acc = c.Call(args)
		}
		return acc
	}
}

func withInfo(obj Object, info *coretypes.ObjectInfo) Object {
	if h, ok := obj.(interface {
		WithInfo(*coretypes.ObjectInfo) Object
	}); ok {
		return h.WithInfo(info)
	}
	return obj
}

func rootObject(obj coretypes.Object) Object {
	return obj.(Object)
}
