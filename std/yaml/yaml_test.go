package yaml

import (
	"strings"
	"testing"

	. "github.com/candid82/joker/core"
)

func TestYAMLReadWriteString(t *testing.T) {
	obj := readString("a: 1\nb:\n  - true\n  - x\n").(Map)
	ok, a := obj.Get(MakeString("a"))
	if !ok || a.(Int).I != 1 {
		t.Fatalf("yaml read mismatch: %v", a)
	}
	m := EmptyArrayMap()
	m.Add(MakeKeyword("name"), MakeString("joker"))
	out := writeString(m).S
	if !strings.Contains(out, "name: joker") {
		t.Fatalf("yaml write mismatch: %s", out)
	}
}
