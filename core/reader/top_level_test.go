package reader

import "testing"

func TestClassifyTopLevelTrivia(t *testing.T) {
	cases := []struct {
		r, peek rune
		want    TopLevelTriviaKind
	}{
		{',', 'x', TopLevelTriviaComma},
		{';', 'x', TopLevelTriviaComment},
		{'#', '!', TopLevelTriviaComment},
		{'#', '{', TopLevelTriviaNone},
		{'a', 'b', TopLevelTriviaNone},
	}
	for _, tc := range cases {
		if got := ClassifyTopLevelTrivia(tc.r, tc.peek); got != tc.want {
			t.Fatalf("ClassifyTopLevelTrivia(%q, %q) = %v, want %v", tc.r, tc.peek, got, tc.want)
		}
	}
}
