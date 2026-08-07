package http

import (
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"strconv"
	"strings"
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"

	. "github.com/rcarmo/go-joker/core"
)

type failingReadCloser struct {
	closed bool
}

func (r *failingReadCloser) Read([]byte) (int, error) { return 0, errors.New("interrupted body") }
func (r *failingReadCloser) Close() error             { r.closed = true; return nil }

func requireInterruptedBodyPanic(t *testing.T, body *failingReadCloser, fn func()) {
	t.Helper()
	defer func() {
		recovered := recover()
		if recovered == nil || !strings.Contains(fmt.Sprint(recovered), "interrupted body") {
			t.Fatalf("body read panic = %v", recovered)
		}
		if !body.closed {
			t.Fatal("failed body was not closed")
		}
	}()
	fn()
}

func TestRequestAndResponseBodyReadFailuresCloseResources(t *testing.T) {
	t.Run("request", func(t *testing.T) {
		body := &failingReadCloser{}
		req, err := stdhttp.NewRequest(stdhttp.MethodPost, "http://example.test", body)
		if err != nil {
			t.Fatal(err)
		}
		requireInterruptedBodyPanic(t, body, func() {
			_ = reqToMap(coretypes.MakeString("example.test"), coretypes.MakeString("80"), req)
		})
	})
	t.Run("response", func(t *testing.T) {
		body := &failingReadCloser{}
		resp := &stdhttp.Response{StatusCode: 200, Header: make(stdhttp.Header), Body: body}
		requireInterruptedBodyPanic(t, body, func() { _ = respToMap(resp) })
	})
}

func TestRespToMapBoundedRejectsOversizedBodyAndClosesIt(t *testing.T) {
	body := &trackingReadCloser{Reader: strings.NewReader("12345")}
	resp := &stdhttp.Response{StatusCode: 200, Header: make(stdhttp.Header), Body: body}
	defer func() {
		recovered := recover()
		if recovered == nil || !strings.Contains(fmt.Sprint(recovered), "max-response-bytes") {
			t.Fatalf("oversized response panic = %v", recovered)
		}
		if !body.closed {
			t.Fatal("oversized response body was not closed")
		}
	}()
	_ = respToMapBounded(resp, 4)
}

type trackingReadCloser struct {
	io.Reader
	closed bool
}

func (r *trackingReadCloser) Close() error { r.closed = true; return nil }

func TestRespToMapBoundedAcceptsExactLimit(t *testing.T) {
	resp := &stdhttp.Response{
		StatusCode: 200, Header: make(stdhttp.Header),
		Body: io.NopCloser(strings.NewReader("1234")),
	}
	m := respToMapBounded(resp, 4)
	ok, body := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "body"))
	if !ok || coretypes.EnsureObjectIsString(body, "body: %s").S != "1234" {
		t.Fatalf("bounded body = %v", body)
	}
}

func TestRespToMapPromotesLargeContentLengthOn32Bit(t *testing.T) {
	if strconv.IntSize != 32 {
		t.Skip("content-length promotion is only observable on 32-bit int platforms")
	}
	resp := &stdhttp.Response{
		StatusCode:    200,
		ContentLength: int64(1) << 40,
		Header:        stdhttp.Header{},
		Body:          io.NopCloser(strings.NewReader("ok")),
	}
	m := respToMap(resp)
	ok, got := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "content-length"))
	if !ok {
		t.Fatal("response map missing :content-length")
	}
	if got.GetType() != TYPE.BigInt {
		t.Fatalf("content-length type = %s (%T), want BigInt", got.GetType().ToString(false), got)
	}
}
