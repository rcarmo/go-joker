package types

import (
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	corert "github.com/rcarmo/go-joker/core/runtime"
)

type Delay struct {
	Fn      Callable
	Runtime *corert.Promise[Object]
}

var DelayCall func(Callable) Object

func NewDelay(fn Callable) *Delay { return &Delay{Fn: fn} }

func (d *Delay) ToString(escape bool) string      { return "#object[Delay]" }
func (d *Delay) Equals(other interface{}) bool    { return d == other }
func (d *Delay) GetInfo() *ObjectInfo             { return nil }
func (d *Delay) GetType() *Type                   { return RuntimeTypes.Delay }
func (d *Delay) Hash() uint32                     { return hashutil.Ptr(uintptr(unsafe.Pointer(d))) }
func (d *Delay) WithInfo(info *ObjectInfo) Object { return d }
func (d *Delay) Force() Object {
	if d.Runtime == nil {
		d.Runtime = corert.NewPromise[Object]()
	}
	if d.Runtime.IsRealized() {
		return d.Runtime.Await()
	}
	if DelayCall == nil {
		panic("DelayCall is not configured")
	}
	value := DelayCall(d.Fn)
	d.Runtime.Deliver(value)
	return value
}
func (d *Delay) Deref() Object    { return d.Force() }
func (d *Delay) IsRealized() bool { return d.Runtime != nil && d.Runtime.IsRealized() }
