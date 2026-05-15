//go:build !plan9
// +build !plan9

package reader

import (
	"io"
	"testing"

	"github.com/candid82/liner"
)

func TestLineRuneReaderEOFWithoutPromptInput(t *testing.T) {
	lrr := NewLineRuneReader(liner.NewLiner())
	defer func() {
		if err := lrr.rl.Close(); err != nil {
			t.Fatalf("close liner: %v", err)
		}
	}()
	r, n, err := lrr.ReadRune()
	if err != io.EOF || r != -1 || n != 0 {
		t.Fatalf("ReadRune = %q, %d, %v; want EOF sentinel", r, n, err)
	}
}
