package runtime

import "testing"

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
