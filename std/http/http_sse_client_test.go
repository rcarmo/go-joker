package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/rcarmo/go-joker/core"
	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

func sseRequest(url string, timeoutMS int) *corecollections.ArrayMap {
	request := corecollections.EmptyArrayMap()
	request.Add(coretypes.MakeKeyword(STRINGS.Intern, "url"), coretypes.MakeString(url))
	request.Add(coretypes.MakeKeyword(STRINGS.Intern, "timeout-ms"), coretypes.MakeInt(timeoutMS))
	return request
}

func TestSendSSEParsesEventsAndSupportsCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("id: 7\nevent: text\ndata: hello\ndata: world\n\ndata: ignored\n\n"))
	}))
	defer server.Close()

	var events []coretypes.Map
	callback := Proc{Name: "test-sse", Fn: func(args []coretypes.Object) coretypes.Object {
		events = append(events, coretypes.EnsureObjectIsMap(args[0], "event: %s"))
		return coretypes.MakeBoolean(false)
	}}
	response := sendSSE(sseRequest(server.URL, 1000), callback)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	_, data := events[0].Get(coretypes.MakeKeyword(STRINGS.Intern, "data"))
	if got := coretypes.EnsureObjectIsString(data, "data: %s").S; got != "hello\nworld" {
		t.Fatalf("data = %q", got)
	}
	if ok, cancelled := response.Get(coretypes.MakeKeyword(STRINGS.Intern, "cancelled")); !ok || !corert.ToBool(cancelled) {
		t.Fatal("expected cancelled response")
	}
}

func TestSendSSEPreservesMetadataAndDispatchesEOF(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("id: last\nretry: 1500\ndata: eof"))
	}))
	defer server.Close()

	var received coretypes.Map
	callback := Proc{Name: "test-sse", Fn: func(args []coretypes.Object) coretypes.Object {
		received = coretypes.EnsureObjectIsMap(args[0], "event: %s")
		return coretypes.MakeBoolean(true)
	}}
	_ = sendSSE(sseRequest(server.URL, 1000), callback)
	if received == nil {
		t.Fatal("expected EOF event")
	}
	_, id := received.Get(coretypes.MakeKeyword(STRINGS.Intern, "id"))
	if coretypes.EnsureObjectIsString(id, "id: %s").S != "last" {
		t.Fatal("expected event id")
	}
	_, retry := received.Get(coretypes.MakeKeyword(STRINGS.Intern, "retry"))
	if coretypes.ExtractInt([]coretypes.Object{retry}, 0) != 1500 {
		t.Fatal("expected retry metadata")
	}
}

func TestSendSSEReturnsBoundedErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte("retry later"))
	}))
	defer server.Close()

	called := false
	callback := Proc{Name: "test-sse", Fn: func([]coretypes.Object) coretypes.Object {
		called = true
		return coretypes.MakeBoolean(true)
	}}
	response := sendSSE(sseRequest(server.URL, 1000), callback)
	if called {
		t.Fatal("callback invoked for non-2xx response")
	}
	_, body := response.Get(coretypes.MakeKeyword(STRINGS.Intern, "body"))
	if coretypes.EnsureObjectIsString(body, "body: %s").S != "retry later" {
		t.Fatal("unexpected error body")
	}
}

func TestSendSSEBoundsWholeEvent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: 1234\ndata: 5678\n\n"))
	}))
	defer server.Close()

	request := sseRequest(server.URL, 1000)
	request.Add(coretypes.MakeKeyword(STRINGS.Intern, "max-event-bytes"), coretypes.MakeInt(16))
	defer func() {
		if recover() == nil {
			t.Fatal("expected whole-event size failure")
		}
	}()
	callback := Proc{Name: "test-sse", Fn: func([]coretypes.Object) coretypes.Object {
		return coretypes.MakeBoolean(true)
	}}
	_ = sendSSE(request, callback)
}

func TestSendSSERejectsZeroEventLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("data: value\n\n"))
	}))
	defer server.Close()
	request := sseRequest(server.URL, 1000)
	request.Add(coretypes.MakeKeyword(STRINGS.Intern, "max-event-bytes"), coretypes.MakeInt(0))
	defer func() {
		if recover() == nil {
			t.Fatal("expected zero event limit failure")
		}
	}()
	_ = sendSSE(request, Proc{Name: "callback", Fn: func([]coretypes.Object) coretypes.Object {
		return coretypes.MakeBoolean(true)
	}})
}

func TestSendSSEHonorsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte("data: late\n\n"))
	}))
	defer server.Close()

	defer func() {
		if recover() == nil {
			t.Fatal("expected timeout panic")
		}
	}()
	callback := Proc{Name: "test-sse", Fn: func([]coretypes.Object) coretypes.Object { return coretypes.MakeBoolean(true) }}
	_ = sendSSE(sseRequest(server.URL, 10), callback)
}
