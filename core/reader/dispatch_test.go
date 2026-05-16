package reader

import "testing"

func TestDispatchFormatPrefix(t *testing.T) {
	cases := map[DispatchKind]string{
		DispatchVar:     "#'",
		DispatchDiscard: "#_",
		DispatchMeta:    "#^",
		DispatchFn:      "#",
	}
	for kind, want := range cases {
		got, ok := DispatchFormatPrefix(kind)
		if !ok || got != want {
			t.Fatalf("DispatchFormatPrefix(%v) = %q, %v; want %q, true", kind, got, ok, want)
		}
	}
	if got, ok := DispatchFormatPrefix(DispatchSet); ok || got != "" {
		t.Fatalf("DispatchFormatPrefix(DispatchSet) = %q, %v; want empty, false", got, ok)
	}
}

func TestClassifyDispatch(t *testing.T) {
	tests := map[rune]DispatchKind{
		'"':  DispatchRegex,
		'\'': DispatchVar,
		'_':  DispatchDiscard,
		'^':  DispatchMeta,
		'{':  DispatchSet,
		'(':  DispatchFn,
		'?':  DispatchConditional,
		':':  DispatchNamespacedMap,
		'#':  DispatchSymbolicValue,
		'a':  DispatchTagged,
	}
	for r, want := range tests {
		if got := ClassifyDispatch(r); got != want {
			t.Fatalf("ClassifyDispatch(%q) = %v, want %v", r, got, want)
		}
	}
}
