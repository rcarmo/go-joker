package markdown

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
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
	opts.Add(coretypes.MakeKeyword(STRINGS.Intern, "with-hard-wraps?"), coretypes.Boolean{B: true})
	opts.Add(coretypes.MakeKeyword(STRINGS.Intern, "with-xhtml?"), coretypes.Boolean{B: true})
	opts.Add(coretypes.MakeKeyword(STRINGS.Intern, "with-unsafe?"), coretypes.Boolean{B: false})
	got := convertStringOpts("<script>alert(1)</script>", opts)
	if strings.Contains(got, "<script>") {
		t.Fatalf("unsafe HTML was not escaped: %q", got)
	}
}
