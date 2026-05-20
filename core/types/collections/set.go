package collections

import (
	"io"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type (
	MapSet struct {
		coretypes.InfoHolder
		coretypes.MetaHolder
		M coretypes.Map
	}
)

func (v *MapSet) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	v.Info = info
	return v
}

func (v *MapSet) WithMeta(meta coretypes.Map) coretypes.Object {
	res := *v
	res.Meta = coretypes.SafeMerge(res.Meta, meta)
	return &res
}

func (set *MapSet) Disjoin(key coretypes.Object) coretypes.Set {
	return &MapSet{InfoHolder: set.InfoHolder, MetaHolder: set.MetaHolder, M: set.M.Without(key)}
}

func (set *MapSet) ensureMap() coretypes.Map {
	set.M = EnsureSetMap(set.M, func() coretypes.Map { return EmptyArrayMap() })
	return set.M
}

func (set *MapSet) Add(obj coretypes.Object) bool {
	switch m := set.ensureMap().(type) {
	case *ArrayMap:
		return m.Add(obj, coretypes.Boolean{B: true})
	case *HashMap:
		next, added := SetAddViaMap(set.M, obj, func(current coretypes.Map, key coretypes.Object) bool {
			hm, ok := current.(*HashMap)
			return ok && hm.ContainsKey(key)
		})
		set.M = next
		return added
	default:
		return false
	}
}

func (set *MapSet) Conj(obj coretypes.Object) coretypes.Conjable {
	return &MapSet{InfoHolder: set.InfoHolder, MetaHolder: set.MetaHolder, M: set.ensureMap().Assoc(obj, coretypes.Boolean{B: true}).(coretypes.Map)}
}

func EmptySet() *MapSet {
	return &MapSet{M: EmptyArrayMap()}
}

func (set *MapSet) ToString(escape bool) string {
	return FormatDelimited("#{", "}", " ", func(yield func(string) bool) {
		for iter := NewSeqIterator(set.M.Keys()); iter.HasNext(); {
			if !yield(iter.Next().ToString(escape)) {
				return
			}
		}
	})
}

func (set *MapSet) Equals(other interface{}) bool {
	switch otherSet := other.(type) {
	case *MapSet:
		return set.M.Equals(otherSet.M)
	default:
		return false
	}
}

func (set *MapSet) Get(key coretypes.Object) (bool, coretypes.Object) {
	return SetGet(set.M, key)
}

func (seq *MapSet) GetType() *coretypes.Type {
	return coretypes.RuntimeTypes.MapSet
}

func (set *MapSet) Hash() uint32 {
	return HashUnordered(set.Seq(), 2)
}

func (set *MapSet) Seq() coretypes.Seq {
	return SetSeq(set.M, EmptyList)
}

func (set *MapSet) Count() int {
	return SetCount(set.M)
}

func (set *MapSet) Call(args []coretypes.Object) coretypes.Object {
	if len(args) != 1 {
		coretypes.RuntimePanicArityMinMax(len(args), 1, 1)
	}
	if ok, _ := set.Get(args[0]); ok {
		return args[0]
	}
	return coretypes.RuntimeNil
}

func (set *MapSet) Empty() coretypes.Collection {
	return EmptySet()
}

func NewSetFromSeq(s coretypes.Seq) *MapSet {
	return SetFromSeq(s, EmptySet(), func(set *MapSet, obj coretypes.Object) *MapSet {
		set.Add(obj)
		return set
	})
}

func (set *MapSet) Pprint(w io.Writer, indent int) int {
	return SetPprint(set.M.Keys(), w, indent, coretypes.RuntimePprintObject, coretypes.RuntimeWriteIndent)
}

func (set *MapSet) Format(w io.Writer, indent int) int {
	return SetFormat(set.M.Keys(), w, indent, coretypes.RuntimeFormatObject, coretypes.RuntimeMaybeNewLine, coretypes.RuntimeIsComment, coretypes.RuntimeWriteIndent)
}
