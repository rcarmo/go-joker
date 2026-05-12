package http

import (
	"io"
	stdhttp "net/http"
	"strconv"
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

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
	ok, got := m.Get(MakeKeyword("content-length"))
	if !ok {
		t.Fatal("response map missing :content-length")
	}
	if got.GetType() != TYPE.BigInt {
		t.Fatalf("content-length type = %s (%T), want BigInt", got.GetType().ToString(false), got)
	}
}
