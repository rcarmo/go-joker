package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	corert "github.com/rcarmo/go-joker/core/runtime"
)

type (
	ChannelReceiveStatus = corert.ChannelReceiveStatus

	FutureResult struct {
		value Object
		err   Error
	}
	Channel struct {
		runtime *corert.Channel[FutureResult]
		hash    uint32
	}
)

const (
	ChannelReceiveValue  = corert.ChannelReceiveValue
	ChannelReceiveClosed = corert.ChannelReceiveClosed
	ChannelReceiveDone   = corert.ChannelReceiveDone
)

func MakeFutureResult(value Object, err Error) FutureResult {
	return FutureResult{value: value, err: err}
}

func (ch *Channel) ToString(escape bool) string {
	return "#object[Channel]"
}

func (ch *Channel) Equals(other interface{}) bool {
	return ch == other
}

func (ch *Channel) GetInfo() *coretypes.ObjectInfo {
	return nil
}

func (ch *Channel) GetType() *coretypes.Type {
	return TYPE.Channel
}

func (ch *Channel) Hash() uint32 {
	return ch.hash
}

func (ch *Channel) WithInfo(info *coretypes.ObjectInfo) Object {
	return ch
}

func MakeChannel(ch chan FutureResult) *Channel {
	res := &Channel{runtime: corert.NewChannel(ch), hash: 0}
	res.hash = hashutil.Ptr(uintptr(unsafe.Pointer(res)))
	return res
}

func ExtractChannel(args []Object, index int) *Channel {
	return EnsureArgIsChannel(args, index)
}

func (ch *Channel) Close() {
	ch.runtime.Close()
}

func (ch *Channel) IsClosed() bool {
	return ch.runtime.IsClosed()
}

func (ch *Channel) raw() chan FutureResult {
	return ch.runtime.Raw()
}

func (ch *Channel) Send(value Object) bool {
	return ch.SendResult(MakeFutureResult(value, nil))
}

func (ch *Channel) SendResult(result FutureResult) bool {
	return ch.runtime.Send(result)
}

func (ch *Channel) Receive(done <-chan struct{}) (Object, ChannelReceiveStatus, Error) {
	res, status := ch.runtime.Receive(done)
	if status != ChannelReceiveValue {
		return NIL, status, nil
	}
	return res.value, status, res.err
}
