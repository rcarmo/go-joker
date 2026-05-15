package reader

import "testing"

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
