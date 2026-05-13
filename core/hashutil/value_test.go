package hashutil

import (
	"encoding/gob"
	"testing"
)

type gobValue struct{ V string }

func (g gobValue) GobEncode() ([]byte, error)   { return []byte(g.V), nil }
func (g *gobValue) GobDecode(data []byte) error { g.V = string(data); return nil }

var _ gob.GobEncoder = gobValue{}

func TestUint32Bytes(t *testing.T) {
	b := Uint32Bytes(0x01020304)
	if len(b) != 4 || b[0] != 0x04 || b[3] != 0x01 {
		t.Fatalf("Uint32Bytes returned %#v", b)
	}
}

func TestSymbol(t *testing.T) {
	ns, name := "joker.core", "map"
	if Symbol(&ns, &name) == 0 {
		t.Fatal("expected non-zero symbol hash")
	}
}

func TestGobEncoder(t *testing.T) {
	if GobEncoder(gobValue{V: "x"}) == 0 {
		t.Fatal("expected non-zero gob hash")
	}
}
