package runtime

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func TestStringCursorAndTransientString(t *testing.T) {
	cursor := NewStringCursor("åb")
	if cursor.Done() || cursor.Char() != 'å' || cursor.Index() != 0 {
		t.Fatalf("initial cursor = done:%v char:%q index:%d", cursor.Done(), cursor.Char(), cursor.Index())
	}
	next := cursor.Next()
	if next == cursor || next.Char() != 'b' || next.Index() <= cursor.Index() {
		t.Fatalf("next cursor mismatch: %#v -> %#v", cursor, next)
	}
	builder := NewTransientString(coretypes.MakeString("b")).(*TransientString)
	builder.PrependString("a").AppendChar('c')
	if got := builder.ToPersistent(); got.S != "abc" {
		t.Fatalf("persistent string = %q, want abc", got.S)
	}
}
