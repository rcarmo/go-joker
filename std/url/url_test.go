package url

import (
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func expectURLPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestURLHelpers(t *testing.T) {
	if got := pathUnescape("a%2Fb"); got != "a/b" {
		t.Fatalf("pathUnescape mismatch: %s", got)
	}
	if got := queryUnescape("a+b%21"); got != "a b!" {
		t.Fatalf("queryUnescape mismatch: %s", got)
	}
	m := parseQuery("a=1&a=2&b=x").(Map)
	ok, av := m.Get(MakeString("a"))
	if !ok || av.(CountedIndexed).Count() != 2 {
		t.Fatalf("parseQuery a mismatch: %v", av)
	}
	ok, b := m.Get(MakeString("b"))
	if !ok || b.(CountedIndexed).At(0).ToString(false) != "x" {
		t.Fatalf("parseQuery b mismatch: %v", b)
	}
}

func TestParseQueryRejectsMalformedEscape(t *testing.T) {
	expectURLPanic(t, func() { parseQuery("bad=%zz") })
}
