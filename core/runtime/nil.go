package runtime

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

type Nil struct {
	coretypes.InfoHolder
	N struct{}
}

func (n Nil) ToString(escape bool) string { return "nil" }
func (n Nil) Equals(other interface{}) bool {
	_, ok := other.(Nil)
	return ok
}
func (n Nil) GetType() *coretypes.Type { return coretypes.RuntimeTypes.Nil }
func (n Nil) Hash() uint32             { return 0 }
func (n Nil) Seq() coretypes.Seq       { return n }
func (n Nil) First() coretypes.Object  { return Nil{} }
func (n Nil) Rest() coretypes.Seq      { return Nil{} }
func (n Nil) IsEmpty() bool            { return true }
func (n Nil) Cons(obj coretypes.Object) coretypes.Seq {
	return corecollections.NewListFrom(obj)
}
func (n Nil) Conj(obj coretypes.Object) coretypes.Conjable {
	return corecollections.NewListFrom(obj)
}
func (n Nil) Without(key coretypes.Object) coretypes.Map { return n }
func (n Nil) Count() int                                 { return 0 }
func (n Nil) Iter() coretypes.MapIterator                { return coretypes.EmptyMapIteratorInstance }
func (n Nil) Merge(other coretypes.Map) coretypes.Map    { return other }
func (n Nil) Assoc(key, value coretypes.Object) coretypes.Associative {
	return corecollections.EmptyArrayMap().Assoc(key, value)
}
func (n Nil) EntryAt(key coretypes.Object) coretypes.Object { return nil }
func (n Nil) Get(key coretypes.Object) (bool, coretypes.Object) {
	return false, Nil{}
}
func (n Nil) Disjoin(key coretypes.Object) coretypes.Set { return n }
func (n Nil) Keys() coretypes.Seq                        { return Nil{} }
func (n Nil) Vals() coretypes.Seq                        { return Nil{} }
func (n Nil) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	n.Info = info
	return n
}
