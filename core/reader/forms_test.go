package reader

import "testing"

func TestMapFormHelpers(t *testing.T) {
	if !ShouldAppendMapCommentSurrogate(true, true) {
		t.Fatal("format comment should append surrogate")
	}
	if ShouldAppendMapCommentSurrogate(false, true) || ShouldAppendMapCommentSurrogate(true, false) {
		t.Fatal("surrogate helper true outside format comments")
	}
	if !HasEvenFormCount(0) || !HasEvenFormCount(2) || HasEvenFormCount(3) {
		t.Fatal("unexpected even form count result")
	}
	if !IsBareArgLiteral(' ') || !IsBareArgLiteral(')') || IsBareArgLiteral('1') {
		t.Fatal("unexpected bare arg literal classification")
	}
	if !ContinueDelimitedForms('x', ')', 0) || !ContinueDelimitedForms(')', ')', 1) || ContinueDelimitedForms(')', ')', 0) {
		t.Fatal("unexpected delimited form continuation result")
	}
	if !NeedsConditionalPair(0, ')', ')') || NeedsConditionalPair(1, ')', ')') || NeedsConditionalPair(0, 'x', ')') {
		t.Fatal("unexpected conditional pair result")
	}
}

func TestFillMissingArgIndexes(t *testing.T) {
	args := map[int]string{1: "a", 3: "c", -1: "rest"}
	n := 0
	FillMissingArgIndexes(args, func() string {
		n++
		return "gen"
	})
	if args[2] != "gen" || args[1] != "a" || args[3] != "c" || args[-1] != "rest" || n != 1 {
		t.Fatalf("FillMissingArgIndexes result = %#v, generated %d", args, n)
	}
}

func TestOrderedArgValues(t *testing.T) {
	got := OrderedArgValues(map[int]string{2: "b", 1: "a", -1: "rest"}, "&")
	want := []string{"a", "b", "&", "rest"}
	if len(got) != len(want) {
		t.Fatalf("OrderedArgValues length = %d, want %d (%#v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("OrderedArgValues[%d] = %q, want %q (%#v)", i, got[i], want[i], got)
		}
	}
}

func TestConditionalSpliceHelpers(t *testing.T) {
	if !IsConditionalSplice('@') || IsConditionalSplice('x') {
		t.Fatal("IsConditionalSplice mismatch")
	}
	if got := ConditionalPrefix(true); got != "#?@" {
		t.Fatalf("ConditionalPrefix splice = %q", got)
	}
	if got := ConditionalPrefix(false); got != "#?" {
		t.Fatalf("ConditionalPrefix plain = %q", got)
	}
}

func TestUnquoteSpliceHelpers(t *testing.T) {
	if !IsUnquoteSplice('@') || IsUnquoteSplice('x') {
		t.Fatal("IsUnquoteSplice mismatch")
	}
	if got := UnquotePrefix(true); got != "~@" {
		t.Fatalf("UnquotePrefix splice = %q", got)
	}
	if got := UnquotePrefix(false); got != "~" {
		t.Fatalf("UnquotePrefix plain = %q", got)
	}
}

func TestNamespacedMapPrefix(t *testing.T) {
	if got := NamespacedMapPrefix(false, "foo"); got != "#:foo" {
		t.Fatalf("NamespacedMapPrefix explicit = %q", got)
	}
	if got := NamespacedMapPrefix(true, "foo"); got != "#::foo" {
		t.Fatalf("NamespacedMapPrefix auto = %q", got)
	}
	if got := NamespacedMapPrefix(true, ""); got != "#::" {
		t.Fatalf("NamespacedMapPrefix current = %q", got)
	}
}

func TestPopLastForm(t *testing.T) {
	last, rest, ok := PopLastForm([]int{1, 2, 3})
	if !ok || last != 3 || len(rest) != 2 || rest[1] != 2 {
		t.Fatalf("PopLastForm = %v/%#v/%v", last, rest, ok)
	}
	if _, rest, ok := PopLastForm([]int{}); ok || len(rest) != 0 {
		t.Fatalf("PopLastForm empty ok=%v rest=%#v", ok, rest)
	}
}
