package runtime

import (
	"sync"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

// Atom holds synchronous mutable state with Joker object/ref protocols.
type Atom struct {
	coretypes.MetaHolder
	mu    sync.Mutex
	value coretypes.Object
}

func NewAtom(value coretypes.Object, meta coretypes.Map) *Atom {
	res := &Atom{value: value}
	if meta != nil {
		res.Meta = meta
	}
	return res
}

func (a *Atom) ToString(escape bool) string {
	return "#object[Atom {:val " + a.value.ToString(escape) + "}]"
}
func (a *Atom) Equals(other interface{}) bool                        { return a == other }
func (a *Atom) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (a *Atom) GetType() *coretypes.Type                             { return coretypes.RuntimeTypes.Atom }
func (a *Atom) Hash() uint32                                         { return hashutil.Ptr(uintptr(unsafe.Pointer(a))) }
func (a *Atom) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return a }

func (a *Atom) WithMeta(meta coretypes.Map) coretypes.Object {
	res := &Atom{value: a.value}
	res.Meta = coretypes.SafeMerge(a.Meta, meta)
	return res
}
func (a *Atom) ResetMeta(newMeta coretypes.Map) coretypes.Map {
	a.Meta = newMeta
	return a.Meta
}
func (a *Atom) AlterMeta(fn coretypes.Callable, args []coretypes.Object) coretypes.Map {
	var meta coretypes.Object = a.GetMeta()
	if meta == nil {
		meta = coretypes.RuntimeNil
	}
	fargs := append([]coretypes.Object{meta}, args...)
	newMeta := coretypes.EnsureObjectIsMap(fn.Call(fargs), "")
	a.SetMeta(newMeta)
	return newMeta
}
func (a *Atom) Deref() coretypes.Object {
	a.mu.Lock()
	v := a.value
	a.mu.Unlock()
	return v
}

func (a *Atom) Swap(fn coretypes.Callable, args []coretypes.Object, validate func(coretypes.Object)) (oldValue, newValue coretypes.Object) {
	a.mu.Lock()
	defer a.mu.Unlock()
	fargs := append([]coretypes.Object{a.value}, args...)
	oldValue = a.value
	newValue = fn.Call(fargs)
	if validate != nil {
		validate(newValue)
	}
	a.value = newValue
	return oldValue, newValue
}

func (a *Atom) Reset(newValue coretypes.Object, validate func(coretypes.Object)) (oldValue coretypes.Object) {
	a.mu.Lock()
	defer a.mu.Unlock()
	oldValue = a.value
	if validate != nil {
		validate(newValue)
	}
	a.value = newValue
	return oldValue
}

func (a *Atom) CompareAndSet(oldValue, newValue coretypes.Object, validate func(coretypes.Object)) (coretypes.Object, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.value.Equals(oldValue) {
		return nil, false
	}
	old := a.value
	if validate != nil {
		validate(newValue)
	}
	a.value = newValue
	return old, true
}
