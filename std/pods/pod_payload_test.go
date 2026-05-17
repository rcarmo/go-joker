package pods

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestPodTransitPayloadSupport(t *testing.T) {
	p := newPod("pod-transit", "transit", "transit+json", nil, nil, nil)
	encoded, err := p.encodeArgs([]Object{MakeKeyword("k")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(encoded, `"~#list"`) || !strings.Contains(encoded, `"~:k"`) {
		t.Fatalf("unexpected transit args: %s", encoded)
	}
	decoded, err := p.decodePayload(`"~:ok"`)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ToString(false) != ":ok" {
		t.Fatalf("decode mismatch: %s", decoded.ToString(false))
	}
}

func TestPodEDNPayloadSupport(t *testing.T) {
	p := newPod("pod-edn", "edn", "edn", nil, nil, nil)
	encoded, err := p.encodeArgs([]Object{coretypes.MakeString("x"), MakeKeyword("k")})
	if err != nil {
		t.Fatal(err)
	}
	if encoded != "[\"x\" :k]" {
		t.Fatalf("unexpected EDN args: %s", encoded)
	}
	decoded, err := p.decodePayload(`{:ok true}`)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ToString(false) != "{:ok true}" {
		t.Fatalf("decode mismatch: %s", decoded.ToString(false))
	}
}
