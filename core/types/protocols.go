package types

import "io"

type Equality interface {
	Equals(interface{}) bool
}

type Counted interface {
	Count() int
}

type Comparable interface {
	Compare(other Object) int
}

type Comparator interface {
	Compare(a, b Object) int
}

type Named interface {
	Name() string
	Namespace() string
}

type Printer interface {
	Print(writer io.Writer, printReadably bool)
}

type Pprinter interface {
	Pprint(writer io.Writer, indent int) int
}

type Formatter interface {
	Format(writer io.Writer, indent int) int
}

type Native interface {
	Native() interface{}
}

type Pending interface {
	IsRealized() bool
}

type StringReader interface {
	ReadString(delim byte) (s string, e error)
}

type Error interface {
	error
	Object
	Message() Object
}

var RuntimeNil Object
var RuntimeError func(string) any
var RuntimePanicArityMinMax func(n, min, max int)
var RuntimePprintObject func(obj Object, indent int, w io.Writer) int
var RuntimeFormatObject func(obj Object, indent int, w io.Writer) int
var RuntimeMaybeNewLine func(w io.Writer, obj, nextObj Object, baseIndent, currentIndent int) int
var RuntimeWriteIndent func(w io.Writer, n int)
var RuntimeIsComment func(obj Object) bool
var RuntimeIsReduced func(obj Object) bool
var RuntimeDerefReduced func(obj Object) Object
var RuntimeReduceType *Type
var RuntimeKVReduceType *Type
var SpecialSymbolLookup func(Symbol) bool

func IsInstance(t *Type, obj Object) bool {
	if RuntimeNil != nil && obj.Equals(RuntimeNil) {
		return false
	}
	if RuntimeReduceType != nil && t == RuntimeReduceType {
		_, ok := obj.(Reduce)
		return ok
	}
	if RuntimeKVReduceType != nil && t == RuntimeKVReduceType {
		_, ok := obj.(KVReduce)
		return ok
	}
	return IsEqualOrImplements(t, obj.GetType())
}

func IsSpecialSymbol(obj Object) bool {
	sym, ok := obj.(Symbol)
	if !ok || sym.NamespaceKey() != nil {
		return false
	}
	if SpecialSymbolLookup == nil {
		return false
	}
	return SpecialSymbolLookup(sym)
}

func SeqsEqual(seq1, seq2 Seq) bool {
	a := seq1
	b := seq2
	for {
		aEmpty := a == nil || a.IsEmpty()
		bEmpty := b == nil || b.IsEmpty()
		if aEmpty || bEmpty {
			return aEmpty == bEmpty
		}
		if !a.First().Equals(b.First()) {
			return false
		}
		a = a.Rest()
		b = b.Rest()
	}
}

func IsSeqEqual(seq Seq, other interface{}) bool {
	if seq == other {
		return true
	}
	if sequential, ok := other.(Sequential); ok {
		if seqable, ok := sequential.(Seqable); ok {
			return SeqsEqual(seq, seqable.Seq())
		}
	}
	return false
}

type Deref interface {
	Deref() Object
}

type Callable interface {
	Call(args []Object) Object
}

type Conjable interface {
	Object
	Conj(obj Object) Conjable
}

type CountedIndexed interface {
	Counted
	At(int) Object
}

type Indexed interface {
	Nth(i int) Object
	TryNth(i int, d Object) Object
}

type Stack interface {
	Peek() Object
	Pop() Stack
}

type Gettable interface {
	Get(key Object) (bool, Object)
}

type Reversible interface {
	Rseq() Seq
}

type Collection interface {
	Object
	Counted
	Seqable
	Empty() Collection
}

type Associative interface {
	Conjable
	Gettable
	EntryAt(key Object) Object
	Assoc(key, val Object) Associative
}

type KVReduce interface {
	KVReduce(c Callable, init Object) Object
}

type Reduce interface {
	ReduceInit(c Callable, init Object) Object
	Reduce(c Callable) Object
}

type Seqable interface {
	Seq() Seq
}

type Seq interface {
	Seqable
	Object
	First() Object
	Rest() Seq
	IsEmpty() bool
	Cons(obj Object) Seq
}

type Sequential interface {
	SequentialMarker()
}
