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

func (ReaderConstructionAdapter) ReadError(reader *Reader, msg string) ReadError {
	return makeReadError(reader, msg)
}

func (ReaderConstructionAdapter) ReadObject(reader *Reader, obj Object) Object {
	return makeReadObject(reader, obj)
}

func (ReaderConstructionAdapter) DeriveReadObject(base Object, obj Object) Object {
	return deriveReadObject(base, obj)
}

func (ReaderConstructionAdapter) Nil() Object { return NIL }

func (ReaderConstructionAdapter) Bool(v bool) Object { return Boolean{B: v} }

func (ReaderConstructionAdapter) Char(v rune) Object { return Char{Ch: v} }

func (ReaderConstructionAdapter) String(v string) Object { return MakeString(v) }

func (ReaderConstructionAdapter) Symbol(v string) Object { return MakeSymbol(v) }

func (ReaderConstructionAdapter) Keyword(v string) Object { return MakeKeyword(v) }

func (ReaderConstructionAdapter) ListFrom(values []Object) Object {
	list := EmptyList
	for i := len(values) - 1; i >= 0; i-- {
		list = list.conj(values[i])
	}
	return list
}

func (ReaderConstructionAdapter) VectorFrom(values []Object) Object {
	return collectionConstruction.ArrayVectorFrom(values...)
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

func (ReaderConstructionAdapter) SetExprFrom(elements []Expr, pos Position) *SetExpr {
	return &SetExpr{elements: elements, Position: pos}
}

func (ReaderConstructionAdapter) MapExprFrom(keys []Expr, values []Expr, pos Position) *MapExpr {
	return &MapExpr{keys: keys, values: values, Position: pos}
}
