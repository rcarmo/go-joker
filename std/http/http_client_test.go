package http

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestPersistentClientReusesConnection(t *testing.T) {
	seen := make(map[string]bool)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.RemoteAddr] = true
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()

	hc := makeClient(nil).(*HTTPClient)
	defer closeClient(hc)

	for i := 0; i < 3; i++ {
		req := EmptyArrayMap()
		req.Add(coretypes.MakeKeyword(STRINGS.Intern, "url"), coretypes.MakeString(srv.URL))
		req.Add(coretypes.MakeKeyword(STRINGS.Intern, "client"), hc)
		resp := sendRequest(req)
		if ok, v := resp.Get(coretypes.MakeKeyword(STRINGS.Intern, "body")); !ok || v.ToString(false) != "ok" {
			t.Fatalf("unexpected response body: %v", v)
		}
	}
	if len(seen) != 1 {
		t.Fatalf("expected one reused TCP connection, saw %d remote addrs: %#v", len(seen), seen)
	}
}

func TestPersistentClientRejectsOverflowingIdleTimeout(t *testing.T) {
	opts := EmptyArrayMap()
	opts.Add(coretypes.MakeKeyword(STRINGS.Intern, "idle-timeout-ms"), coretypes.MakeInt(int(^uint(0)>>1)))
	defer func() {
		if recover() == nil {
			t.Fatal("overflowing idle timeout option did not panic")
		}
	}()
	_ = makeClient([]coretypes.Object{opts})
}

func TestPersistentClientRejectsNegativeOptions(t *testing.T) {
	opts := EmptyArrayMap()
	opts.Add(coretypes.MakeKeyword(STRINGS.Intern, "idle-timeout-ms"), coretypes.MakeInt(-1))
	defer func() {
		if recover() == nil {
			t.Fatal("negative client option did not panic")
		}
	}()
	_ = makeClient([]coretypes.Object{opts})
}

func TestPersistentClientOptions(t *testing.T) {
	opts := EmptyArrayMap()
	opts.Add(coretypes.MakeKeyword(STRINGS.Intern, "max-idle-conns"), coretypes.MakeInt(7))
	opts.Add(coretypes.MakeKeyword(STRINGS.Intern, "max-idle-conns-per-host"), coretypes.MakeInt(3))
	opts.Add(coretypes.MakeKeyword(STRINGS.Intern, "idle-timeout-ms"), coretypes.MakeInt(1234))
	hc := makeClient([]coretypes.Object{opts}).(*HTTPClient)
	if hc.transport.MaxIdleConns != 7 || hc.transport.MaxIdleConnsPerHost != 3 {
		t.Fatalf("options not applied: %#v", hc.transport)
	}
	closeClient(hc)
}
