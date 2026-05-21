package runtime

import (
	"strings"
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type testTraceable struct {
	name string
	pos  coretypes.Position
}

func (t testTraceable) Name() string            { return t.name }
func (t testTraceable) Pos() coretypes.Position { return t.pos }

func TestCallstackCloneAndStacktrace(t *testing.T) {
	filename := "trace.joke"
	stack := NewCallstack(1)
	frame := testTraceable{name: "'joker.core/f", pos: coretypes.Position{Filename: &filename, StartLine: 3, StartColumn: 4}}
	current := testTraceable{name: "joker.core/g", pos: coretypes.Position{Filename: &filename, StartLine: 5, StartColumn: 6}}
	stack.Push(frame)
	clone := stack.Clone()
	stack.Pop()
	if stack.Len() != 0 {
		t.Fatalf("original stack Len = %d, want 0", stack.Len())
	}
	if clone.Len() != 1 {
		t.Fatalf("clone Len = %d, want 1", clone.Len())
	}
	if got := clone.FirstPos().StartLine; got != 3 {
		t.Fatalf("FirstPos line = %d, want 3", got)
	}
	trace := clone.Stacktrace(current)
	for _, want := range []string{"global trace.joke:3:4", "joker.core/f trace.joke:5:6"} {
		if !strings.Contains(trace, want) {
			t.Fatalf("Stacktrace() = %q, want substring %q", trace, want)
		}
	}
}
