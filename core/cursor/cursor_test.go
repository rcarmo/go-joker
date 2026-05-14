package cursor

import "testing"

func TestCursorASCIIAndUnicode(t *testing.T) {
	cur := New("aπ")
	if cur.Done() || cur.Char() != 'a' || cur.Index() != 0 {
		t.Fatalf("initial cursor = done %v char %q index %d", cur.Done(), cur.Char(), cur.Index())
	}
	next := cur.Next()
	if next == cur || next.Done() || next.Char() != 'π' || next.Index() != 1 {
		t.Fatalf("next cursor = same %v done %v char %q index %d", next == cur, next.Done(), next.Char(), next.Index())
	}
	done := next.Next()
	if !done.Done() || done.Char() != -1 || done.Index() != 2 {
		t.Fatalf("done cursor = done %v char %q index %d", done.Done(), done.Char(), done.Index())
	}
	if done.Next() != done {
		t.Fatal("advancing an exhausted cursor should be stable")
	}
}

func TestCursorIdentityAndHash(t *testing.T) {
	cur := New("abc")
	same := New("abc")
	if !cur.Equal(same) || cur.Hash() != same.Hash() {
		t.Fatalf("equal cursors should compare and hash equally")
	}
	if cur.Equal(cur.Next()) {
		t.Fatal("different offsets should not compare equal")
	}
	if cur.String() != "#<StringCursor>" {
		t.Fatalf("String = %q", cur.String())
	}
}
