package runtime

import (
	"sync"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

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

type FutureResult struct {
	Value coretypes.Object
	Err   coretypes.Error
}

type ObjectChannel struct {
	runtime *Channel[FutureResult]
	hash    uint32
}

func NewFutureResult(value coretypes.Object, err coretypes.Error) FutureResult {
	return FutureResult{Value: value, Err: err}
}

func NewObjectChannel(ch chan FutureResult) *ObjectChannel {
	res := &ObjectChannel{runtime: NewChannel(ch), hash: 0}
	res.hash = hashutil.Ptr(uintptr(unsafe.Pointer(res)))
	return res
}

func ExtractChannel(args []coretypes.Object, index int) *ObjectChannel {
	if ch, ok := args[index].(*ObjectChannel); ok {
		return ch
	}
	panic(coretypes.RuntimeError("arg " + coretypes.MakeInt(index).ToString(false) + " must be Channel"))
}

func (ch *ObjectChannel) ToString(escape bool) string                          { return "#object[Channel]" }
func (ch *ObjectChannel) Equals(other interface{}) bool                        { return ch == other }
func (ch *ObjectChannel) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (ch *ObjectChannel) GetType() *coretypes.Type                             { return coretypes.RuntimeTypes.Channel }
func (ch *ObjectChannel) Hash() uint32                                         { return ch.hash }
func (ch *ObjectChannel) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return ch }

func (ch *ObjectChannel) Close()                 { ch.runtime.Close() }
func (ch *ObjectChannel) IsClosed() bool         { return ch.runtime.IsClosed() }
func (ch *ObjectChannel) Raw() chan FutureResult { return ch.runtime.Raw() }
func (ch *ObjectChannel) Send(value coretypes.Object) bool {
	return ch.SendResult(NewFutureResult(value, nil))
}
func (ch *ObjectChannel) SendResult(result FutureResult) bool { return ch.runtime.Send(result) }

func (ch *ObjectChannel) Receive(done <-chan struct{}) (coretypes.Object, ChannelReceiveStatus, coretypes.Error) {
	res, status := ch.runtime.Receive(done)
	if status != ChannelReceiveValue {
		return coretypes.RuntimeNil, status, nil
	}
	return res.Value, status, res.Err
}
