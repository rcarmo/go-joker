package string

import "testing"

func TestSplitQualified(t *testing.T) {
	ns, local, ok := SplitQualified("joker.core/map")
	if !ok || ns != "joker.core" || local != "map" {
		t.Fatalf("SplitQualified returned ok=%v ns=%q local=%q", ok, ns, local)
	}

	ns, local, ok = SplitQualified("map")
	if ok || ns != "" || local != "map" {
		t.Fatalf("SplitQualified(unqualified) returned ok=%v ns=%q local=%q", ok, ns, local)
	}
}
