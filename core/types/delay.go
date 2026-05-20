package types

import (
	"sync"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
)

type Delay struct {
	Fn      Callable
	Runtime *DelayPromise
}

type DelayPromise struct {
	mu        sync.Mutex
	value     Object
	delivered bool
	done      chan struct{}
}

func NewDelayPromise() *DelayPromise { return &DelayPromise{done: make(chan struct{})} }

func (p *DelayPromise) Deliver(value Object) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.delivered {
		return false
	}
	p.value = value
	p.delivered = true
	close(p.done)
	return true
}

func (p *DelayPromise) Await() Object {
	<-p.done
	return p.value
}

func (p *DelayPromise) IsRealized() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
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
		d.Runtime = NewDelayPromise()
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
