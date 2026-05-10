package hex

import (
	"testing"

	. "github.com/candid82/joker/core"
)

func TestHexEncodeDecodeString(t *testing.T) {
	encoded := __encode_string_([]Object{MakeString("joker")}).(String).S
	if encoded != "6a6f6b6572" {
		t.Fatalf("unexpected encoding: %s", encoded)
	}
	decoded := __decode_string_([]Object{MakeString(encoded)}).(String).S
	if decoded != "joker" {
		t.Fatalf("unexpected decoding: %s", decoded)
	}
}
