package runtime

import (
	"strings"
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func TestEvalErrorFormatting(t *testing.T) {
	filename := "err.joke"
	state := NewGoroutineRT(1)
	state.Callstack.Push(testTraceable{name: "'joker.core/f", pos: coretypes.Position{Filename: &filename, StartLine: 1, StartColumn: 2}})
	err := NewEvalError("boom", coretypes.Position{Filename: &filename, StartLine: 3, StartColumn: 4}, state, false)
	text := err.Error()
	for _, want := range []string{"err.joke:3:4: Eval error: boom", "Stacktrace:", "global err.joke:1:2"} {
		if !strings.Contains(text, want) {
			t.Fatalf("Error() = %q, want substring %q", text, want)
		}
	}
	if got := err.Message().(coretypes.String).S; got != "boom" {
		t.Fatalf("Message = %q, want boom", got)
	}
}
