package core

import "io"

// ReaderConstructionAdapter is the narrow root-owned construction surface for
// reader/parser objects and expression nodes. Future core/reader extraction
// should route construction through this surface before moving implementation
// files across package boundaries.
type ReaderConstructionAdapter struct{}

var readerConstruction ReaderConstructionAdapter

func (ReaderConstructionAdapter) NewReader(runeReader io.RuneReader, filename string) *Reader {
	return NewReader(runeReader, filename)
}

func (ReaderConstructionAdapter) Read(reader *Reader) (Object, bool) {
	return Read(reader)
}

func (ReaderConstructionAdapter) TryRead(reader *Reader) (Object, error) {
	return TryRead(reader)
}

func (ReaderConstructionAdapter) LiteralExpr(obj Object) *LiteralExpr {
	return NewLiteralExpr(obj)
}

func (ReaderConstructionAdapter) SurrogateExpr(obj Object) *LiteralExpr {
	return NewSurrogateExpr(obj)
}

func (ReaderConstructionAdapter) VectorExpr(elements []Expr, pos Position) *VectorExpr {
	return &VectorExpr{v: elements, Position: pos}
}

func (ReaderConstructionAdapter) MapExpr(size int, pos Position) *MapExpr {
	return &MapExpr{keys: make([]Expr, size), values: make([]Expr, size), Position: pos}
}

func (ReaderConstructionAdapter) SetExpr(size int, pos Position) *SetExpr {
	return &SetExpr{elements: make([]Expr, size), Position: pos}
}
