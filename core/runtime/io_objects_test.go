package runtime

import (
	"bytes"
	"strings"
	"testing"
)

func TestIOObjectWrappers(t *testing.T) {
	buf := MakeBuffer(bytes.NewBufferString("abc"))
	if got := buf.ToString(false); got != "abc" {
		t.Fatalf("Buffer ToString = %q, want abc", got)
	}
	reader := MakeIOReader(strings.NewReader("abc"))
	if reader.ToString(false) != "#object[IOReader]" {
		t.Fatalf("unexpected IOReader string: %s", reader.ToString(false))
	}
	buffered := MakeBufferedReader(strings.NewReader("abc"))
	if buffered.ToString(false) != "#object[BufferedReader]" {
		t.Fatalf("unexpected BufferedReader string: %s", buffered.ToString(false))
	}
	writer := MakeIOWriter(&bytes.Buffer{})
	if writer.ToString(false) != "#object[IOWriter]" {
		t.Fatalf("unexpected IOWriter string: %s", writer.ToString(false))
	}
}
