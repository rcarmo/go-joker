package core

import coretypes "github.com/rcarmo/go-joker/core/types"

// reduced.go — Proper Reduced type for transducer early termination.
//
// In Clojure, (reduced x) wraps x in a Reduced box that signals
// early termination to reduce/transduce. This replaces the ArrayMap-based
// shim with a proper type that's fast to create, check, and unwrap.

// Reduced wraps a value to signal early termination in reduce/transduce.
type Reduced struct {
	coretypes.InfoHolder
	MetaHolder
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

func (r *Reduced) GetType() *coretypes.Type {
	return TYPE.Fn // reuse Fn type slot for now
}

func (r *Reduced) Hash() uint32 {
	return r.Val.Hash() ^ 0xDEADBEEF
}

func (r *Reduced) WithInfo(info *coretypes.ObjectInfo) coretypes.Object {
	res := *r
	res.Info = info
	return &res
}

func (r *Reduced) WithMeta(m Map) coretypes.Object {
	res := *r
	res.meta = SafeMerge(res.meta, m)
	return &res
}

// MakeReduced wraps a value in a Reduced box.
func MakeReduced(val coretypes.Object) *Reduced {
	return &Reduced{Val: val}
}

// IsReduced checks if an object is a Reduced box (type assertion, no map lookup).
func IsReduced(obj coretypes.Object) bool {
	_, ok := obj.(*Reduced)
	return ok
}

// DerefReduced unwraps a Reduced box, returning the inner value.
// If not reduced, returns the value as-is.
func DerefReduced(obj coretypes.Object) coretypes.Object {
	if r, ok := obj.(*Reduced); ok {
		return r.Val
	}
	return obj
}

// EnsureReduced wraps a value in Reduced if it isn't already.
func EnsureReduced(obj coretypes.Object) *Reduced {
	if r, ok := obj.(*Reduced); ok {
		return r
	}
	return MakeReduced(obj)
}
