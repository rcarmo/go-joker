package runtime

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func TestVersionHasLeadingV(t *testing.T) {
	if VERSION == "" || VERSION[0] != 'v' {
		t.Fatalf("VERSION = %q, want leading v", VERSION)
	}
}

func TestVersionMap(t *testing.T) {
	keys := map[string]*string{}
	intern := func(s string) *string {
		if keys[s] == nil {
			v := s
			keys[s] = &v
		}
		return keys[s]
	}
	m := VersionMap(intern)
	for _, key := range []string{"major", "minor", "incremental"} {
		if ok, _ := m.Get(coretypes.MakeKeyword(intern, key)); !ok {
			t.Fatalf("VersionMap missing %s", key)
		}
	}
}
