package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	ws "github.com/gorilla/websocket"
	. "github.com/rcarmo/go-joker/core"
)

func TestHandleStreamSSE(t *testing.T) {
	rec := httptest.NewRecorder()
	respMap := EmptyArrayMap()
	respMap.Add(MakeKeyword("status"), MakeInt(200))

	streamFn := Proc{Name: "test-stream", Fn: func(args []Object) Object {
		send := EnsureArgIsCallable(args, 0)
		send.Call([]Object{MakeString("hello")})
		send.Call([]Object{MakeString("tick"), MakeString("42")})
		return NIL
	}}

	handleStream(rec, respMap, streamFn)

	body := rec.Body.String()
	if !strings.Contains(body, "data: hello") {
		t.Fatalf("expected SSE data line, got: %q", body)
	}
	if !strings.Contains(body, "event: tick") {
		t.Fatalf("expected SSE event line, got: %q", body)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
}

type failingStreamWriter struct {
	header http.Header
}

func (w *failingStreamWriter) Header() http.Header {
	return w.header
}

func (w *failingStreamWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func (w *failingStreamWriter) WriteHeader(statusCode int) {}

func (w *failingStreamWriter) Flush() {}

func TestMapToRespRejectsInvalidStatus(t *testing.T) {
	respMap := EmptyArrayMap()
	respMap.Add(MakeKeyword("status"), MakeInt(99))
	defer func() {
		if recover() == nil {
			t.Fatal("invalid response status did not panic")
		}
	}()
	mapToResp(respMap, httptest.NewRecorder())
}

func TestMapToRespWriteErrorsSurface(t *testing.T) {
	respMap := EmptyArrayMap()
	respMap.Add(MakeKeyword("body"), MakeString("hello"))
	defer func() {
		r := recover()
		err, ok := r.(Error)
		if !ok {
			t.Fatalf("panic = %T, want core Error", r)
		}
		if !strings.Contains(err.Error(), "write failed") {
			t.Fatalf("unexpected write error: %s", err.Error())
		}
	}()
	mapToResp(respMap, &failingStreamWriter{header: make(http.Header)})
}

func TestHandleStreamRejectsInvalidStatus(t *testing.T) {
	respMap := EmptyArrayMap()
	respMap.Add(MakeKeyword("status"), MakeInt(1000))
	defer func() {
		if recover() == nil {
			t.Fatal("invalid stream status did not panic")
		}
	}()
	handleStream(httptest.NewRecorder(), respMap, Proc{Name: "stream", Fn: func(args []Object) Object { return NIL }})
}

func TestHandleStreamWriteErrorsSurface(t *testing.T) {
	streamFn := Proc{Name: "test-stream", Fn: func(args []Object) Object {
		send := EnsureArgIsCallable(args, 0)
		send.Call([]Object{MakeString("hello")})
		return NIL
	}}
	defer func() {
		r := recover()
		err, ok := r.(Error)
		if !ok {
			t.Fatalf("panic = %T, want core Error", r)
		}
		if !strings.Contains(err.Error(), "stream write error") {
			t.Fatalf("unexpected stream error: %s", err.Error())
		}
	}()
	handleStream(&failingStreamWriter{header: make(http.Header)}, EmptyArrayMap(), streamFn)
}

func TestHandleWebSocketUpgradeAndCallbacks(t *testing.T) {
	var (
		sendMu sync.Mutex
		sendFn Callable
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conf := EmptyArrayMap()
		conf.Add(MakeKeyword("on-open"), Proc{Name: "on-open", Fn: func(args []Object) Object {
			sendMu.Lock()
			sendFn = EnsureArgIsCallable(args, 0)
			s := sendFn
			sendMu.Unlock()
			s.Call([]Object{MakeString("welcome")})
			return NIL
		}})
		conf.Add(MakeKeyword("on-message"), Proc{Name: "on-message", Fn: func(args []Object) Object {
			sendMu.Lock()
			s := sendFn
			sendMu.Unlock()
			if s != nil {
				s.Call([]Object{args[0]})
			}
			return NIL
		}})
		handleWebSocket(w, r, conf)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read welcome: %v", err)
	}
	if string(msg) != "welcome" {
		t.Fatalf("expected welcome, got %q", string(msg))
	}

	if err := conn.WriteMessage(ws.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("write ping: %v", err)
	}
	_, msg, err = conn.ReadMessage()
	if err != nil {
		t.Fatalf("read echo: %v", err)
	}
	if string(msg) != "ping" {
		t.Fatalf("expected echo ping, got %q", string(msg))
	}
}

func TestHandleWebSocketCloseCallbackIsIdempotent(t *testing.T) {
	done := make(chan Object, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conf := EmptyArrayMap()
		conf.Add(MakeKeyword("on-open"), Proc{Name: "on-open", Fn: func(args []Object) Object {
			closeFn := EnsureArgIsCallable(args, 1)
			closeFn.Call(nil)
			closeFn.Call(nil)
			return NIL
		}})
		conf.Add(MakeKeyword("on-close"), Proc{Name: "on-close", Fn: func(args []Object) Object {
			done <- Boolean{B: true}
			return NIL
		}})
		handleWebSocket(w, r, conf)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := ws.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("websocket dial: %v", err)
	}
	defer conn.Close()
	_, _, _ = conn.ReadMessage()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("websocket on-close was not called")
	}
}

func TestReqToMapRemoteAddrIPv6(t *testing.T) {
	for remote, want := range map[string]string{
		"[2001:db8::1]:8080": "2001:db8::1",
		"2001:db8::1":        "2001:db8::1",
		"[::1]:8080":         "::1",
		"127.0.0.1:12345":    "127.0.0.1",
	} {
		req := httptest.NewRequest("GET", "http://example.com/path?q=1", nil)
		req.RemoteAddr = remote
		m := reqToMap(MakeString("host"), MakeString("8080"), req)
		ok, got := m.Get(MakeKeyword("remote-addr"))
		if !ok || got.ToString(false) != want {
			t.Fatalf("remote %q mapped to %v, want %q", remote, got, want)
		}
	}
}

func FuzzReqToMapRemoteAddr(f *testing.F) {
	f.Add("127.0.0.1:12345")
	f.Add("127.0.0.1")
	f.Add("")
	f.Add("[::1]:8080")
	f.Add("unix")
	f.Fuzz(func(t *testing.T, remote string) {
		req := httptest.NewRequest("GET", "http://example.com/path?q=1", nil)
		req.RemoteAddr = remote
		_ = reqToMap(MakeString("host"), MakeString("8080"), req)
	})
}
