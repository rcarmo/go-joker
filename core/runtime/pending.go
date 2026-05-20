package runtime

import (
	"reflect"
	"sync"

	"github.com/rcarmo/go-joker/core/hashutil"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

type Future[T any, E any] struct {
	value T
	err   E
	done  chan struct{}
}

func NewFuture[T any, E any]() *Future[T, E] {
	return &Future[T, E]{done: make(chan struct{})}
}

func (f *Future[T, E]) Complete(value T, err E) {
	f.value = value
	f.err = err
	close(f.done)
}

func (f *Future[T, E]) Await() (T, E) {
	<-f.done
	return f.value, f.err
}

func (f *Future[T, E]) IsRealized() bool {
	select {
	case <-f.done:
		return true
	default:
		return false
	}
}

type Promise[T any] struct {
	mu        sync.Mutex
	value     T
	delivered bool
	done      chan struct{}
}

func NewPromise[T any]() *Promise[T] {
	return &Promise[T]{done: make(chan struct{})}
}

func (p *Promise[T]) Deliver(value T) bool {
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

func (p *Promise[T]) Await() T {
	<-p.done
	return p.value
}

func (p *Promise[T]) IsRealized() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

// ObjectFuture holds a value computed asynchronously and exposes Joker object
// protocols while reusing the runtime Future primitive.
type ObjectFuture struct {
	runtime *Future[coretypes.Object, coretypes.Error]
}

func NewObjectFuture() *ObjectFuture {
	return &ObjectFuture{runtime: NewFuture[coretypes.Object, coretypes.Error]()}
}

func (f *ObjectFuture) Complete(value coretypes.Object, err coretypes.Error) {
	f.runtime.Complete(value, err)
}

func (f *ObjectFuture) ToString(escape bool) string    { return "#object[Future]" }
func (f *ObjectFuture) Equals(other interface{}) bool  { return f == other }
func (f *ObjectFuture) GetInfo() *coretypes.ObjectInfo { return nil }
func (f *ObjectFuture) GetType() *coretypes.Type       { return coretypes.RuntimeTypes.Fn }
func (f *ObjectFuture) Hash() uint32 {
	return hashutil.Ptr(uintptr(reflect.ValueOf(f).Pointer()))
}
func (f *ObjectFuture) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return f }

func (f *ObjectFuture) Deref() coretypes.Object {
	value, err := f.runtime.Await()
	if err != nil {
		panic(coretypes.Object(err))
	}
	return value
}

func (f *ObjectFuture) IsRealized() bool { return f.runtime.IsRealized() }

// ObjectPromise holds a value that can be delivered once and exposes Joker
// object protocols while reusing the runtime Promise primitive.
type ObjectPromise struct {
	runtime *Promise[coretypes.Object]
}

func NewObjectPromise() *ObjectPromise {
	return &ObjectPromise{runtime: NewPromise[coretypes.Object]()}
}

func (p *ObjectPromise) Deliver(value coretypes.Object) bool { return p.runtime.Deliver(value) }

func (p *ObjectPromise) ToString(escape bool) string    { return "#object[Promise]" }
func (p *ObjectPromise) Equals(other interface{}) bool  { return p == other }
func (p *ObjectPromise) GetInfo() *coretypes.ObjectInfo { return nil }
func (p *ObjectPromise) GetType() *coretypes.Type       { return coretypes.RuntimeTypes.Fn }
func (p *ObjectPromise) Hash() uint32 {
	return hashutil.Ptr(uintptr(reflect.ValueOf(p).Pointer()))
}
func (p *ObjectPromise) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return p }

func (p *ObjectPromise) Deref() coretypes.Object { return p.runtime.Await() }
func (p *ObjectPromise) IsRealized() bool        { return p.runtime.IsRealized() }
