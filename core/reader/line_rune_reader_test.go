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
	defer lrr.rl.Close()
	r, n, err := lrr.ReadRune()
	if err != io.EOF || r != -1 || n != 0 {
		t.Fatalf("ReadRune = %q, %d, %v; want EOF sentinel", r, n, err)
	}
}
