package yaml

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestYAMLReadWriteString(t *testing.T) {
	obj := readString("a: 1\nb:\n  - true\n  - x\n").(coretypes.Map)
	ok, a := obj.Get(coretypes.MakeString("a"))
	if !ok || a.(coretypes.Int).I != 1 {
		t.Fatalf("yaml read mismatch: %v", a)
	}
	m := EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "name"), coretypes.MakeString("joker"))
	out := writeString(m).S
	if !strings.Contains(out, "name: joker") {
		t.Fatalf("yaml write mismatch: %s", out)
	}
}
