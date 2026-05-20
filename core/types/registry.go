package types

type Registry map[*string]*Type

func (r Registry) Register(name *string, typ *Type) *Type {
	r[name] = typ
	return typ
}

func (r Registry) Lookup(name *string) *Type {
	return r[name]
}

func (r Registry) Contains(name *string) bool {
	return r.Lookup(name) != nil
}

var RuntimeTypes *Types

type Types struct {
	Associative    *Type
	Callable       *Type
	Collection     *Type
	Comparable     *Type
	Comparator     *Type
	Counted        *Type
	CountedIndexed *Type
	Deref          *Type
	Channel        *Type
	Error          *Type
	Gettable       *Type
	Indexed        *Type
	IOReader       *Type
	IOWriter       *Type
	KVReduce       *Type
	Reduce         *Type
	Map            *Type
	Meta           *Type
	Named          *Type
	Number         *Type
	Pending        *Type
	Ref            *Type
	Reversible     *Type
	Seq            *Type
	Seqable        *Type
	Sequential     *Type
	Set            *Type
	Stack          *Type
	ArrayMap       *Type
	ArrayMapSeq    *Type
	ArrayNodeSeq   *Type
	ArraySeq       *Type
	MapSet         *Type
	Atom           *Type
	BigFloat       *Type
	BigInt         *Type
	Boolean        *Type
	Time           *Type
	Buffer         *Type
	Char           *Type
	ConsSeq        *Type
	Delay          *Type
	Double         *Type
	EvalError      *Type
	ExInfo         *Type
	Fn             *Type
	File           *Type
	BufferedReader *Type
	HashMap        *Type
	Int            *Type
	Keyword        *Type
	LazySeq        *Type
	List           *Type
	MappingSeq     *Type
	Namespace      *Type
	Nil            *Type
	NodeSeq        *Type
	ParseError     *Type
	Proc           *Type
	ProcFn         *Type
	Ratio          *Type
	RecurBindings  *Type
	Regex          *Type
	String         *Type
	Symbol         *Type
	Type           *Type
	Var            *Type
	Vector         *Type
	Vec            *Type
	ArrayVector    *Type
	VectorRSeq     *Type
	VectorSeq      *Type
	StringSeq      *Type
}
