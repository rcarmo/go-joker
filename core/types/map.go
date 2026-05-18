package types

import "errors"

func NewIteratorError() error {
	return errors.New("Iterator reached the end of collection")
}

type Map interface {
	Associative
	Seqable
	Counted
	Without(key Object) Map
	Keys() Seq
	Vals() Seq
	Merge(m Map) Map
	Iter() MapIterator
}

type MapIterator interface {
	HasNext() bool
	Next() *Pair
}

type EmptyMapIterator struct{}

var EmptyMapIteratorInstance MapIterator = &EmptyMapIterator{}

type Pair struct {
	Key   Object
	Value Object
}

func (iter *EmptyMapIterator) HasNext() bool { return false }

func (iter *EmptyMapIterator) Next() *Pair { panic("Iterator reached the end of collection") }

func SafeMerge(m1, m2 Map) Map {
	if m1 == nil {
		return m2
	}
	if m2 == nil {
		return m1
	}
	return m1.Merge(m2)
}
