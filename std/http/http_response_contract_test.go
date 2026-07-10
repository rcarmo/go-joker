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
