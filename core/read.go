package core

import (
	"fmt"
	"io"
	"math/big"
	"math/rand"
	"regexp"
	"strconv"

	"github.com/rcarmo/go-joker/core/numutil"
	corereader "github.com/rcarmo/go-joker/core/reader"
)

type (
	ReadError struct {
		line     int
		column   int
		filename *string
		msg      string
	}
	ReadFunc func(reader *Reader) Object
)

const EOF = corereader.EOF

var (
	LINTER_MODE   bool = false
	FORMAT_MODE   bool = false
	PROBLEM_COUNT      = 0
	DIALECT       Dialect
	LINTER_CONFIG *Var
	SUPPRESS_READ bool = false
)

var (
	ARGS   map[int]Symbol
	GENSYM int
)

var NIL = Nil{}
var posStack = corereader.NewPositionStack(8)

func pushPos(reader *Reader) {
	posStack.Push(corereader.Position{Line: reader.line, Column: reader.column})
}

func popPos() corereader.Position {
	p, ok := posStack.Pop()
	if !ok {
		panic("reader position stack underflow")
	}
	return p
}

func MakeReadError(reader *Reader, msg string) ReadError {
	return ReadError{
		line:     reader.line,
		column:   reader.column,
		filename: reader.filename,
		msg:      msg,
	}
}

func MakeReadObject(reader *Reader, obj Object) Object {
	p := popPos()
	return obj.WithInfo(&ObjectInfo{Position: Position{
		startColumn: p.Column,
		startLine:   p.Line,
		endLine:     reader.line,
		endColumn:   reader.column,
		filename:    reader.filename,
	}})
}

func DeriveReadObject(base Object, obj Object) Object {
	baseInfo := base.GetInfo()
	if baseInfo != nil {
		bi := *baseInfo
		return obj.WithInfo(&bi)
	}
	return obj
}

func (err ReadError) Message() Object {
	return MakeString(err.msg)
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

func readSpecialCharacter(reader *Reader, ending string, r rune) Object {
	eatString(reader, ending)
	peekExpectedDelimiter(reader)
	return MakeReadObject(reader, Char{Ch: r})
}

func readComment(reader *Reader) Object {
	return MakeReadObject(reader, Comment{C: corereader.ReadCommentText(reader)})
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

func readUnicodeCharacter(reader *Reader, length, base int) Object {
	str := corereader.ScanUntilDelimiter(reader)
	r, ok := corereader.ParseExactUnicodeCode(str, length, base)
	if !ok {
		panic(MakeReadError(reader, "Invalid unicode character: \\o"+str))
	}
	peekExpectedDelimiter(reader)
	return MakeReadObject(reader, Char{Ch: r})
}

func readCharacter(reader *Reader) Object {
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
	return MakeReadObject(reader, Char{Ch: r})
}

func invalidNumberError(reader *Reader, str string) error {
	return MakeReadError(reader, fmt.Sprintf("Invalid number: %s", str))
}

func scanBigInt(orig, str string, base int, reader *Reader) Object {
	var bi = &big.Int{}
	if _, ok := bi.SetString(str, base); !ok {
		panic(invalidNumberError(reader, str))
	}
	res := BigInt{b: bi, Original: orig}
	return MakeReadObject(reader, &res)
}

func scanRatio(str string, reader *Reader) Object {
	var rat = &big.Rat{}
	if _, ok := rat.SetString(str); !ok {
		panic(invalidNumberError(reader, str))
	}
	return MakeReadObject(reader, ratioOrIntWithOriginal(str, rat))
}

func scanBigFloat(orig, str string, reader *Reader) Object {
	if f, ok := MakeBigFloatWithOrig(str, orig); ok {
		return MakeReadObject(reader, f)
	}
	panic(invalidNumberError(reader, str))
}

func scanInt(orig, str string, base int, reader *Reader) Object {
	i, e := numutil.ParseInt(str, base, strconv.IntSize)
	if e != nil {
		return scanBigInt(orig, str, base, reader)
	}
	return MakeReadObject(reader, Int{I: int(i)})
}

func scanFloat(str string, reader *Reader) Object {
	dbl, e := numutil.ParseFloat64(str)
	if e != nil {
		panic(invalidNumberError(reader, str))
	}
	return MakeReadObject(reader, Double{D: dbl})
}

func readNumber(reader *Reader) Object {
	str := corereader.ScanUntilDelimiter(reader)
	token, err := corereader.AnalyzeNumberToken(str)
	if err != nil {
		panic(invalidNumberError(reader, str))
	}
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

/* Reads (lexes) a token and returns either a Symbol or Keyword. */
func readIdent(reader *Reader, first rune) Object {
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
				return MakeReadObject(reader, MakeKeyword(str))
			}
			sym := MakeSymbol(str[1:])
			ns := GLOBAL_ENV.NamespaceFor(GLOBAL_ENV.CurrentNamespace(), sym)
			if ns == nil {
				msg := fmt.Sprintf("Unable to resolve namespace %s in keyword %s", *sym.ns, ":"+str)
				if LINTER_MODE {
					printReadWarning(reader, msg)
					return MakeReadObject(reader, MakeKeyword(*sym.name))
				}
				panic(MakeReadError(reader, msg))
			}
			ns.isUsed = true
			ns.isGloballyUsed = true
			return MakeReadObject(reader, MakeKeyword(*ns.Name.name+"/"+*sym.name))
		}
		return MakeReadObject(reader, MakeKeyword(str))
	default:
		switch corereader.ClassifyIdentLiteral(str) {
		case corereader.IdentLiteralNil:
			return MakeReadObject(reader, NIL)
		case corereader.IdentLiteralTrue:
			return MakeReadObject(reader, Boolean{B: true})
		case corereader.IdentLiteralFalse:
			return MakeReadObject(reader, Boolean{B: false})
		default:
			return MakeReadObject(reader, MakeSymbol(str))
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

func readValidatedIdent(reader *Reader, first rune) Object {
	obj := readIdent(reader, first)
	switch o := obj.(type) {
	case Keyword:
		warnInvalidIdent(reader, o.ns)
		if *o.name != "/" {
			warnInvalidIdent(reader, o.name)
		}
	case Symbol:
		warnInvalidIdent(reader, o.ns)
		warnInvalidIdent(reader, o.name)
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

func readRegex(reader *Reader) Object {
	s, ok := corereader.ScanRegexLiteral(reader)
	if !ok {
		panic(MakeReadError(reader, "Non-terminated regex literal"))
	}
	regex, err := regexp.Compile(s)
	if err != nil {
		switch corereader.ClassifyInvalidRegexAction(LINTER_MODE, FORMAT_MODE) {
		case corereader.InvalidRegexPlaceholder:
			return MakeReadObject(reader, &Regex{})
		case corereader.InvalidRegexPreserveString:
			res := MakeReadObject(reader, MakeString(s))
			addPrefix(res, "#")
			return res
		default:
			panic(MakeReadError(reader, "Invalid regex: "+err.Error()))
		}
	}
	return MakeReadObject(reader, &Regex{R: regex})
}

func readUnicodeCharacterInString(reader *Reader, initial rune, length, base int, exactLength bool) rune {
	str := corereader.ScanStringEscapeCode(reader, initial, length)
	r, err := corereader.DecodeStringEscapeCode(str, length, base, exactLength)
	if err != nil {
		panic(MakeReadError(reader, err.Error()))
	}
	return r
}

func readString(reader *Reader) Object {
	s, err := corereader.ScanStringLiteral(reader, FORMAT_MODE, func(initial rune, length, base int, exactLength bool) rune {
		return readUnicodeCharacterInString(reader, initial, length, base, exactLength)
	})
	if err != nil {
		panic(MakeReadError(reader, err.Error()))
	}
	return MakeReadObject(reader, String{S: s})
}

func readMulti(reader *Reader, previouslyRead []Object) (Object, []Object) {
	for len(previouslyRead) == 0 {
		obj, multi := readerConstruction.Read(reader)
		if !multi {
			return obj, previouslyRead
		}
		v := obj.(Vec)
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

func readCondList(reader *Reader) Object {
	previousSuppressRead := SUPPRESS_READ
	defer func() {
		SUPPRESS_READ = previousSuppressRead
	}()

	var forms []Object
	eatWhitespace(reader)
	r := reader.Peek()
	var res Object = nil
	for corereader.ContinueDelimitedForms(r, ')', len(forms)) {
		if res == nil {
			var feature Object
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

func readList(reader *Reader) Object {
	s := make([]Object, 0, 10)
	eatWhitespace(reader)
	r := reader.Peek()
	for r != ')' {
		obj, multi := readerConstruction.Read(reader)
		if multi {
			v := obj.(Vec)
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
	list := EmptyList
	for i := len(s) - 1; i >= 0; i-- {
		list = list.conj(s[i])
	}
	res := MakeReadObject(reader, list)
	return res
}

func readVector(reader *Reader) Object {
	res := collectionConstruction.EmptyArrayVector()
	eatWhitespace(reader)
	r := reader.Peek()
	for r != ']' {
		obj, multi := readerConstruction.Read(reader)
		if multi {
			v := obj.(Vec)
			for i := 0; i < v.Count(); i++ {
				res.Append(v.At(i))
			}
		} else {
			res.Append(obj)
		}
		eatWhitespace(reader)
		r = reader.Peek()
	}
	reader.Get()
	return MakeReadObject(reader, res)
}

func resolveKey(key Object, nsname string) Object {
	if nsname == "" {
		return key
	}
	switch key := key.(type) {
	case Keyword:
		if key.ns == nil {
			return DeriveReadObject(key, MakeKeyword(nsname+"/"+key.Name()))
		}
		if key.Namespace() == "_" {
			return DeriveReadObject(key, MakeKeyword(key.Name()))
		}
	case Symbol:
		if key.ns == nil {
			return DeriveReadObject(key, MakeSymbol(nsname+"/"+key.Name()))
		}
		if key.Namespace() == "_" {
			return DeriveReadObject(key, MakeSymbol(key.Name()))
		}
	}
	return key
}

func readMap(reader *Reader) Object {
	return readMapWithNamespace(reader, "")
}

func appendMapElement(objs []Object, obj Object) []Object {
	objs = append(objs, obj)
	if corereader.ShouldAppendMapCommentSurrogate(FORMAT_MODE, isComment(obj)) {
		// Add surrogate object to always have even number of elements in the map.
		// Use rand to avoid duplicate keys.
		objs = append(objs, MakeDouble(rand.Float64()))
	}
	return objs
}

func readMapWithNamespace(reader *Reader, nsname string) Object {
	eatWhitespace(reader)
	r := reader.Peek()
	objs := []Object{}
	for r != '}' {
		obj, multi := readerConstruction.Read(reader)
		if !multi {
			objs = appendMapElement(objs, obj)
		} else {
			v := obj.(Vec)
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
	if int64(len(objs)) >= HASHMAP_THRESHOLD {
		hashMap := collectionConstruction.HashMapFrom()
		for i := 0; i < len(objs); i += 2 {
			key := resolveKey(objs[i], nsname)
			if hashMap.containsKey(key) {
				panic(MakeReadError(reader, "Duplicate key "+key.ToString(false)))
			}
			hashMap = hashMap.Assoc(key, objs[i+1]).(*HashMap)
		}
		return MakeReadObject(reader, hashMap)
	}
	m := collectionConstruction.EmptyArrayMap()
	for i := 0; i < len(objs); i += 2 {
		key := resolveKey(objs[i], nsname)
		if !m.Add(key, objs[i+1]) {
			panic(MakeReadError(reader, "Duplicate key "+key.ToString(false)))
		}
	}
	return MakeReadObject(reader, m)
}

func readSet(reader *Reader) Object {
	set := collectionConstruction.EmptySet()
	eatWhitespace(reader)
	r := reader.Peek()
	for r != '}' {
		obj, multi := readerConstruction.Read(reader)
		if !multi {
			if !set.Add(obj) {
				panic(MakeReadError(reader, "Duplicate set element "+obj.ToString(false)))
			}
		} else {
			v := obj.(Vec)
			for i := 0; i < v.Count(); i++ {
				if !set.Add(v.At(i)) {
					panic(MakeReadError(reader, "Duplicate set element "+v.At(i).ToString(false)))
				}
			}
		}
		eatWhitespace(reader)
		r = reader.Peek()
	}
	reader.Get()
	return MakeReadObject(reader, set)
}

func makeQuote(obj Object, quote Symbol) Object {
	res := NewListFrom(quote, obj)
	return DeriveReadObject(obj, res)
}

func readMeta(reader *Reader) *ArrayMap {
	obj := readFirst(reader)
	switch v := obj.(type) {
	case *ArrayMap:
		return v
	case String, Symbol:
		return &ArrayMap{arr: []Object{DeriveReadObject(obj, KEYWORDS.tag), obj}}
	case Keyword:
		return &ArrayMap{arr: []Object{obj, DeriveReadObject(obj, Boolean{B: true})}}
	default:
		panic(MakeReadError(reader, "Metadata must be Symbol, Keyword, String or Map"))
	}
}

func fillInMissingArgs(args map[int]Symbol) {
	corereader.FillMissingArgIndexes(args, func() Symbol { return generateSymbol("p__") })
}

func makeFnForm(args map[int]Symbol, body Object) Object {
	fillInMissingArgs(args)
	a, ok := corereader.OrderedArgValues(args, SYMBOLS.amp)
	if !ok {
		panic(RT.NewError("Invalid arg literal index"))
	}
	argVector := collectionConstruction.EmptyVector()
	for _, v := range a {
		argVector = argVector.Conjoin(v)
	}
	if LINTER_MODE {
		if meta, ok := body.(Meta); ok {
			m := collectionConstruction.EmptyArrayMap().Plus(MakeKeyword("skip-redundant-do"), Boolean{B: true})
			body = meta.WithMeta(m)
		}
	}
	return DeriveReadObject(body, NewListFrom(MakeSymbol("joker.core/fn"), argVector, body))
}

func genSym(prefix string, postfix string) Symbol {
	GENSYM++
	return MakeSymbol(fmt.Sprintf("%s%d%s", prefix, GENSYM, postfix))
}

func generateSymbol(prefix string) Symbol {
	return genSym(prefix, "#")
}

func registerArg(index int) Symbol {
	if s, ok := ARGS[index]; ok {
		return s
	}
	ARGS[index] = generateSymbol("p__")
	return ARGS[index]
}

func readArgSymbol(reader *Reader) Object {
	r := reader.Peek()
	if corereader.IsBareArgLiteral(r) {
		return MakeReadObject(reader, registerArg(1))
	}
	obj := readFirst(reader)
	if obj.Equals(SYMBOLS.amp) {
		return MakeReadObject(reader, registerArg(-1))
	}
	switch n := obj.(type) {
	case Int:
		return MakeReadObject(reader, registerArg(n.I))
	default:
		panic(MakeReadError(reader, "Arg literal must be %, %& or %integer"))
	}
}

func isSelfEvaluating(obj Object) bool {
	if obj == EmptyList {
		return true
	}
	switch obj.(type) {
	case Boolean, Double, Int, Char, Keyword, String:
		return true
	default:
		return false
	}
}

func isCall(obj Object, name Symbol) bool {
	switch seq := obj.(type) {
	case Seq:
		return seq.First().Equals(name)
	default:
		return false
	}
}

func syntaxQuoteSeq(seq Seq, env map[*string]Symbol, reader *Reader) Seq {
	res := make([]Object, 0)
	for iter := iter(seq); iter.HasNext(); {
		obj := iter.Next()
		if isCall(obj, SYMBOLS.unquoteSplicing) {
			res = append(res, (obj).(Seq).Rest().First())
		} else {
			q := makeSyntaxQuote(obj, env, reader)
			res = append(res, DeriveReadObject(q, NewListFrom(SYMBOLS.list, q)))
		}
	}
	return &ArraySeq{arr: res}
}

func syntaxQuoteColl(seq Seq, env map[*string]Symbol, reader *Reader, ctor Symbol, info *ObjectInfo) Object {
	q := syntaxQuoteSeq(seq, env, reader)
	concat := q.Cons(SYMBOLS.concat)
	seqList := NewListFrom(SYMBOLS.seq, concat)
	var res Object = seqList
	if ctor != SYMBOLS.emptySymbol {
		res = NewListFrom(ctor, seqList).Cons(SYMBOLS.apply)
	}
	return res.WithInfo(info)
}

func makeSyntaxQuote(obj Object, env map[*string]Symbol, reader *Reader) Object {
	if isSelfEvaluating(obj) {
		return obj
	}
	if IsSpecialSymbol(obj) {
		return makeQuote(obj, SYMBOLS.quote)
	}
	info := obj.GetInfo()
	switch s := obj.(type) {
	case Symbol:
		str := *s.name
		if corereader.IsAutoGensymSymbolName(str, s.ns != nil) {
			sym, ok := env[s.name]
			if !ok {
				sym = generateSymbol(corereader.AutoGensymPrefix(str))
				env[s.name] = sym
			}
			obj = DeriveReadObject(obj, sym)
		} else {
			obj = DeriveReadObject(obj, GLOBAL_ENV.ResolveSymbol(s))
		}
		return makeQuote(obj, SYMBOLS.quote)
	case Seq:
		if isCall(obj, SYMBOLS.unquote) {
			return Second(s)
		}
		if isCall(obj, SYMBOLS.unquoteSplicing) {
			panic(MakeReadError(reader, "Splice not in list"))
		}
		return syntaxQuoteColl(s, env, reader, SYMBOLS.emptySymbol, info)
	case Vec:
		return syntaxQuoteColl(s.Seq(), env, reader, SYMBOLS.vector, info)
	case *ArrayMap:
		return syntaxQuoteColl(ArraySeqFromArrayMap(s), env, reader, SYMBOLS.hashMap, info)
	case *MapSet:
		return syntaxQuoteColl(s.Seq(), env, reader, SYMBOLS.hashSet, info)
	default:
		return obj
	}
}

func handleNoReaderError(reader *Reader, s Symbol) Object {
	return handleNoReaderErrorValue(reader, s, readFirst(reader))
}

func handleNoReaderErrorValue(reader *Reader, s Symbol, value Object) Object {
	msg := "No reader function for tag " + s.ToString(false)
	switch corereader.ClassifyMissingTaggedReaderAction(SUPPRESS_READ, LINTER_MODE, DIALECT == EDN) {
	case corereader.MissingTaggedReaderReturnValue:
		return value
	case corereader.MissingTaggedReaderWarnAndReturnValue:
		printReadWarning(reader, msg)
		return value
	default:
		panic(MakeReadError(reader, msg))
	}
}

func lookupDataReader(s Symbol) (Object, bool) {
	for _, name := range corereader.DataReaderVarNames() {
		vr := GLOBAL_ENV.CoreNamespace.Resolve(name)
		if vr == nil {
			continue
		}
		readersMap, ok := vr.Value.(Map)
		if !ok {
			continue
		}
		if ok, readFunc := readersMap.Get(s); ok {
			return readFunc, true
		}
	}
	return nil, false
}

func lookupDefaultDataReaderFn() (Callable, bool) {
	vr := GLOBAL_ENV.CoreNamespace.Resolve(corereader.DefaultDataReaderFnVarName())
	if vr == nil || vr.Value == nil || IsNil(vr.Value) {
		return nil, false
	}
	return EnsureObjectIsCallable(vr.Value, "*default-data-reader-fn* must be callable, got %s"), true
}

func readTagged(reader *Reader) Object {
	obj := readFirst(reader)
	if FORMAT_MODE {
		next := readFirst(reader)
		addPrefix(next, corereader.TaggedLiteralPrefix(obj.ToString(false)))
		return next
	}
	switch s := obj.(type) {
	case Symbol:
		value := readFirst(reader)
		if readFunc, ok := lookupDataReader(s); ok {
			return call1(EnsureObjectIsCallable(readFunc, "data reader must be callable, got %s"), value)
		}
		if fallback, ok := lookupDefaultDataReaderFn(); ok {
			return call2(fallback, s, value)
		}
		return handleNoReaderErrorValue(reader, s, value)
	default:
		panic(MakeReadError(reader, "Reader tag must be a symbol"))
	}
}

func readConditional(reader *Reader) (Object, bool) {
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
		cond := readList(reader).(*List)
		addPrefix(cond, corereader.ConditionalPrefix(isSplicing))
		return cond, false
	}
	v := readCondList(reader)
	s, seqable := v.(Seqable)
	switch corereader.ClassifyConditionalResult(v != nil, isSplicing, seqable) {
	case corereader.ConditionalResultEmptySplice:
		return collectionConstruction.EmptyVector(), true
	case corereader.ConditionalResultSpliceSeq:
		return DeriveReadObject(v, collectionConstruction.VectorFromSeq(s.Seq())), true
	case corereader.ConditionalResultSpliceError:
		readError(reader, "Spliced form in reader conditional must be Seqable, got "+v.GetType().ToString(false))
		return collectionConstruction.EmptyVector(), true
	default:
		return v, false
	}
}

func readNamespacedMap(reader *Reader) Object {
	auto := reader.Get() == ':'
	if !auto {
		reader.Unget()
	}
	var sym Object
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
			sym, ok := sym.(Symbol)
			if !ok || sym.ns != nil {
				panic(MakeReadError(reader, "Namespaced map must specify a valid namespace: "+sym.ToString(false)))
			}
			ns := GLOBAL_ENV.CurrentNamespace().aliases[sym.name]
			if ns == nil {
				ns = GLOBAL_ENV.Namespaces[sym.name]
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
		sym, ok := sym.(Symbol)
		if !ok || sym.ns != nil {
			panic(MakeReadError(reader, "Namespaced map must specify a valid namespace: "+sym.ToString(false)))
		}
		nsname = sym.Name()
	}
	return readMapWithNamespace(reader, nsname)
}

func readSymbolicValue(reader *Reader) Object {
	obj := readFirst(reader)
	switch o := obj.(type) {
	case Symbol:
		if v, found := corereader.SymbolicValue(o.ToString(false)); found {
			return Double{D: v}
		}
		panic(MakeReadError(reader, "Unknown symbolic value: ##"+o.ToString(false)))
	default:
		panic(MakeReadError(reader, "Invalid token: ##"+o.ToString(false)))
	}
}

func readDispatch(reader *Reader) (Object, bool) {
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
		return DeriveReadObject(nextObj, NewListFrom(DeriveReadObject(nextObj, SYMBOLS._var), nextObj)), false
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
		ARGS = make(map[int]Symbol)
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

func readWithMeta(reader *Reader) Object {
	meta := readMeta(reader)
	nextObj := readFirst(reader)
	switch v := nextObj.(type) {
	case Meta:
		return DeriveReadObject(nextObj, v.WithMeta(meta))
	default:
		panic(MakeReadError(reader, "Metadata cannot be applied to "+v.ToString(false)))
	}
}

func readFirst(reader *Reader) Object {
	obj, multi := readerConstruction.Read(reader)
	if !multi {
		return obj
	}
	v := obj.(Vec)
	if v.Count() == 0 {
		return readFirst(reader)
	}
	return v.At(0)
}

func addPrefix(obj Object, prefix string) {
	obj.GetInfo().prefix = prefix + obj.GetInfo().prefix
}

func Read(reader *Reader) (Object, bool) {
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
		return MakeReadObject(reader, Comment{C: ","}), false
	case corereader.TopLevelTriviaComment:
		reader.Unget()
		return readComment(reader), false
	}

	peek = 0
	if corereader.NeedsReadFormPeek(r) {
		peek = reader.Peek()
	}
	switch corereader.ClassifyReadForm(r, peek, ARGS != nil, FORMAT_MODE, DIALECT == CLJS) {
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
		return DeriveReadObject(nextObj, NewListFrom(DeriveReadObject(nextObj, SYMBOLS.deref), nextObj)), false
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
		return makeSyntaxQuote(nextObj, make(map[*string]Symbol), reader), false
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

func TryRead(reader *Reader) (obj Object, err error) {
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
