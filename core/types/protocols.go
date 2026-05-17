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

type KVReduce interface {
	KVReduce(c Callable, init Object) Object
}

type Reduce interface {
	ReduceInit(c Callable, init Object) Object
	Reduce(c Callable) Object
}

type Sequential interface {
	SequentialMarker()
}
