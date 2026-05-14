package core

import (
	"io"
	"strings"
	"testing"
)

func TestReaderConstructionAdapterReaderSurface(t *testing.T) {
	adapter := ReaderConstructionAdapter{}
	reader := adapter.NewReader(strings.NewReader("[1 2]"), "<adapter>")
	obj, err := adapter.TryRead(reader)
	if err != nil {
		t.Fatalf("TryRead via adapter: %v", err)
	}
	vec := obj.(CountedIndexed)
	if vec.Count() != 2 || !vec.At(0).Equals(MakeInt(1)) || obj.GetInfo().Filename() != "<adapter>" {
		t.Fatalf("adapter reader result mismatch: %s info=%#v", obj.ToString(false), obj.GetInfo())
	}
	if _, err := adapter.TryRead(reader); err != io.EOF {
		t.Fatalf("adapter TryRead should reach EOF, got %v", err)
	}
}

func TestReaderConstructionAdapterExpressionSurface(t *testing.T) {
	adapter := ReaderConstructionAdapter{}
	obj := MakeString("literal")
	lit := adapter.LiteralExpr(obj)
	if lit.obj != obj || lit.isSurrogate {
		t.Fatalf("LiteralExpr mismatch: %#v", lit)
	}
	surrogate := adapter.SurrogateExpr(obj)
	if surrogate.obj != obj || !surrogate.isSurrogate {
		t.Fatalf("SurrogateExpr mismatch: %#v", surrogate)
	}
	pos := Position{startLine: 1, startColumn: 2, endLine: 1, endColumn: 3}
	vec := adapter.VectorExpr([]Expr{lit}, pos)
	if len(vec.v) != 1 || vec.Position != pos {
		t.Fatalf("VectorExpr mismatch: %#v", vec)
	}
	m := adapter.MapExpr(2, pos)
	if len(m.keys) != 2 || len(m.values) != 2 || m.Position != pos {
		t.Fatalf("MapExpr mismatch: %#v", m)
	}
	set := adapter.SetExpr(3, pos)
	if len(set.elements) != 3 || set.Position != pos {
		t.Fatalf("SetExpr mismatch: %#v", set)
	}
}
