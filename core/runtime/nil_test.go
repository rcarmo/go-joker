package runtime

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func TestNilBehavesAsEmptySeqAndMap(t *testing.T) {
	n := Nil{}
	if !n.Equals(Nil{}) || !n.IsEmpty() || n.Count() != 0 {
		t.Fatalf("unexpected nil behavior: empty=%v count=%d", n.IsEmpty(), n.Count())
	}
	if !n.First().Equals(Nil{}) || !n.Rest().(Nil).Equals(Nil{}) {
		t.Fatalf("nil seq first/rest mismatch")
	}
	if ok, got := n.Get(coretypes.MakeKeyword(func(s string) *string { return &s }, "missing")); ok || !got.Equals(Nil{}) {
		t.Fatalf("nil get = %v, %v", ok, got)
	}
	if counted, ok := n.Conj(coretypes.MakeInt(1)).(coretypes.Counted); !ok || counted.Count() != 1 {
		t.Fatal("nil conj should create singleton collection")
	}
}
