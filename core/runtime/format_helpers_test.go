package runtime

import (
	"bytes"
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func TestFormatHelpers(t *testing.T) {
	var buf bytes.Buffer
	if got := PprintObject(coretypes.MakeString("x"), 0, &buf); got != 3 || buf.String() != `"x"` {
		t.Fatalf("PprintObject = %d/%q, want 3/\"x\"", got, buf.String())
	}
	buf.Reset()
	WriteIndent(&buf, 3)
	if buf.String() != "   " {
		t.Fatalf("WriteIndent = %q", buf.String())
	}
	obj := coretypes.Comment{C: ","}
	if !IsComment(obj) || !IsComma(obj) {
		t.Fatal("comment/comma helpers failed")
	}
}
