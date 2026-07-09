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
	mu            sync.Mutex
	value         coretypes.Object
	valueRevision uint64
}

func NewAtom(value coretypes.Object, meta coretypes.Map) *Atom {
	res := &Atom{value: value}
	if meta != nil {
		res.Meta = meta
	}
	return res
}

func (a *Atom) ToString(escape bool) string {
	return "#object[Atom {:val " + a.Deref().ToString(escape) + "}]"
}
func (a *Atom) Equals(other interface{}) bool                        { return a == other }
func (a *Atom) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (a *Atom) GetType() *coretypes.Type                             { return coretypes.RuntimeTypes.Atom }
func (a *Atom) Hash() uint32                                         { return hashutil.Ptr(uintptr(unsafe.Pointer(a))) }
func (a *Atom) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return a }

func (a *Atom) WithMeta(meta coretypes.Map) coretypes.Object {
	res := &Atom{value: a.Deref()}
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
	for {
		a.mu.Lock()
		oldValue = a.value
		revision := a.valueRevision
		a.mu.Unlock()

		fargs := append([]coretypes.Object{oldValue}, args...)
		newValue = fn.Call(fargs)
		if validate != nil {
			validate(newValue)
		}

		a.mu.Lock()
		if a.valueRevision != revision {
			a.mu.Unlock()
			continue
		}
		a.value = newValue
		a.valueRevision++
		a.mu.Unlock()
		return oldValue, newValue
	}
}

func (a *Atom) Reset(newValue coretypes.Object, validate func(coretypes.Object)) (oldValue coretypes.Object) {
	if validate != nil {
		validate(newValue)
	}
	a.mu.Lock()
	oldValue = a.value
	a.value = newValue
	a.valueRevision++
	a.mu.Unlock()
	return oldValue
}

func (a *Atom) CompareAndSet(oldValue, newValue coretypes.Object, validate func(coretypes.Object)) (coretypes.Object, bool) {
	a.mu.Lock()
	matches := a.value.Equals(oldValue)
	a.mu.Unlock()
	if !matches {
		return nil, false
	}
	if validate != nil {
		validate(newValue)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.value.Equals(oldValue) {
		return nil, false
	}
	old := a.value
	a.value = newValue
	a.valueRevision++
	return old, true
}
