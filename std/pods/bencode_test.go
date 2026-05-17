package pods

import (
	"bytes"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestBencodeRoundTripPodMessage(t *testing.T) {
	msg := EmptyArrayMap()
	msg.Add(coretypes.MakeString("op"), coretypes.MakeString("describe"))
	msg.Add(coretypes.MakeString("id"), coretypes.MakeString("joker-1"))
	msg.Add(coretypes.MakeString("args"), NewVectorFrom(coretypes.MakeString("x"), coretypes.MakeInt(42)))

	encoded := bencodeEncodeObject(msg)
	if !bytes.Contains(encoded, []byte("2:id7:joker-1")) || !bytes.Contains(encoded, []byte("2:op8:describe")) {
		t.Fatalf("unexpected bencode message: %q", string(encoded))
	}
	decoded := bencodeDecodeBytes(encoded).(Map)
	if ok, op := decoded.Get(coretypes.MakeString("op")); !ok || op.ToString(false) != "describe" {
		t.Fatalf("op mismatch: %v", op)
	}
	if ok, args := decoded.Get(coretypes.MakeString("args")); !ok || args.(coretypes.CountedIndexed).At(1).(coretypes.Int).I != 42 {
		t.Fatalf("args mismatch: %v", args)
	}
}

func TestBencodeDecodeReader(t *testing.T) {
	obj, err := bencodeDecodeReader(bytes.NewReader([]byte("d2:id1:x2:op8:describee")))
	if err != nil {
		t.Fatal(err)
	}
	m := obj.(Map)
	if ok, id := m.Get(coretypes.MakeString("id")); !ok || id.ToString(false) != "x" {
		t.Fatalf("id mismatch: %v", id)
	}
}
