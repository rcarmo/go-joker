package types

import (
	"sync"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
)

type Delay struct {
	Fn      Callable
	mu      sync.Mutex
	Runtime *DelayPromise
}

type DelayPromise struct {
	mu        sync.Mutex
	value     Object
	delivered bool
	forcing   bool
	done      chan struct{}
}

func NewDelayPromise() *DelayPromise { return &DelayPromise{done: make(chan struct{})} }

func (p *DelayPromise) Force(fn func() Object) Object {
	for {
		p.mu.Lock()
		if p.delivered {
			value := p.value
			p.mu.Unlock()
			return value
		}
		if p.forcing {
			done := p.done
			p.mu.Unlock()
			<-done
			continue
		}
		p.forcing = true
		p.mu.Unlock()

		value, panicValue := callDelay(fn)

		p.mu.Lock()
		p.forcing = false
		if p.delivered {
			value = p.value
		} else if panicValue == nil {
			p.value = value
			p.delivered = true
			close(p.done)
		} else {
			close(p.done)
			p.done = make(chan struct{})
		}
		p.mu.Unlock()
		if panicValue != nil {
			panic(panicValue)
		}
		return value
	}
}

func callDelay(fn func() Object) (value Object, panicValue any) {
	defer func() {
		panicValue = recover()
	}()
	value = fn()
	return value, nil
}

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

func NewDelay(fn Callable) *Delay { return &Delay{Fn: fn, Runtime: NewDelayPromise()} }

func (d *Delay) promise() *DelayPromise {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Runtime == nil {
		d.Runtime = NewDelayPromise()
	}
	return d.Runtime
}

func (d *Delay) ToString(escape bool) string      { return "#object[Delay]" }
func (d *Delay) Equals(other interface{}) bool    { return d == other }
func (d *Delay) GetInfo() *ObjectInfo             { return nil }
func (d *Delay) GetType() *Type                   { return RuntimeTypes.Delay }
func (d *Delay) Hash() uint32                     { return hashutil.Ptr(uintptr(unsafe.Pointer(d))) }
func (d *Delay) WithInfo(info *ObjectInfo) Object { return d }
func (d *Delay) Force() Object {
	return d.promise().Force(func() Object {
		if DelayCall == nil {
			panic("DelayCall is not configured")
		}
		return DelayCall(d.Fn)
	})
}
func (d *Delay) Deref() Object    { return d.Force() }
func (d *Delay) IsRealized() bool { return d.promise().IsRealized() }
