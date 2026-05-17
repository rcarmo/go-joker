package runtime

import "sync"

type ChannelReceiveStatus int

const (
	ChannelReceiveValue ChannelReceiveStatus = iota
	ChannelReceiveClosed
	ChannelReceiveDone
)

type Channel[T any] struct {
	ch       chan T
	closeMu  sync.Mutex
	isClosed bool
}

func NewChannel[T any](ch chan T) *Channel[T] {
	return &Channel[T]{ch: ch}
}

func (ch *Channel[T]) Raw() chan T {
	return ch.ch
}

func (ch *Channel[T]) Close() {
	ch.closeMu.Lock()
	defer ch.closeMu.Unlock()
	if ch.isClosed {
		return
	}
	ch.isClosed = true
	close(ch.ch)
}

func (ch *Channel[T]) IsClosed() bool {
	ch.closeMu.Lock()
	defer ch.closeMu.Unlock()
	return ch.isClosed
}

func (ch *Channel[T]) Send(value T) (ok bool) {
	if ch.IsClosed() {
		return false
	}
	ok = true
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	ch.ch <- value
	return ok
}

func (ch *Channel[T]) Receive(done <-chan struct{}) (T, ChannelReceiveStatus) {
	var zero T
	if done == nil {
		res, ok := <-ch.ch
		if !ok {
			return zero, ChannelReceiveClosed
		}
		return res, ChannelReceiveValue
	}
	select {
	case res, ok := <-ch.ch:
		if !ok {
			return zero, ChannelReceiveClosed
		}
		return res, ChannelReceiveValue
	case <-done:
		return zero, ChannelReceiveDone
	}
}
