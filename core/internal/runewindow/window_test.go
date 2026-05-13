package runewindow

import "testing"

func TestWindowKeepsRecentRunes(t *testing.T) {
	var w Window
	for _, r := range []rune{'a', 'b', 'c', 'd', 'e', 'f'} {
		w.Add(r)
	}
	if got := w.Size(); got != 4 {
		t.Fatalf("Size = %d, want 4", got)
	}
	if got := w.Top(0); got != 'f' {
		t.Fatalf("Top(0) = %q, want f", got)
	}
	if got := w.Top(3); got != 'c' {
		t.Fatalf("Top(3) = %q, want c", got)
	}
}
