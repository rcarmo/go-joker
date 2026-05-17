//go:build gen_code
// +build gen_code

package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	corestr "github.com/rcarmo/go-joker/core/string"
)

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
			meta.Add(KEYWORDS.name, MakeString(name))
			return MetaHolder{meta}
		},
	}
}

func init() {
	TYPE = coretypes.Types{
		Associative:    typeBuilder().RegisterInterface("Associative", (*Associative)(nil), ""),
		Callable:       typeBuilder().RegisterInterface("Callable", (*Callable)(nil), ""),
		Collection:     typeBuilder().RegisterInterface("Collection", (*Collection)(nil), ""),
		Comparable:     typeBuilder().RegisterInterface("Comparable", (*coretypes.Comparable)(nil), ""),
		Comparator:     typeBuilder().RegisterInterface("Comparator", (*coretypes.Comparator)(nil), ""),
		Counted:        typeBuilder().RegisterInterface("Counted", (*coretypes.Counted)(nil), ""),
		CountedIndexed: typeBuilder().RegisterInterface("CountedIndexed", (*CountedIndexed)(nil), ""),
		Deref:          typeBuilder().RegisterInterface("Deref", (*Deref)(nil), ""),
		Error:          typeBuilder().RegisterInterface("Error", (*Error)(nil), ""),
		Gettable:       typeBuilder().RegisterInterface("Gettable", (*Gettable)(nil), ""),
		Indexed:        typeBuilder().RegisterInterface("Indexed", (*Indexed)(nil), ""),
		IOReader:       typeBuilder().RegisterInterface("IOReader", (*io.Reader)(nil), ""),
		IOWriter:       typeBuilder().RegisterInterface("IOWriter", (*io.Writer)(nil), ""),
		KVReduce:       typeBuilder().RegisterInterface("KVReduce", (*KVReduce)(nil), ""),
		Reduce:         typeBuilder().RegisterInterface("Reduce", (*Reduce)(nil), ""),
		Map:            typeBuilder().RegisterInterface("Map", (*Map)(nil), ""),
		Meta:           typeBuilder().RegisterInterface("Meta", (*Meta)(nil), ""),
		Named:          typeBuilder().RegisterInterface("Named", (*coretypes.Named)(nil), ""),
		Number:         typeBuilder().RegisterInterface("Number", (*coretypes.Number)(nil), ""),
		Pending:        typeBuilder().RegisterInterface("Pending", (*coretypes.Pending)(nil), ""),
		Ref:            typeBuilder().RegisterInterface("Ref", (*Ref)(nil), ""),
		Reversible:     typeBuilder().RegisterInterface("Reversible", (*Reversible)(nil), ""),
		Seq:            typeBuilder().RegisterInterface("Seq", (*Seq)(nil), ""),
		Seqable:        typeBuilder().RegisterInterface("Seqable", (*Seqable)(nil), ""),
		Sequential:     typeBuilder().RegisterInterface("Sequential", (*coretypes.Sequential)(nil), ""),
		Set:            typeBuilder().RegisterInterface("Set", (*Set)(nil), ""),
		Stack:          typeBuilder().RegisterInterface("Stack", (*Stack)(nil), ""),
		ArrayMap:       typeBuilder().RegisterReference("ArrayMap", (*ArrayMap)(nil), ""),
		ArrayMapSeq:    typeBuilder().RegisterReference("ArrayMapSeq", (*ArrayMapSeq)(nil), ""),
		ArrayNodeSeq:   typeBuilder().RegisterReference("ArrayNodeSeq", (*ArrayNodeSeq)(nil), ""),
		ArraySeq:       typeBuilder().RegisterReference("ArraySeq", (*ArraySeq)(nil), ""),
		MapSet:         typeBuilder().RegisterReference("MapSet", (*MapSet)(nil), ""),
		Atom:           typeBuilder().RegisterReference("Atom", (*Atom)(nil), ""),
		BigFloat:       typeBuilder().RegisterReference("BigFloat", (*BigFloat)(nil), "Wraps the Go 'math/big.Float' type"),
		BigInt:         typeBuilder().RegisterReference("BigInt", (*BigInt)(nil), "Wraps the Go 'math/big.Int' type"),
		Boolean:        typeBuilder().RegisterValue("Boolean", (*coretypes.Boolean)(nil), "Wraps the Go 'bool' type"),
		Time:           typeBuilder().RegisterValue("Time", (*coretypes.Time)(nil), "Wraps the Go 'time.Time' type"),
		Buffer:         typeBuilder().RegisterReference("Buffer", (*Buffer)(nil), ""),
		Char:           typeBuilder().RegisterValue("Char", (*coretypes.Char)(nil), "Wraps the Go 'rune' type"),
		ConsSeq:        typeBuilder().RegisterReference("ConsSeq", (*ConsSeq)(nil), ""),
		Delay:          typeBuilder().RegisterReference("Delay", (*Delay)(nil), ""),
		Channel:        typeBuilder().RegisterReference("Channel", (*Channel)(nil), ""),
		Double:         typeBuilder().RegisterValue("Double", (*coretypes.Double)(nil), "Wraps the Go 'float64' type"),
		EvalError:      typeBuilder().RegisterReference("EvalError", (*EvalError)(nil), ""),
		ExInfo:         typeBuilder().RegisterReference("ExInfo", (*ExInfo)(nil), ""),
		Fn:             typeBuilder().RegisterReference("Fn", (*Fn)(nil), "A callable function or macro implemented via Joker code"),
		File:           typeBuilder().RegisterReference("File", (*File)(nil), ""),
		BufferedReader: typeBuilder().RegisterReference("BufferedReader", (*BufferedReader)(nil), ""),
		HashMap:        typeBuilder().RegisterReference("HashMap", (*HashMap)(nil), ""),
		Int: typeBuilder().RegisterValue("Int", (*coretypes.Int)(nil),
			"Wraps the Go 'int' type, which is 32 bits wide on 32-bit hosts, 64 bits wide on 64-bit hosts, etc."),
		Keyword:       typeBuilder().RegisterValue("Keyword", (*Keyword)(nil), "A possibly-namespace-qualified name prefixed by ':'"),
		LazySeq:       typeBuilder().RegisterReference("LazySeq", (*LazySeq)(nil), ""),
		List:          typeBuilder().RegisterReference("List", (*List)(nil), ""),
		MappingSeq:    typeBuilder().RegisterReference("MappingSeq", (*MappingSeq)(nil), ""),
		Namespace:     typeBuilder().RegisterReference("Namespace", (*Namespace)(nil), ""),
		Nil:           typeBuilder().RegisterValue("Nil", (*Nil)(nil), "The 'nil' value"),
		NodeSeq:       typeBuilder().RegisterReference("NodeSeq", (*NodeSeq)(nil), ""),
		ParseError:    typeBuilder().RegisterReference("ParseError", (*ParseError)(nil), ""),
		Proc:          typeBuilder().RegisterReference("Proc", (*Proc)(nil), "A callable function implemented via Go code"),
		Ratio:         typeBuilder().RegisterReference("Ratio", (*Ratio)(nil), "Wraps the Go 'math.big/Rat' type"),
		RecurBindings: typeBuilder().RegisterReference("RecurBindings", (*RecurBindings)(nil), ""),
		Regex:         typeBuilder().RegisterReference("Regex", (*coretypes.Regex)(nil), "Wraps the Go 'regexp.Regexp' type"),
		String:        typeBuilder().RegisterValue("String", (*String)(nil), "Wraps the Go 'string' type"),
		Symbol:        typeBuilder().RegisterValue("Symbol", (*Symbol)(nil), ""),
		Type:          typeBuilder().RegisterReference("Type", (*coretypes.Type)(nil), ""),
		Var:           typeBuilder().RegisterReference("Var", (*Var)(nil), ""),
		Vector:        typeBuilder().RegisterReference("Vector", (*Vector)(nil), ""),
		Vec:           typeBuilder().RegisterInterface("Vec", (*Vec)(nil), ""),
		ArrayVector:   typeBuilder().RegisterReference("ArrayVector", (*ArrayVector)(nil), ""),
		VectorRSeq:    typeBuilder().RegisterReference("VectorRSeq", (*VectorRSeq)(nil), ""),
		VectorSeq:     typeBuilder().RegisterReference("VectorSeq", (*VectorSeq)(nil), ""),
		StringSeq:     typeBuilder().RegisterReference("StringSeq", (*stringSeq)(nil), ""),
	}
	coretypes.RuntimeTypes = &TYPE
	coretypes.NumberCompare = CompareNumbers
	coretypes.NumberEquals = equalsNumbers
}
