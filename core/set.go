package core

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	corecollections "github.com/rcarmo/go-joker/core/collections"
)

type (
	Set interface {
		Conjable
		Gettable
		Disjoin(key Object) Set
	}
	MapSet struct {
		coretypes.InfoHolder
		MetaHolder
		m Map
	}
)

func (v *MapSet) WithMeta(meta Map) Object {
	res := *v
	res.meta = SafeMerge(res.meta, meta)
	return &res
}

func (set *MapSet) Disjoin(key Object) Set {
	return &MapSet{InfoHolder: set.InfoHolder, MetaHolder: set.MetaHolder, m: set.m.Without(key)}
}

func (set *MapSet) ensureMap() Map {
	if set.m == nil {
		set.m = EmptyArrayMap()
	}
	return set.m
}

func (set *MapSet) Add(obj Object) bool {
	switch m := set.ensureMap().(type) {
	case *ArrayMap:
		return m.Add(obj, coretypes.Boolean{B: true})
	case *HashMap:
		if m.containsKey(obj) {
			return false
		}
		set.m = set.m.Assoc(obj, coretypes.Boolean{B: true}).(Map)
		return true
	default:
		return false
	}
}

func (set *MapSet) Conj(obj Object) Conjable {
	return &MapSet{InfoHolder: set.InfoHolder, MetaHolder: set.MetaHolder, m: set.ensureMap().Assoc(obj, coretypes.Boolean{B: true}).(Map)}
}

func EmptySet() *MapSet {
	return &MapSet{m: EmptyArrayMap()}
}

func (set *MapSet) ToString(escape bool) string {
	return corecollections.FormatDelimited("#{", "}", " ", func(yield func(string) bool) {
		for iter := iter(set.m.Keys()); iter.HasNext(); {
			if !yield(iter.Next().ToString(escape)) {
				return
			}
		}
	})
}

func (set *MapSet) Equals(other interface{}) bool {
	switch otherSet := other.(type) {
	case *MapSet:
		return set.m.Equals(otherSet.m)
	default:
		return false
	}
}

func (set *MapSet) Get(key Object) (bool, Object) {
	if set.m == nil {
		return false, nil
	}
	if ok, _ := set.m.Get(key); ok {
		return true, key
	}
	return false, nil
}

func (seq *MapSet) GetType() *coretypes.Type {
	return TYPE.MapSet
}

func (set *MapSet) Hash() uint32 {
	return hashUnordered(set.Seq(), 2)
}

func (set *MapSet) Seq() Seq {
	if set.m == nil {
		return EmptyList
	}
	return set.m.Keys()
}

func (set *MapSet) Count() int {
	if set.m == nil {
		return 0
	}
	return set.m.Count()
}

func (set *MapSet) Call(args []Object) Object {
	CheckArity(args, 1, 1)
	if ok, _ := set.Get(args[0]); ok {
		return args[0]
	}
	return NIL
}

func (set *MapSet) Empty() Collection {
	return collectionConstruction.NewEmptySet()
}

func NewSetFromSeq(s Seq) *MapSet {
	res := EmptySet()
	for !s.IsEmpty() {
		res.Add(s.First())
		s = s.Rest()
	}
	return res
}

func (set *MapSet) Pprint(w io.Writer, indent int) int {
	i := indent + 1
	fmt.Fprint(w, "#{")
	for iter := iter(set.m.Keys()); iter.HasNext(); {
		i = pprintObject(iter.Next(), indent+2, w)
		if iter.HasNext() {
			fmt.Fprint(w, "\n")
			writeIndent(w, indent+2)
		}
	}
	fmt.Fprint(w, "}")
	return i + 1
}

func (set *MapSet) Format(w io.Writer, indent int) int {
	i := indent + 2
	fmt.Fprint(w, "#{")
	var prevObj Object
	for iter := iter(set.m.Keys()); iter.HasNext(); {
		obj := iter.Next()
		if prevObj != nil {
			i = maybeNewLine(w, prevObj, obj, indent+2, i)
		}
		i = formatObject(obj, i, w)
		prevObj = obj
	}
	if prevObj != nil {
		if isComment(prevObj) {
			fmt.Fprint(w, "\n")
			writeIndent(w, indent+2)
			i = indent + 2
		}
	}
	fmt.Fprint(w, "}")
	return i + 1
}
