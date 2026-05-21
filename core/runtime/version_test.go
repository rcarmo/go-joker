package runtime

import "testing"

func TestVersionHasLeadingV(t *testing.T) {
	if VERSION == "" || VERSION[0] != 'v' {
		t.Fatalf("VERSION = %q, want leading v", VERSION)
	}
}
