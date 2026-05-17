package runtime

import "sync"

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
