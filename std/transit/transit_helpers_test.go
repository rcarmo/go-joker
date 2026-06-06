package transit

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestTransitPodHelpers(t *testing.T) {
	encoded, err := TransitEncodeArgs([]coretypes.Object{coretypes.MakeString("hi"), coretypes.MakeKeyword(STRINGS.Intern, "k")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded, `"~#list"`) || !strings.Contains(encoded, `"~:k"`) {
		t.Fatalf("unexpected pod args transit: %s", encoded)
	}
	decoded, err := TransitDecodeValue(`"~:ok"`)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ToString(false) != ":ok" {
		t.Fatalf("decode helper mismatch: %s", decoded.ToString(false))
	}
}

func FuzzTransitDecodeValue(f *testing.F) {
	for _, seed := range []string{
		`null`,
		`true`,
		`42`,
		`"plain"`,
		`"~:ok"`,
		`["~#list",[1,2]]`,
		`["~#set",[1,"~:a"]]`,
		`["~#cmap",["~:k",1]]`,
		`{"~:k":1}`,
		`[`,
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, src string) {
		if len(src) > 64*1024 {
			t.Skip("fuzz input too large for Transit decode smoke target")
		}
		_, _ = TransitDecodeValue(src)
	})
}
