package markdown

import (
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestConvertStringRendersMarkdown(t *testing.T) {
	got := convertString("# Title\n\nhello")
	if !strings.Contains(got, "<h1") || !strings.Contains(got, "Title") || !strings.Contains(got, "hello") {
		t.Fatalf("unexpected markdown output: %q", got)
	}
}

func TestConvertStringOptsCanDisableUnsafeHTML(t *testing.T) {
	opts := EmptyArrayMap()
	opts.Add(MakeKeyword("with-hard-wraps?"), Boolean{B: true})
	opts.Add(MakeKeyword("with-xhtml?"), Boolean{B: true})
	opts.Add(MakeKeyword("with-unsafe?"), Boolean{B: false})
	got := convertStringOpts("<script>alert(1)</script>", opts)
	if strings.Contains(got, "<script>") {
		t.Fatalf("unsafe HTML was not escaped: %q", got)
	}
}
