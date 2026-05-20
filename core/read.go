package core

import (
	"fmt"
	"io"
	"math/big"
	"math/rand"
	"regexp"
	"strconv"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	corereader "github.com/rcarmo/go-joker/core/reader"
	"github.com/rcarmo/go-joker/core/types/numerical"
)

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
