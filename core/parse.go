package core

import (
	"bytes"
	"fmt"
	"github.com/rcarmo/go-joker/core/types/numerical"
	"io"
	"math/big"
	"math/rand"
	"regexp"
	"sort"
	"strconv"
	"unsafe"

	coretypes "github.com/rcarmo/go-joker/core/types"

	"github.com/rcarmo/go-joker/core/hashutil"
	corereader "github.com/rcarmo/go-joker/core/reader"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

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
