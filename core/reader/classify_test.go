package reader

import "testing"

func TestClassifyInitialToken(t *testing.T) {
	tests := []struct {
		r        rune
		peek     rune
		dotNum   bool
		wantKind InitialTokenKind
	}{
		{r: '1', wantKind: InitialTokenNumber},
		{r: '-', peek: '9', wantKind: InitialTokenNumber},
		{r: '+', peek: '9', wantKind: InitialTokenNumber},
		{r: '-', peek: 'x', wantKind: InitialTokenIdent},
		{r: '.', peek: '5', dotNum: true, wantKind: InitialTokenNumber},
		{r: '.', peek: '5', dotNum: false, wantKind: InitialTokenIdent},
		{r: 'a', wantKind: InitialTokenIdent},
	}
	for _, tt := range tests {
		if got := ClassifyInitialToken(tt.r, tt.peek, tt.dotNum); got != tt.wantKind {
			t.Fatalf("ClassifyInitialToken(%q, %q, %v) = %v, want %v", tt.r, tt.peek, tt.dotNum, got, tt.wantKind)
		}
	}
}
