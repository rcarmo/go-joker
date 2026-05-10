package pods

import (
	"strings"
	"testing"

	. "github.com/candid82/joker/core"
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

func TestPodEDNPayloadStillExplicitlyUnsupported(t *testing.T) {
	p := newPod("pod-edn", "edn", "edn", nil, nil, nil)
	if _, err := p.encodeArgs([]Object{MakeString("x")}); err == nil || !strings.Contains(err.Error(), "EDN") {
		t.Fatalf("expected EDN unsupported error, got %v", err)
	}
}
