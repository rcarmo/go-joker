package core

import (
	"sync"
	"testing"
)

func TestChannelCloseIsIdempotentUnderConcurrency(t *testing.T) {
	ch := MakeChannel(make(chan FutureResult, 1))
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch.Close()
		}()
	}
	wg.Wait()
	if !ch.IsClosed() {
		t.Fatal("channel should report closed after concurrent Close calls")
	}
	if ch.Send(MakeInt(1)) {
		t.Fatal("Send on closed channel should return false")
	}
}
