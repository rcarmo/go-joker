package pods

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestPodPayloadEDNArgsAndResult(t *testing.T) {
	p := &Pod{format: "edn"}
	payload, err := p.encodeArgs([]Object{coretypes.MakeInt(1), MakeKeyword("a"), NewVectorFrom(coretypes.MakeString("x"))})
	if err != nil {
		t.Fatal(err)
	}
	if payload != "[1 :a [\"x\"]]" {
		t.Fatalf("payload = %q", payload)
	}
	obj, err := p.decodePayload("{:ok true :n 2}")
	if err != nil {
		t.Fatal(err)
	}
	if obj.ToString(false) != "{:ok true, :n 2}" {
		t.Fatalf("decoded = %s", obj.ToString(false))
	}
}
