//go:build gen_code
// +build gen_code

package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	corestr "github.com/rcarmo/go-joker/core/types/string"
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
		ArrayMap:       typeBuilder().RegisterReference("ArrayMap", (*ArrayMap)(nil), ""),
		ArrayMapSeq:    typeBuilder().RegisterReference("ArrayMapSeq", (*ArrayMapSeq)(nil), ""),
		ArrayNodeSeq:   typeBuilder().RegisterReference("ArrayNodeSeq", (*ArrayNodeSeq)(nil), ""),
		ArraySeq:       typeBuilder().RegisterReference("ArraySeq", (*ArraySeq)(nil), ""),
		MapSet:         typeBuilder().RegisterReference("MapSet", (*MapSet)(nil), ""),
		Atom:           typeBuilder().RegisterReference("Atom", (*Atom)(nil), ""),
		BigFloat:       typeBuilder().RegisterReference("coretypes.BigFloat", (*coretypes.BigFloat)(nil), "Wraps the Go 'math/big.Float' type"),
		BigInt:         typeBuilder().RegisterReference("coretypes.BigInt", (*coretypes.BigInt)(nil), "Wraps the Go 'math/big.Int' type"),
		Boolean:        typeBuilder().RegisterValue("Boolean", (*coretypes.Boolean)(nil), "Wraps the Go 'bool' type"),
		Time:           typeBuilder().RegisterValue("Time", (*coretypes.Time)(nil), "Wraps the Go 'time.Time' type"),
		Buffer:         typeBuilder().RegisterReference("Buffer", (*Buffer)(nil), ""),
		Char:           typeBuilder().RegisterValue("Char", (*coretypes.Char)(nil), "Wraps the Go 'rune' type"),
		ConsSeq:        typeBuilder().RegisterReference("ConsSeq", (*ConsSeq)(nil), ""),
		Delay:          typeBuilder().RegisterReference("coretypes.Delay", (*coretypes.Delay)(nil), ""),
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
		Keyword:       typeBuilder().RegisterValue("Keyword", (*coretypes.Keyword)(nil), "A possibly-namespace-qualified name prefixed by ':'"),
		LazySeq:       typeBuilder().RegisterReference("LazySeq", (*LazySeq)(nil), ""),
		List:          typeBuilder().RegisterReference("List", (*List)(nil), ""),
		MappingSeq:    typeBuilder().RegisterReference("MappingSeq", (*MappingSeq)(nil), ""),
		Namespace:     typeBuilder().RegisterReference("Namespace", (*Namespace)(nil), ""),
		Nil:           typeBuilder().RegisterValue("Nil", (*Nil)(nil), "The 'nil' value"),
		NodeSeq:       typeBuilder().RegisterReference("NodeSeq", (*NodeSeq)(nil), ""),
		ParseError:    typeBuilder().RegisterReference("ParseError", (*ParseError)(nil), ""),
		Proc:          typeBuilder().RegisterReference("Proc", (*Proc)(nil), "A callable function implemented via Go code"),
		Ratio:         typeBuilder().RegisterReference("coretypes.Ratio", (*coretypes.Ratio)(nil), "Wraps the Go 'math.big/Rat' type"),
		RecurBindings: typeBuilder().RegisterReference("RecurBindings", (*coretypes.RecurBindings)(nil), ""),
		Regex:         typeBuilder().RegisterReference("Regex", (*coretypes.Regex)(nil), "Wraps the Go 'regexp.Regexp' type"),
		String:        typeBuilder().RegisterValue("String", (*coretypes.String)(nil), "Wraps the Go 'string' type"),
		Symbol:        typeBuilder().RegisterValue("Symbol", (*coretypes.Symbol)(nil), ""),
		Type:          typeBuilder().RegisterReference("Type", (*coretypes.Type)(nil), ""),
		Var:           typeBuilder().RegisterReference("Var", (*Var)(nil), ""),
		Vector:        typeBuilder().RegisterReference("Vector", (*Vector)(nil), ""),
		Vec:           typeBuilder().RegisterInterface("Vec", (*coretypes.Vec)(nil), ""),
		ArrayVector:   typeBuilder().RegisterReference("ArrayVector", (*ArrayVector)(nil), ""),
		VectorRSeq:    typeBuilder().RegisterReference("VectorRSeq", (*VectorRSeq)(nil), ""),
		VectorSeq:     typeBuilder().RegisterReference("VectorSeq", (*VectorSeq)(nil), ""),
		StringSeq:     typeBuilder().RegisterReference("StringSeq", (*stringSeq)(nil), ""),
	}
	coretypes.RuntimeTypes = &TYPE
	coretypes.RuntimeNil = NIL
	coretypes.RuntimeReduceType = TYPE.Reduce
	coretypes.RuntimeKVReduceType = TYPE.KVReduce
	coretypes.SpecialSymbolLookup = func(sym coretypes.Symbol) bool { return SPECIAL_SYMBOLS[sym.NameKey()] }
	coretypes.NumberCompare = coretypes.CompareNumbers
	coretypes.NumberEquals = equalsNumbers
	coretypes.NamedLookup = getMap
	coretypes.TransientMutationError = func() any { return RT.NewError("Cannot mutate a frozen transient") }
	coretypes.TransientVectorIndexTypeError = func(obj coretypes.Object) any { return RT.NewArgTypeError(1, obj, "Int") }
	coretypes.TransientVectorToPersistent = func(arr []coretypes.Object) coretypes.Object { return &ArrayVector{arr: arr} }
	coretypes.TransientMapToPersistent = func(tm *coretypes.TransientMap) coretypes.Object {
		if tm.CountN <= int(HASHMAP_THRESHOLD/2) {
			res := collectionConstruction.NewEmptyArrayMap()
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
		res := EmptyHashMap
		for k, v := range tm.SM {
			res = res.Assoc(coretypes.String{S: k}, v).(*HashMap)
		}
		for _, bucket := range tm.M {
			for _, e := range bucket {
				res = res.Assoc(e.Key, e.Val).(*HashMap)
			}
		}
		return res
	}
	installAssertionErrors()
	coretypes.DelayCall = call0
}
