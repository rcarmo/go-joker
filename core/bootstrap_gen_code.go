//go:build gen_code
// +build gen_code

// Helpers for gen_code.

package core

import (
	"fmt"
	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	"io"
	"reflect"

	gen_go "github.com/rcarmo/go-joker/core/gen/gengo"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

func filenameAsGo(name string) string {
	return corestr.GoName(corestr.FilenameUnbracketed(name))
}

func positionAsGo(filename *string, startLine, startColumn, endLine, endColumn int) string {
	name := ""
	if filename != nil {
		name = filenameAsGo(*filename)
		if name != "" && name != "_" {
			name += "_"
		}
	}
	return fmt.Sprintf("%s%d_%d__%d_%d", name, startLine, startColumn, endLine, endColumn)
}

func isPositionNil(p coretypes.Position) bool {
	return p.EndLine == 0 && p.EndColumn == 0 && p.StartLine == 0 && p.StartColumn == 0 && (p.Filename == nil || *p.Filename == "")
}

func isObjectInfoNil(p *coretypes.ObjectInfo) bool {
	return p == nil || (p.EndLine == 0 && p.EndColumn == 0 && p.StartLine == 0 && p.StartColumn == 0 && (p.Filename == nil || *p.Filename == ""))
}

func symAsGo(sym coretypes.Symbol) string {
	if sym.NameKey() == nil {
		return "EMPTY"
	} else {
		return corestr.SymbolGoName(sym.ToString(false))
	}
}

func fnExprAsGo(f *FnExpr) string {
	return symAsGo(f.self)
}

func (f *FnExpr) AsGo() string {
	name := fmt.Sprintf("fnExpr_POS_%s", positionAsGo(f.Filename, f.StartLine, f.StartColumn, f.EndLine, f.EndColumn))
	return fmt.Sprintf("%s_NUM_%d", name, ordinalForObj(name, f))
}

func (fn *Fn) AsGo() string {
	if f := fn.fnExpr; f != nil {
		baseName := fmt.Sprintf("fn_%s_POS_%s", fnExprAsGo(f), positionAsGo(f.Filename, f.StartLine, f.StartColumn, f.EndLine, f.EndColumn))
		return fmt.Sprintf("%s_NUM_%d", baseName, ordinalForObj(baseName, fn))
	}
	panic("(*Fn)Asgo(): fn.fnExpr == nil")
}

func (ns *Namespace) AsGo() string {
	file := ""
	name := ns.Name.Name()
	if ns.Name.Info != nil && ns.Name.Info.Filename != nil && *ns.Name.Info.Filename != name && corestr.FilenameUnbracketed(*ns.Name.Info.Filename) != name {
		file = "_FILE_" + corestr.GoName(*ns.Name.Info.Filename)
	}
	return "ns_" + corestr.GoName(name) + file
}

func (e *Env) AsGo() string {
	if e == GLOBAL_ENV {
		return "global_env"
	}
	panic("not GLOBAL_ENV")
}

func kwAsGo(kw coretypes.Keyword) string {
	return corestr.KeywordGoName(kw.ToString(false))
}

func objectInfoAsGo(oi *coretypes.ObjectInfo) string {
	if res, ok := infoHolderAsGoName(*oi); ok {
		return "objectInfo_" + res
	}
	panic("could not make useful name out of coretypes.ObjectInfo")
}

func (v *Var) AsGo() string {
	sym := v.name
	name := symAsGo(sym)
	ns := ""
	if v.ns != nil {
		nsName := v.ns.Name.Name()
		if symNs := sym.Namespace(); symNs != "" && symNs != nsName {
			msg := fmt.Sprintf("Symbol namespace discrepancy: Var %s has %s, its sym has %s", name, nsName, symNs)
			fmt.Fprintln(Stderr, msg)
			panic(msg)
		}
		if sym.NamespaceKey() == nil {
			i := v.ns.Name.Info
			if i == nil || i.Filename == nil || corestr.FilenameUnbracketed(*i.Filename) != nsName {
				ns = "_NS_" + corestr.GoName(nsName)
			}
		}
	}
	pos := ""
	f := v.Info
	if f == nil {
		f = sym.Info
	}
	if f != nil {
		pos = fmt.Sprintf("_POS_%s", positionAsGo(f.Filename, f.StartLine, f.StartColumn, f.EndLine, f.EndColumn))
	}
	return "var" + ns + "_NAME_" + name + pos
}

func (v *VarRefExpr) AsGo() string {
	s := v.vr.name.Name()
	if res, ok := infoHolderAsGoName(*v); ok {
		return "varRef_" + corestr.GoName(s) + "_" + res
	}
	return fmt.Sprintf("%s_%d_%d", corestr.VarRefExprName(s), v.StartLine, v.StartColumn)
}

// Returns typename of object as it should be represented in package
// core.
func typeInCore(e interface{}) string {
	return corestr.TypeNameInCore(fmt.Sprintf("%T", e))
}

func typeInCoreAsGo(e interface{}) string {
	return corestr.TypeNameAsGo(typeInCore(e))
}

func infoHolderAsGoName(obj interface{}) (string, bool) {
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return "", false
	}
	vt := v.Type()
	sf, yes := vt.FieldByName("InfoHolder")
	if yes {
		if !sf.Anonymous {
			return "", false
		}
		v = v.FieldByName("InfoHolder")
		vt = v.Type()
		if vt.Kind() != reflect.Struct {
			return "", false
		}
		sf, yes = vt.FieldByName("Info")
		if !yes || sf.Anonymous {
			return "", false
		}
		v = v.FieldByName("Info")
		vt = v.Type()
		if vt.Kind() != reflect.Ptr {
			panic("'Info' field not a pointer")
		}
		if v.IsNil() {
			return "", false
		}
		v = v.Elem()
		vt = v.Type()
	}
	sf, yes = vt.FieldByName("Position")
	if !yes || !sf.Anonymous {
		return "", false
	}
	v = v.FieldByName("Position")
	vt = v.Type()
	if vt.Kind() != reflect.Struct {
		return "", false
	}
	sf, yes = vt.FieldByName("StartLine")
	if !yes || sf.Anonymous {
		return "", false
	}
	filename := ""
	filenamePtr := gen_go.UnsafeReflectValue(v.FieldByName("Filename"))
	if !(filenamePtr.IsZero() || filenamePtr.IsNil()) {
		filename = filenameAsGo(filenamePtr.Elem().Interface().(string))
		if filename != "" && filename != "_" {
			filename = filename + "_"
		}
	}
	startLine := gen_go.UnsafeReflectValue(v.FieldByName("StartLine")).Interface().(int)
	startColumn := gen_go.UnsafeReflectValue(v.FieldByName("StartColumn")).Interface().(int)
	endLine := gen_go.UnsafeReflectValue(v.FieldByName("EndLine")).Interface().(int)
	endColumn := gen_go.UnsafeReflectValue(v.FieldByName("EndColumn")).Interface().(int)
	return "POS_" + positionAsGo(&filename, startLine, startColumn, endLine, endColumn), true
}

var generatedIds = map[string]*gIdInfo{}

type gIdInfo struct {
	gIds   map[interface{}]uint
	nextId uint
}

func ordinalForObj(id string, obj interface{}) uint {
	info, found := generatedIds[id]
	if !found {
		info = &gIdInfo{map[interface{}]uint{}, 0}
		generatedIds[id] = info
	}
	n, found := info.gIds[obj]
	if !found {
		info.nextId++
		n = info.nextId
		info.gIds[obj] = n
	}
	return n
}

// Tries to call obj.AsGo() and return the result. If that fails,
// cobbles together something reasonable and informative, and returns
// that.
func UniqueId(obj interface{}) (id string) {
	defer func() {
		if r := recover(); r != nil {
			id = typeInCoreAsGo(obj)
			pos, havePos := infoHolderAsGoName(obj)
			if havePos {
				id = fmt.Sprintf("%s_%s", id, pos)
			} else {
				origType := reflect.TypeOf(obj).String()
				if origType == "core.Keyword" || origType == "core.Symbol" {
					fmt.Fprintf(Stderr, "UniqueId: Using %s for %s due to %s\n", id, origType, r)
				}
			}
			n := ordinalForObj(id, obj)
			id = fmt.Sprintf("%s_NUM_%d", id, n)
		}
	}()
	if t, ok := obj.(*coretypes.Type); ok {
		id = "ty_" + corestr.GoName(t.Name)
		return
	}
	id = obj.(interface{ AsGo() string }).AsGo()
	return
}

// Receivers for Joker objects that gen_code.go needs, but no other
// Joker code needs.  (These could be put into object.go, parse.go,
// ns.go, etc., as appropriate, if desired.)

func (v *Var) Expr() Expr {
	return v.expr
}

func (v Var) Namespace() *Namespace {
	return v.ns
}

func (v *VarRefExpr) Var() *Var {
	return v.vr
}

// ---- parse_slow_init.go ----
var (
	GLOBAL_ENV = NewEnv()
	KEYWORDS   = Keywords{
		tag:                coretypes.MakeKeyword(STRINGS.Intern, "tag"),
		skipUnused:         coretypes.MakeKeyword(STRINGS.Intern, "skip-unused"),
		private:            coretypes.MakeKeyword(STRINGS.Intern, "private"),
		line:               coretypes.MakeKeyword(STRINGS.Intern, "line"),
		column:             coretypes.MakeKeyword(STRINGS.Intern, "column"),
		file:               coretypes.MakeKeyword(STRINGS.Intern, "file"),
		ns:                 coretypes.MakeKeyword(STRINGS.Intern, "ns"),
		macro:              coretypes.MakeKeyword(STRINGS.Intern, "macro"),
		message:            coretypes.MakeKeyword(STRINGS.Intern, "message"),
		form:               coretypes.MakeKeyword(STRINGS.Intern, "form"),
		data:               coretypes.MakeKeyword(STRINGS.Intern, "data"),
		cause:              coretypes.MakeKeyword(STRINGS.Intern, "cause"),
		arglist:            coretypes.MakeKeyword(STRINGS.Intern, "arglists"),
		doc:                coretypes.MakeKeyword(STRINGS.Intern, "doc"),
		added:              coretypes.MakeKeyword(STRINGS.Intern, "added"),
		meta:               coretypes.MakeKeyword(STRINGS.Intern, "meta"),
		knownMacros:        coretypes.MakeKeyword(STRINGS.Intern, "known-macros"),
		rules:              coretypes.MakeKeyword(STRINGS.Intern, "rules"),
		ifWithoutElse:      coretypes.MakeKeyword(STRINGS.Intern, "if-without-else"),
		unusedFnParameters: coretypes.MakeKeyword(STRINGS.Intern, "unused-fn-parameters"),
		fnWithEmptyBody:    coretypes.MakeKeyword(STRINGS.Intern, "fn-with-empty-body"),
		_prefix:            coretypes.MakeKeyword(STRINGS.Intern, "_prefix"),
		pos:                coretypes.MakeKeyword(STRINGS.Intern, "pos"),
		startLine:          coretypes.MakeKeyword(STRINGS.Intern, "start-line"),
		endLine:            coretypes.MakeKeyword(STRINGS.Intern, "end-line"),
		startColumn:        coretypes.MakeKeyword(STRINGS.Intern, "start-column"),
		endColumn:          coretypes.MakeKeyword(STRINGS.Intern, "end-column"),
		filename:           coretypes.MakeKeyword(STRINGS.Intern, "filename"),
		object:             coretypes.MakeKeyword(STRINGS.Intern, "object"),
		type_:              coretypes.MakeKeyword(STRINGS.Intern, "type"),
		var_:               coretypes.MakeKeyword(STRINGS.Intern, "var"),
		value:              coretypes.MakeKeyword(STRINGS.Intern, "value"),
		vector:             coretypes.MakeKeyword(STRINGS.Intern, "vector"),
		name:               coretypes.MakeKeyword(STRINGS.Intern, "name"),
		dynamic:            coretypes.MakeKeyword(STRINGS.Intern, "dynamic"),
		require:            coretypes.MakeKeyword(STRINGS.Intern, "require"),
		_import:            coretypes.MakeKeyword(STRINGS.Intern, "import"),
		else_:              coretypes.MakeKeyword(STRINGS.Intern, "else"),
		none:               coretypes.MakeKeyword(STRINGS.Intern, "none"),
		validIdent:         coretypes.MakeKeyword(STRINGS.Intern, "valid-ident"),
		characterSet:       coretypes.MakeKeyword(STRINGS.Intern, "character-set"),
		encodingRange:      coretypes.MakeKeyword(STRINGS.Intern, "encoding-range"),
		core:               coretypes.MakeKeyword(STRINGS.Intern, "core"),
		symbol:             coretypes.MakeKeyword(STRINGS.Intern, "symbol"),
		visible:            coretypes.MakeKeyword(STRINGS.Intern, "visible"),
		ascii:              coretypes.MakeKeyword(STRINGS.Intern, "ascii"),
		unicode:            coretypes.MakeKeyword(STRINGS.Intern, "unicode"),
		any:                coretypes.MakeKeyword(STRINGS.Intern, "any"),
	}
	SYMBOLS = Symbols{
		joker_core:         coretypes.MakeSymbol(STRINGS.Intern, "joker.core"),
		underscore:         coretypes.MakeSymbol(STRINGS.Intern, "_"),
		catch:              coretypes.MakeSymbol(STRINGS.Intern, "catch"),
		finally:            coretypes.MakeSymbol(STRINGS.Intern, "finally"),
		amp:                coretypes.MakeSymbol(STRINGS.Intern, "&"),
		_if:                coretypes.MakeSymbol(STRINGS.Intern, "if"),
		quote:              coretypes.MakeSymbol(STRINGS.Intern, "quote"),
		fn_:                coretypes.MakeSymbol(STRINGS.Intern, "fn*"),
		fn:                 coretypes.MakeSymbol(STRINGS.Intern, "fn"),
		let_:               coretypes.MakeSymbol(STRINGS.Intern, "let*"),
		let:                coretypes.MakeSymbol(STRINGS.Intern, "let"),
		letfn_:             coretypes.MakeSymbol(STRINGS.Intern, "letfn*"),
		letfn:              coretypes.MakeSymbol(STRINGS.Intern, "letfn"),
		loop_:              coretypes.MakeSymbol(STRINGS.Intern, "loop*"),
		loop:               coretypes.MakeSymbol(STRINGS.Intern, "loop"),
		recur:              coretypes.MakeSymbol(STRINGS.Intern, "recur"),
		setMacro_:          coretypes.MakeSymbol(STRINGS.Intern, "set-macro__"),
		def:                coretypes.MakeSymbol(STRINGS.Intern, "def"),
		defLinter:          coretypes.MakeSymbol(STRINGS.Intern, "def-linter__"),
		_var:               coretypes.MakeSymbol(STRINGS.Intern, "var"),
		do:                 coretypes.MakeSymbol(STRINGS.Intern, "do"),
		throw:              coretypes.MakeSymbol(STRINGS.Intern, "throw"),
		try:                coretypes.MakeSymbol(STRINGS.Intern, "try"),
		unquoteSplicing:    coretypes.MakeSymbol(STRINGS.Intern, "unquote-splicing"),
		list:               coretypes.MakeSymbol(STRINGS.Intern, "list"),
		concat:             coretypes.MakeSymbol(STRINGS.Intern, "concat"),
		seq:                coretypes.MakeSymbol(STRINGS.Intern, "seq"),
		apply:              coretypes.MakeSymbol(STRINGS.Intern, "apply"),
		emptySymbol:        coretypes.MakeSymbol(STRINGS.Intern, ""),
		unquote:            coretypes.MakeSymbol(STRINGS.Intern, "unquote"),
		vector:             coretypes.MakeSymbol(STRINGS.Intern, "vector"),
		hashMap:            coretypes.MakeSymbol(STRINGS.Intern, "hash-map"),
		hashSet:            coretypes.MakeSymbol(STRINGS.Intern, "hash-set"),
		defaultDataReaders: coretypes.MakeSymbol(STRINGS.Intern, "default-data-readers"),
		backslash:          coretypes.MakeSymbol(STRINGS.Intern, "/"),
		deref:              coretypes.MakeSymbol(STRINGS.Intern, "deref"),
		ns:                 coretypes.MakeSymbol(STRINGS.Intern, "ns"),
		defrecord:          coretypes.MakeSymbol(STRINGS.Intern, "defrecord"),
		defprotocol:        coretypes.MakeSymbol(STRINGS.Intern, "defprotocol"),
		extendProtocol:     coretypes.MakeSymbol(STRINGS.Intern, "extend-protocol"),
		extendType:         coretypes.MakeSymbol(STRINGS.Intern, "extend-type"),
		deftype:            coretypes.MakeSymbol(STRINGS.Intern, "deftype"),
		proxy:              coretypes.MakeSymbol(STRINGS.Intern, "proxy"),
		reify:              coretypes.MakeSymbol(STRINGS.Intern, "reify"),
	}
	STR = Str{
		_if:          STRINGS.Intern("if"),
		quote:        STRINGS.Intern("quote"),
		fn_:          STRINGS.Intern("fn*"),
		let_:         STRINGS.Intern("let*"),
		letfn_:       STRINGS.Intern("letfn*"),
		loop_:        STRINGS.Intern("loop*"),
		recur:        STRINGS.Intern("recur"),
		setMacro_:    STRINGS.Intern("set-macro__"),
		def:          STRINGS.Intern("def"),
		defLinter:    STRINGS.Intern("def-linter__"),
		_var:         STRINGS.Intern("var"),
		do:           STRINGS.Intern("do"),
		throw:        STRINGS.Intern("throw"),
		try:          STRINGS.Intern("try"),
		coreFilename: STRINGS.Intern("<joker.core>"),
	}
	SPECIAL_SYMBOLS = make(map[*string]bool)
)

func init() {
	SPECIAL_SYMBOLS[SYMBOLS._if.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.quote.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.fn_.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.let_.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.letfn_.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.loop_.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.recur.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.setMacro_.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.def.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.defLinter.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS._var.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.do.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.throw.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.try.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.catch.NameKey()] = true
	SPECIAL_SYMBOLS[SYMBOLS.finally.NameKey()] = true
}

// ---- procs_slow_init.go ----

var privateMeta coretypes.Map = corecollections.EmptyArrayMap().Assoc(KEYWORDS.private, coretypes.Boolean{B: true}).(coretypes.Map)

func intern(name string, proc ProcFn, procName string) {
	vr := GLOBAL_ENV.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, name))
	vr.Value = Proc{Fn: proc, Name: procName}
	vr.isPrivate = true
	vr.Meta = privateMeta
}

func init() {
	GLOBAL_ENV.CoreNamespace.InternVar("*assert*", coretypes.Boolean{B: true},
		MakeMeta(nil, "When set to logical false, assert is a noop. Defaults to true.", "1.0"))

	intern("list__", procList, "procList")
	intern("cons__", procCons, "procCons")
	intern("first__", procFirst, "procFirst")
	intern("next__", procNext, "procNext")
	intern("rest__", procRest, "procRest")
	intern("conj__", procConj, "procConj")
	intern("seq__", procSeq, "procSeq")
	intern("instance?__", procIsInstance, "procIsInstance")
	intern("assoc__", procAssoc, "procAssoc")
	intern("meta__", procMeta, "procMeta")
	intern("with-meta__", procWithMeta, "procWithMeta")
	intern("=__", procEquals, "procEquals")
	intern("count__", procCount, "procCount")
	intern("subvec__", procSubvec, "procSubvec")
	intern("cast__", procCast, "procCast")
	intern("vec__", procVec, "procVec")
	intern("hash-map__", procHashMap, "procHashMap")
	intern("hash-set__", procHashSet, "procHashSet")
	intern("str__", procStr, "procStr")
	intern("symbol__", procSymbol, "procSymbol")
	intern("gensym__", procGensym, "procGensym")
	intern("keyword__", procKeyword, "procKeyword")
	intern("apply__", procApply, "procApply")
	intern("lazy-seq__", procLazySeq, "procLazySeq")
	intern("delay__", procDelay, "procDelay")
	intern("force__", procForce, "procForce")
	intern("identical__", procIdentical, "procIdentical")
	intern("compare__", procCompare, "procCompare")
	intern("zero?__", procIsZero, "procIsZero")
	intern("int__", procInt, "procInt")
	intern("nth__", procNth, "procNth")
	intern("<__", procLt, "procLt")
	intern("<=__", procLte, "procLte")
	intern(">__", procGt, "procGt")
	intern(">=__", procGte, "procGte")
	intern("==__", procEq, "procEq")
	intern("inc'__", procIncEx, "procIncEx")
	intern("inc__", procInc, "procInc")
	intern("dec'__", procDecEx, "procDecEx")
	intern("dec__", procDec, "procDec")
	intern("add'__", procAddEx, "procAddEx")
	intern("add__", procAdd, "procAdd")
	intern("multiply'__", procMultiplyEx, "procMultiplyEx")
	intern("multiply__", procMultiply, "procMultiply")
	intern("divide__", procDivide, "procDivide")
	intern("subtract'__", procSubtractEx, "procSubtractEx")
	intern("subtract__", procSubtract, "procSubtract")
	intern("max__", procMax, "procMax")
	intern("min__", procMin, "procMin")
	intern("pos__", procIsPos, "procIsPos")
	intern("neg__", procIsNeg, "procIsNeg")
	intern("quot__", procQuot, "procQuot")
	intern("rem__", procRem, "procRem")
	intern("bit-not__", procBitNot, "procBitNot")
	intern("bit-and__", procBitAnd, "procBitAnd")
	intern("bit-or__", procBitOr, "procBitOr")
	intern("bit-xor_", procBitXor, "procBitXor")
	intern("bit-and-not__", procBitAndNot, "procBitAndNot")
	intern("bit-clear__", procBitClear, "procBitClear")
	intern("bit-set__", procBitSet, "procBitSet")
	intern("bit-flip__", procBitFlip, "procBitFlip")
	intern("bit-test__", procBitTest, "procBitTest")
	intern("bit-shift-left__", procBitShiftLeft, "procBitShiftLeft")
	intern("bit-shift-right__", procBitShiftRight, "procBitShiftRight")
	intern("unsigned-bit-shift-right__", procUnsignedBitShiftRight, "procUnsignedBitShiftRight")
	intern("peek__", procPeek, "procPeek")
	intern("pop__", procPop, "procPop")
	intern("contains?__", procContains, "procContains")
	intern("get__", procGet, "procGet")
	intern("dissoc__", procDissoc, "procDissoc")
	intern("disj__", procDisj, "procDisj")
	intern("find__", procFind, "procFind")
	intern("keys__", procKeys, "procKeys")
	intern("vals__", procVals, "procVals")
	intern("rseq__", procRseq, "procRseq")
	intern("name__", procName, "procName")
	intern("namespace__", procNamespace, "procNamespace")
	intern("find-var__", procFindVar, "procFindVar")
	intern("sort__", procSort, "procSort")
	intern("eval__", procEval, "procEval")
	intern("type__", procType, "procType")
	intern("num__", procNumber, "procNumber")
	intern("double__", procDouble, "procDouble")
	intern("char__", procChar, "procChar")
	intern("boolean__", procBoolean, "procBoolean")
	intern("numerator__", procNumerator, "procNumerator")
	intern("denominator__", procDenominator, "procDenominator")
	intern("bigint__", procBigInt, "procBigInt")
	intern("bigfloat__", procBigFloat, "procBigFloat")
	intern("pr__", procPr, "procPr")
	intern("pprint__", procPprint, "procPprint")
	intern("newline__", procNewline, "procNewline")
	intern("flush__", procFlush, "procFlush")
	intern("read__", procRead, "procRead")
	intern("read-line__", procReadLine, "procReadLine")
	intern("reader-read-line__", procReaderReadLine, "procReaderReadLine")
	intern("read-string__", procReadString, "procReadString")
	intern("nano-time__", procNanoTime, "procNanoTime")
	intern("macroexpand-1__", procMacroexpand1, "procMacroexpand1")
	intern("load-string__", procLoadString, "procLoadString")
	intern("find-ns__", procFindNamespace, "procFindNamespace")
	intern("create-ns__", procCreateNamespace, "procCreateNamespace")
	intern("inject-ns__", procInjectNamespace, "procInjectNamespace")
	intern("inject-linter-type__", procInjectLinterType, "procInjectLinterType")
	intern("remove-ns__", procRemoveNamespace, "procRemoveNamespace")
	intern("all-ns__", procAllNamespaces, "procAllNamespaces")
	intern("ns-name__", procNamespaceName, "procNamespaceName")
	intern("ns-map__", procNamespaceMap, "procNamespaceMap")
	intern("ns-unmap__", procNamespaceUnmap, "procNamespaceUnmap")
	intern("var-ns__", procVarNamespace, "procVarNamespace")
	intern("ns-initialized?__", procIsNamespaceInitialized, "procIsNamespaceInitialized")
	intern("refer__", procRefer, "procRefer")
	intern("alias__", procAlias, "procAlias")
	intern("ns-aliases__", procNamespaceAliases, "procNamespaceAliases")
	intern("ns-unalias__", procNamespaceUnalias, "procNamespaceUnalias")
	intern("var-get__", procVarGet, "procVarGet")
	intern("var-set__", procVarSet, "procVarSet")
	intern("ns-resolve__", procNsResolve, "procNsResolve")
	intern("array-map__", procArrayMap, "procArrayMap")
	intern("buffer__", procBuffer, "procBuffer")
	intern("buffered-reader__", procBufferedReader, "procBufferedReader")
	intern("ex-info__", procExInfo, "procExInfo")
	intern("ex-data__", procExData, "procExData")
	intern("ex-cause__", procExCause, "procExCause")
	intern("ex-message__", procExMessage, "procExMessage")
	intern("regex__", procRegex, "procRegex")
	intern("re-seq__", procReSeq, "procReSeq")
	intern("re-find__", procReFind, "procReFind")
	intern("rand__", procRand, "procRand")
	intern("special-symbol?__", procIsSpecialSymbol, "procIsSpecialSymbol")
	intern("subs__", procSubs, "procSubs")
	intern("intern__", procIntern, "procIntern")
	intern("set-meta__", procSetMeta, "procSetMeta")
	intern("atom__", procAtom, "procAtom")
	intern("deref__", procDeref, "procDeref")
	intern("swap__", procSwap, "procSwap")
	intern("swap-vals__", procSwapVals, "procSwapVals")
	intern("reset__", procReset, "procReset")
	intern("reset-vals__", procResetVals, "procResetVals")
	intern("alter-meta__", procAlterMeta, "procAlterMeta")
	intern("reset-meta__", procResetMeta, "procResetMeta")
	intern("empty__", procEmpty, "procEmpty")
	intern("bound?__", procIsBound, "procIsBound")
	intern("format__", procFormat, "procFormat")
	intern("load-file__", procLoadFile, "procLoadFile")
	intern("load-lib-from-path__", procLoadLibFromPath, "procLoadLibFromPath")
	intern("reduce-kv__", procReduceKv, "procReduceKv")
	intern("reduce__", procReduce, "procReduce")
	intern("slurp__", procSlurp, "procSlurp")
	intern("spit__", procSpit, "procSpit")
	intern("shuffle__", procShuffle, "procShuffle")
	intern("realized?__", procIsRealized, "procIsRealized")
	intern("derive-info__", procDeriveInfo, "procDeriveInfo")
	intern("joker-version__", procJokerVersion, "procJokerVersion")

	intern("hash__", procHash, "procHash")

	intern("index-of__", procIndexOf, "procIndexOf")
	intern("lib-path__", procLibPath, "procLibPath")
	intern("intern-fake-var__", procInternFakeVar, "procInternFakeVar")
	intern("parse__", procParse, "procParse")
	intern("inc-problem-count__", procIncProblemCount, "procIncProblemCount")
	intern("types__", procTypes, "procTypes")
	intern("go__", procGo, "procGo")
	intern("<!__", procReceive, "procReceive")
	intern(">!__", procSend, "procSend")
	intern("chan__", procCreateChan, "procCreateChan")
	intern("close!__", procCloseChan, "procCloseChan")

	intern("go-spew__", procGoSpew, "procGoSpew")
	intern("verbosity-level__", procVerbosityLevel, "procVerbosityLevel")
	intern("exit__", procExit, "procExit")
	intern("nan?__", procIsNaN, "procIsNaN")
	intern("abs__", procAbs, "procAbs")
	intern("infinite?__", procIsInfinite, "procIsInfinite")
	intern("parseDouble__", procParseDouble, "procParseDouble")
	intern("parseLong__", procParseLong, "procParseLong")

	// Transient operations
	intern("transient__", procTransient, "procTransient")
	intern("assoc!__", procAssocBang, "procAssocBang")
	intern("conj!__", procConjBang, "procConjBang")
	intern("pop!__", procPopBang, "procPopBang")
	intern("persistent!__", procPersistentBang, "procPersistentBang")
	intern("transient?__", procIsTransient, "procIsTransient")
}

func init() {
	// Register transient operations as public core vars
	ns := GLOBAL_ENV.CoreNamespace
	for _, r := range []struct {
		name  string
		fn    ProcFn
		pname string
	}{
		{"transient", procTransient, "procTransient"},
		{"assoc!", procAssocBang, "procAssocBang"},
		{"conj!", procConjBang, "procConjBang"},
		{"pop!", procPopBang, "procPopBang"},
		{"persistent!", procPersistentBang, "procPersistentBang"},
		{"transient?", procIsTransient, "procIsTransient"},
	} {
		vr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, r.name))
		vr.Value = Proc{Fn: r.fn, Name: r.pname}
	}
}

// ---- object_slow_init.go ----

var STRINGS corestr.Pool = corestr.Pool{}
var TYPES = coretypes.Registry{}
var TYPE coretypes.Types
var LINTER_TYPES = map[*string]bool{}

func typeBuilder() coretypes.Builder {
	return coretypes.Builder{
		Registry: TYPES,
		Intern:   STRINGS.Intern,
		MetaFactory: func(kind coretypes.Kind, name string, doc string) any {
			meta := MakeMeta(nil, coretypes.TypeMetadataDoc(kind, doc), "1.0")
			meta.Add(KEYWORDS.name, coretypes.MakeString(name))
			return coretypes.MetaHolder{meta}
		},
	}
}

func init() {
	TYPE = coretypes.Types{
		Associative:    typeBuilder().RegisterInterface("Associative", (*coretypes.Associative)(nil), ""),
		Callable:       typeBuilder().RegisterInterface("coretypes.Callable", (*coretypes.Callable)(nil), ""),
		Collection:     typeBuilder().RegisterInterface("Collection", (*coretypes.Collection)(nil), ""),
		Comparable:     typeBuilder().RegisterInterface("Comparable", (*coretypes.Comparable)(nil), ""),
		Comparator:     typeBuilder().RegisterInterface("Comparator", (*coretypes.Comparator)(nil), ""),
		Counted:        typeBuilder().RegisterInterface("Counted", (*coretypes.Counted)(nil), ""),
		CountedIndexed: typeBuilder().RegisterInterface("coretypes.CountedIndexed", (*coretypes.CountedIndexed)(nil), ""),
		Deref:          typeBuilder().RegisterInterface("Deref", (*coretypes.Deref)(nil), ""),
		Error:          typeBuilder().RegisterInterface("Error", (*coretypes.Error)(nil), ""),
		Gettable:       typeBuilder().RegisterInterface("coretypes.Gettable", (*coretypes.Gettable)(nil), ""),
		Indexed:        typeBuilder().RegisterInterface("coretypes.Indexed", (*coretypes.Indexed)(nil), ""),
		IOReader:       typeBuilder().RegisterInterface("IOReader", (*io.Reader)(nil), ""),
		IOWriter:       typeBuilder().RegisterInterface("IOWriter", (*io.Writer)(nil), ""),
		KVReduce:       typeBuilder().RegisterInterface("coretypes.KVReduce", (*coretypes.KVReduce)(nil), ""),
		Reduce:         typeBuilder().RegisterInterface("coretypes.Reduce", (*coretypes.Reduce)(nil), ""),
		Map:            typeBuilder().RegisterInterface("Map", (*coretypes.Map)(nil), ""),
		Meta:           typeBuilder().RegisterInterface("Meta", (*coretypes.Meta)(nil), ""),
		Named:          typeBuilder().RegisterInterface("Named", (*coretypes.Named)(nil), ""),
		Number:         typeBuilder().RegisterInterface("Number", (*coretypes.Number)(nil), ""),
		Pending:        typeBuilder().RegisterInterface("Pending", (*coretypes.Pending)(nil), ""),
		Ref:            typeBuilder().RegisterInterface("Ref", (*coretypes.Ref)(nil), ""),
		Reversible:     typeBuilder().RegisterInterface("Reversible", (*coretypes.Reversible)(nil), ""),
		Seq:            typeBuilder().RegisterInterface("coretypes.Seq", (*coretypes.Seq)(nil), ""),
		Seqable:        typeBuilder().RegisterInterface("coretypes.Seqable", (*coretypes.Seqable)(nil), ""),
		Sequential:     typeBuilder().RegisterInterface("Sequential", (*coretypes.Sequential)(nil), ""),
		Set:            typeBuilder().RegisterInterface("Set", (*coretypes.Set)(nil), ""),
		Stack:          typeBuilder().RegisterInterface("coretypes.Stack", (*coretypes.Stack)(nil), ""),
		ArrayMap:       typeBuilder().RegisterReference("corecollections.ArrayMap", (*corecollections.ArrayMap)(nil), ""),
		ArrayMapSeq:    typeBuilder().RegisterReference("corecollections.ArrayMapSeq", (*corecollections.ArrayMapSeq)(nil), ""),
		ArrayNodeSeq:   typeBuilder().RegisterReference("corecollections.ArrayNodeSeq", (*corecollections.ArrayNodeSeq)(nil), ""),
		ArraySeq:       typeBuilder().RegisterReference("corecollections.ArraySeq", (*corecollections.ArraySeq)(nil), ""),
		MapSet:         typeBuilder().RegisterReference("corecollections.MapSet", (*corecollections.MapSet)(nil), ""),
		Atom:           typeBuilder().RegisterReference("Atom", (*corert.Atom)(nil), ""),
		BigFloat:       typeBuilder().RegisterReference("coretypes.BigFloat", (*coretypes.BigFloat)(nil), "Wraps the Go 'math/big.Float' type"),
		BigInt:         typeBuilder().RegisterReference("coretypes.BigInt", (*coretypes.BigInt)(nil), "Wraps the Go 'math/big.Int' type"),
		Boolean:        typeBuilder().RegisterValue("Boolean", (*coretypes.Boolean)(nil), "Wraps the Go 'bool' type"),
		Time:           typeBuilder().RegisterValue("Time", (*coretypes.Time)(nil), "Wraps the Go 'time.Time' type"),
		Buffer:         typeBuilder().RegisterReference("Buffer", (*corert.Buffer)(nil), ""),
		Char:           typeBuilder().RegisterValue("Char", (*coretypes.Char)(nil), "Wraps the Go 'rune' type"),
		ConsSeq:        typeBuilder().RegisterReference("corecollections.ConsSeq", (*corecollections.ConsSeq)(nil), ""),
		Delay:          typeBuilder().RegisterReference("coretypes.Delay", (*coretypes.Delay)(nil), ""),
		Channel:        typeBuilder().RegisterReference("Channel", (*corert.ObjectChannel)(nil), ""),
		Double:         typeBuilder().RegisterValue("Double", (*coretypes.Double)(nil), "Wraps the Go 'float64' type"),
		EvalError:      typeBuilder().RegisterReference("EvalError", (*corert.EvalError)(nil), ""),
		ExInfo:         typeBuilder().RegisterReference("ExInfo", (*ExInfo)(nil), ""),
		Fn:             typeBuilder().RegisterReference("Fn", (*Fn)(nil), "A callable function or macro implemented via Joker code"),
		File:           typeBuilder().RegisterReference("File", (*corert.File)(nil), ""),
		BufferedReader: typeBuilder().RegisterReference("BufferedReader", (*corert.BufferedReader)(nil), ""),
		HashMap:        typeBuilder().RegisterReference("corecollections.HashMap", (*corecollections.HashMap)(nil), ""),
		Int: typeBuilder().RegisterValue("Int", (*coretypes.Int)(nil),
			"Wraps the Go 'int' type, which is 32 bits wide on 32-bit hosts, 64 bits wide on 64-bit hosts, etc."),
		Keyword:       typeBuilder().RegisterValue("Keyword", (*coretypes.Keyword)(nil), "A possibly-namespace-qualified name prefixed by ':'"),
		LazySeq:       typeBuilder().RegisterReference("corecollections.LazySeq", (*corecollections.LazySeq)(nil), ""),
		List:          typeBuilder().RegisterReference("corecollections.List", (*corecollections.List)(nil), ""),
		MappingSeq:    typeBuilder().RegisterReference("corecollections.MappingSeq", (*corecollections.MappingSeq)(nil), ""),
		Namespace:     typeBuilder().RegisterReference("Namespace", (*Namespace)(nil), ""),
		Nil:           typeBuilder().RegisterValue("Nil", (*Nil)(nil), "The 'nil' value"),
		NodeSeq:       typeBuilder().RegisterReference("corecollections.NodeSeq", (*corecollections.NodeSeq)(nil), ""),
		ParseError:    typeBuilder().RegisterReference("ParseError", (*ParseError)(nil), ""),
		Proc:          typeBuilder().RegisterReference("Proc", (*Proc)(nil), "A callable function implemented via Go code"),
		Ratio:         typeBuilder().RegisterReference("coretypes.Ratio", (*coretypes.Ratio)(nil), "Wraps the Go 'math.big/Rat' type"),
		RecurBindings: typeBuilder().RegisterReference("RecurBindings", (*coretypes.RecurBindings)(nil), ""),
		Regex:         typeBuilder().RegisterReference("Regex", (*coretypes.Regex)(nil), "Wraps the Go 'regexp.Regexp' type"),
		String:        typeBuilder().RegisterValue("String", (*coretypes.String)(nil), "Wraps the Go 'string' type"),
		Symbol:        typeBuilder().RegisterValue("Symbol", (*coretypes.Symbol)(nil), ""),
		Type:          typeBuilder().RegisterReference("Type", (*coretypes.Type)(nil), ""),
		Var:           typeBuilder().RegisterReference("Var", (*Var)(nil), ""),
		Vector:        typeBuilder().RegisterReference("corecollections.Vector", (*corecollections.Vector)(nil), ""),
		Vec:           typeBuilder().RegisterInterface("Vec", (*coretypes.Vec)(nil), ""),
		ArrayVector:   typeBuilder().RegisterReference("corecollections.ArrayVector", (*corecollections.ArrayVector)(nil), ""),
		VectorRSeq:    typeBuilder().RegisterReference("corecollections.VectorRSeq", (*corecollections.VectorRSeq)(nil), ""),
		VectorSeq:     typeBuilder().RegisterReference("corecollections.VectorSeq", (*corecollections.VectorSeq)(nil), ""),
		StringSeq:     typeBuilder().RegisterReference("StringSeq", (*stringSeq)(nil), ""),
	}
	coretypes.RuntimeTypes = &TYPE
	coretypes.RuntimeNil = NIL
	coretypes.RuntimeError = func(msg string) any { return RT.NewError(msg) }
	coretypes.RuntimePanicArityMinMax = PanicArityMinMax
	coretypes.RuntimePprintObject = pprintObject
	coretypes.RuntimeFormatObject = formatObject
	coretypes.RuntimeMaybeNewLine = maybeNewLine
	coretypes.RuntimeWriteIndent = writeIndent
	coretypes.RuntimeIsComment = isComment
	coretypes.RuntimeIsReduced = corert.IsReduced
	coretypes.RuntimeDerefReduced = corert.DerefReduced
	coretypes.RuntimeReduceType = TYPE.Reduce
	coretypes.RuntimeKVReduceType = TYPE.KVReduce
	coretypes.SpecialSymbolLookup = func(sym coretypes.Symbol) bool { return SPECIAL_SYMBOLS[sym.NameKey()] }
	coretypes.NumberCompare = coretypes.CompareNumbers
	coretypes.NumberEquals = equalsNumbers
	coretypes.NamedLookup = getMap
	coretypes.TransientMutationError = func() any { return RT.NewError("Cannot mutate a frozen transient") }
	coretypes.TransientVectorIndexTypeError = func(obj coretypes.Object) any { return RT.NewArgTypeError(1, obj, "Int") }
	coretypes.TransientVectorToPersistent = func(arr []coretypes.Object) coretypes.Object { return &corecollections.ArrayVector{Arr: arr} }
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
	installAssertionErrors()
	coretypes.DelayCall = call0
}

// ---- environment_slow_init.go ----

func NewEnv() *Env {
	features := corecollections.EmptySet()
	features.Add(coretypes.MakeKeyword(STRINGS.Intern, "default"))
	features.Add(coretypes.MakeKeyword(STRINGS.Intern, "joker"))
	res := &Env{
		Namespaces: make(map[*string]*Namespace),
		Features:   features,
	}
	res.CoreNamespace = res.EnsureSymbolIsNamespace(SYMBOLS.joker_core)
	res.CoreNamespace.Meta = MakeMeta(nil, "Core library of Joker.", "1.0")
	res.NS_VAR = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "ns"))
	res.IN_NS_VAR = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "in-ns"))
	res.ns = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*ns*"))
	res.stdin = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*in*"))
	res.stdout = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*out*"))
	res.stderr = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*err*"))
	res.file = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*file*"))
	res.MainFile = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*main-file*"))
	res.version = res.CoreNamespace.InternVar("*joker-version*", versionMap(),
		MakeMeta(nil, `The version info for Clojure core, as a map containing :major :minor
			:incremental and :qualifier keys. Feature releases may increment
			:minor and/or :major, bugfix releases will increment :incremental.`, "1.0"))
	res.args = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*command-line-args*"))
	res.classPath = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*classpath*"))
	res.classPath.Value = NIL
	res.classPath.isPrivate = true
	res.printReadably = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*print-readably*"))
	res.printReadably.Value = coretypes.Boolean{B: true}
	res.CoreNamespace.InternVar("*repl*", coretypes.Boolean{B: false},
		MakeMeta(nil, "true if Joker is running in repl mode", "1.5"))
	res.CoreNamespace.InternVar("*linter-mode*", coretypes.Boolean{B: LINTER_MODE},
		MakeMeta(nil, "true if Joker is running in linter mode", "1.0"))
	res.CoreNamespace.InternVar("*linter-config*", corecollections.EmptyArrayMap(),
		MakeMeta(nil, "Map of configuration key/value pairs for linter mode", "1.0"))
	res.libs = res.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*loaded-libs*"))
	res.libs.Value = corecollections.EmptySet()
	res.libs.isPrivate = true
	return res
}

func (env *Env) ReferCoreToUser() {
	env.FindNamespace(coretypes.MakeSymbol(STRINGS.Intern, "user")).ReferAll(env.CoreNamespace)
}
