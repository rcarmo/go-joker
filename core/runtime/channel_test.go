package runtime

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func TestChannelSendReceiveAndClose(t *testing.T) {
	ch := NewChannel(make(chan int, 1))
	if ch.IsClosed() {
		t.Fatal("new channel is closed")
	}
	if !ch.Send(42) {
		t.Fatal("send failed")
	}
	got, status := ch.Receive(nil)
	if status != ChannelReceiveValue || got != 42 {
		t.Fatalf("receive = (%d, %d)", got, status)
	}
	ch.Close()
	if !ch.IsClosed() {
		t.Fatal("close did not mark closed")
	}
	if ch.Send(1) {
		t.Fatal("send to closed channel succeeded")
	}
	_, status = ch.Receive(nil)
	if status != ChannelReceiveClosed {
		t.Fatalf("closed receive status = %d", status)
	}
	ch.Close()
}

func TestChannelReceiveDone(t *testing.T) {
	ch := NewChannel(make(chan int))
	done := make(chan struct{})
	close(done)
	_, status := ch.Receive(done)
	if status != ChannelReceiveDone {
		t.Fatalf("status = %d, want done", status)
	}
}

type channelObjectTestError string

func (e channelObjectTestError) Error() string                                   { return string(e) }
func (e channelObjectTestError) Message() coretypes.Object                       { return coretypes.MakeString(string(e)) }
func (e channelObjectTestError) ToString(escape bool) string                     { return string(e) }
func (e channelObjectTestError) Equals(other interface{}) bool                   { return e == other }
func (e channelObjectTestError) GetInfo() *coretypes.ObjectInfo                  { return nil }
func (e channelObjectTestError) GetType() *coretypes.Type                        { return nil }
func (e channelObjectTestError) Hash() uint32                                    { return uint32(len(e)) }
func (e channelObjectTestError) WithInfo(*coretypes.ObjectInfo) coretypes.Object { return e }

func TestObjectChannelSendReceiveAndClose(t *testing.T) {
	ch := NewObjectChannel(make(chan FutureResult, 1))
	value := coretypes.Int{I: 42}
	if !ch.Send(value) {
		t.Fatal("send failed")
	}
	got, status, err := ch.Receive(nil)
	if status != ChannelReceiveValue || err != nil || !got.Equals(value) {
		t.Fatalf("receive = (%v, %d, %v), want (42, value, nil)", got, status, err)
	}
	ch.Close()
	if !ch.IsClosed() {
		t.Fatal("close did not mark channel closed")
	}
	if ch.Send(value) {
		t.Fatal("send to closed object channel succeeded")
	}
	_, status, err = ch.Receive(nil)
	if status != ChannelReceiveClosed || err != nil {
		t.Fatalf("closed receive = (%d, %v), want (closed, nil)", status, err)
	}
}

func TestObjectChannelReceivePropagatesFutureResultError(t *testing.T) {
	ch := NewObjectChannel(make(chan FutureResult, 1))
	sentinel := channelObjectTestError("boom")
	if !ch.SendResult(NewFutureResult(nil, sentinel)) {
		t.Fatal("send result failed")
	}
	_, status, err := ch.Receive(nil)
	if status != ChannelReceiveValue || err != sentinel {
		t.Fatalf("receive error = (%d, %v), want (value, sentinel)", status, err)
	}
}
