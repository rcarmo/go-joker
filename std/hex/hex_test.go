package hex

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestHexEncodeDecodeString(t *testing.T) {
	encoded := __encode_string_([]coretypes.Object{coretypes.MakeString("joker")}).(coretypes.String).S
	if encoded != "6a6f6b6572" {
		t.Fatalf("unexpected encoding: %s", encoded)
	}
	decoded := __decode_string_([]coretypes.Object{coretypes.MakeString(encoded)}).(coretypes.String).S
	if decoded != "joker" {
		t.Fatalf("unexpected decoding: %s", decoded)
	}
}
