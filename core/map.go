package core

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	corecollections "github.com/rcarmo/go-joker/core/collections"
)

type (
	Map interface {
		Associative
		Seqable
		coretypes.Counted
		Without(key Object) Map
		Keys() Seq
		Vals() Seq
		Merge(m Map) Map
		Iter() MapIterator
	}
	MapIterator interface {
		HasNext() bool
		Next() *Pair
	}
	EmptyMapIterator struct {
	}
	Pair struct {
		Key   Object
		Value Object
	}
)

var (
	emptyMapIterator = &EmptyMapIterator{}
)

func (iter *EmptyMapIterator) HasNext() bool {
	return false
}

func (iter *EmptyMapIterator) Next() *Pair {
	panic(newIteratorError())
}

func mapConj(m Map, obj Object) Conjable {
	switch obj := obj.(type) {
	case Vec:
		if obj.Count() != 2 {
			panic(RT.NewError("Vector argument to map's conj must be a vector with two elements"))
		}
		return m.Assoc(obj.At(0), obj.At(1))
	case Map:
		return m.Merge(obj)
	default:
		panic(RT.NewError("Argument to map's conj must be a vector with two elements or a map"))
	}
}

func mapEquals(m Map, other interface{}) bool {
	if m == other {
		return true
	}
	switch otherMap := other.(type) {
	case Nil:
		return false
	case Map:
		if m.Count() != otherMap.Count() {
			return false
		}
		// ArrayMap vs ArrayMap: use direct array scan (avoid generic Iter/Get).
		if am, mOK := m.(*ArrayMap); mOK {
			if amOther, otherOK := otherMap.(*ArrayMap); otherOK {
				return arrayMapEquals(am, amOther)
			}
		}
		// When other is ArrayMap, iterate it and do Get on m so we use O(n) HashMap.Get
		// instead of O(n²) ArrayMap.Get when m is HashMap.
		if am, ok := otherMap.(*ArrayMap); ok {
			for i := 0; i < len(am.arr); i += 2 {
				k, v := am.arr[i], am.arr[i+1]
				ok, val := m.Get(k)
				if !ok || !val.Equals(v) {
					return false
				}
			}
			return true
		}
		return corecollections.EqualMaps(
			m.Count(),
			otherMap.Count(),
			func(yield func(corecollections.Pair[Object, Object]) bool) {
				for iter := m.Iter(); iter.HasNext(); {
					p := iter.Next()
					if !yield(corecollections.Pair[Object, Object]{Key: p.Key, Value: p.Value}) {
						return
					}
				}
			},
			func(key Object) (Object, bool) {
				found, value := otherMap.Get(key)
				return value, found
			},
			func(a Object, b Object) bool {
				return b.Equals(a)
			},
		)
	default:
		return false
	}
}

func mapToString(m Map, escape bool) string {
	return corecollections.FormatPairDelimited(
		"{",
		"}",
		" ",
		", ",
		func(yield func(corecollections.Pair[Object, Object]) bool) {
			for iter := m.Iter(); iter.HasNext(); {
				p := iter.Next()
				if !yield(corecollections.Pair[Object, Object]{Key: p.Key, Value: p.Value}) {
					return
				}
			}
		},
		func(key Object) string { return key.ToString(escape) },
		func(value Object) string { return value.ToString(escape) },
	)
}

func callMap(m Map, args []Object) Object {
	CheckArity(args, 1, 2)
	if ok, v := m.Get(args[0]); ok {
		return v
	}
	if len(args) == 2 {
		return args[1]
	}
	return NIL
}

func pprintMap(m Map, w io.Writer, indent int) int {
	i := indent + 1
	fmt.Fprint(w, "{")
	if m.Count() > 0 {
		for iter := m.Iter(); ; {
			p := iter.Next()
			i = pprintObject(p.Key, indent+1, w)
			fmt.Fprint(w, " ")
			i = pprintObject(p.Value, i+1, w)
			if iter.HasNext() {
				fmt.Fprint(w, "\n")
				writeIndent(w, indent+1)
			} else {
				break
			}
		}
	}
	fmt.Fprint(w, "}")
	return i + 1
}
