package runtime

import coretypes "github.com/rcarmo/go-joker/core/types"

// Reduced wraps a value to signal early termination in reduce/transduce.
type Reduced struct {
	coretypes.InfoHolder
	coretypes.MetaHolder
	Val coretypes.Object
}

func (r *Reduced) ToString(escape bool) string {
	return "#object[Reduced " + r.Val.ToString(escape) + "]"
}

func (r *Reduced) Equals(other interface{}) bool {
	if o, ok := other.(*Reduced); ok {
		return r.Val.Equals(o.Val)
	}
	return false
}

func (r *Reduced) GetType() *coretypes.Type { return coretypes.RuntimeTypes.Fn }
func (r *Reduced) Hash() uint32             { return r.Val.Hash() ^ 0xDEADBEEF }

func (r *Reduced) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *r
	res.Info = info
	return &res
}

func (r *Reduced) WithMeta(m coretypes.Map) coretypes.Object {
	res := *r
	res.Meta = coretypes.SafeMerge(res.Meta, m)
	return &res
}

func MakeReduced(val coretypes.Object) *Reduced { return &Reduced{Val: val} }

func IsReduced(obj coretypes.Object) bool {
	_, ok := obj.(*Reduced)
	return ok
}

func DerefReduced(obj coretypes.Object) coretypes.Object {
	if r, ok := obj.(*Reduced); ok {
		return r.Val
	}
	return obj
}

func EnsureReduced(obj coretypes.Object) *Reduced {
	if r, ok := obj.(*Reduced); ok {
		return r
	}
	return MakeReduced(obj)
}
