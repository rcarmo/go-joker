package transit

import (
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestTransitPodHelpers(t *testing.T) {
	encoded, err := TransitEncodeArgs([]Object{MakeString("hi"), MakeKeyword("k")})
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
