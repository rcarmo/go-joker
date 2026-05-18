//go:build gen_code
// +build gen_code

package core

import coretypes "github.com/rcarmo/go-joker/core/types"

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
