package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"
	"math/big"
	"regexp"

	corereader "github.com/rcarmo/go-joker/core/reader"
)

// ReaderConstructionAdapter is the narrow root-owned construction surface for
// reader/parser objects and expression nodes. Future core/reader extraction
// should route construction through this surface before moving implementation
// files across package boundaries.
type ReaderConstructionAdapter struct{}

var readerConstruction ReaderConstructionAdapter

func (ReaderConstructionAdapter) NewReader(runeReader io.RuneReader, filename string) *Reader {
	return NewReader(runeReader, filename)
}

func (ReaderConstructionAdapter) Read(reader *Reader) (coretypes.Object, bool) {
	return Read(reader)
}

func (ReaderConstructionAdapter) TryRead(reader *Reader) (coretypes.Object, error) {
	return TryRead(reader)
}

func (ReaderConstructionAdapter) ReadError(reader *Reader, msg string) ReadError {
	return makeReadError(reader, msg)
}

func (ReaderConstructionAdapter) ReadObject(reader *Reader, obj coretypes.Object) coretypes.Object {
	return makeReadObject(reader, obj)
}

func (ReaderConstructionAdapter) DeriveReadObject(base coretypes.Object, obj coretypes.Object) coretypes.Object {
	return deriveReadObject(base, obj)
}

func (ReaderConstructionAdapter) Nil() coretypes.Object { return NIL }

func (ReaderConstructionAdapter) Bool(v bool) coretypes.Object { return coretypes.Boolean{B: v} }

func (ReaderConstructionAdapter) Char(v rune) coretypes.Object { return coretypes.Char{Ch: v} }

func (ReaderConstructionAdapter) Int(v int) coretypes.Object { return coretypes.Int{I: v} }

func (ReaderConstructionAdapter) String(v string) coretypes.Object { return coretypes.MakeString(v) }

func (ReaderConstructionAdapter) Symbol(v string) coretypes.Object { return MakeSymbol(v) }

func (ReaderConstructionAdapter) Keyword(v string) coretypes.Object { return MakeKeyword(v) }

func (ReaderConstructionAdapter) ListFrom(values []coretypes.Object) coretypes.Object {
	return collectionConstruction.NewListFrom(values...)
}

func (ReaderConstructionAdapter) VectorFrom(values []coretypes.Object) coretypes.Object {
	return collectionConstruction.NewArrayVectorFrom(values...)
}

func (ReaderConstructionAdapter) PersistentVectorFromSeq(seq Seq) coretypes.Object {
	return collectionConstruction.NewVectorFromSeq(seq)
}

func (ReaderConstructionAdapter) MapLiteral(reader *Reader, values []coretypes.Object, nsname string) coretypes.Object {
	if int64(len(values)) >= HASHMAP_THRESHOLD {
		hashMap := collectionConstruction.NewHashMapFrom()
		for i := 0; i < len(values); i += 2 {
			key := resolveKey(values[i], nsname)
			if hashMap.containsKey(key) {
				panic(MakeReadError(reader, "Duplicate key "+key.ToString(false)))
			}
			hashMap = hashMap.Assoc(key, values[i+1]).(*HashMap)
		}
		return hashMap
	}
	m := collectionConstruction.NewEmptyArrayMap()
	for i := 0; i < len(values); i += 2 {
		key := resolveKey(values[i], nsname)
		if !m.Add(key, values[i+1]) {
			panic(MakeReadError(reader, "Duplicate key "+key.ToString(false)))
		}
	}
	return m
}

func (ReaderConstructionAdapter) SetLiteral(reader *Reader, values []coretypes.Object) coretypes.Object {
	set := collectionConstruction.NewEmptySet()
	for _, obj := range values {
		if !set.Add(obj) {
			panic(MakeReadError(reader, "Duplicate set element "+obj.ToString(false)))
		}
	}
	return set
}

func (ReaderConstructionAdapter) Double(v float64) coretypes.Object { return coretypes.MakeDouble(v) }

func (ReaderConstructionAdapter) BigInt(v *big.Int, original string) coretypes.Object {
	return &coretypes.BigInt{B: v, Original: original}
}

func (ReaderConstructionAdapter) BigFloatFromString(value string, original string) (coretypes.Object, bool) {
	return coretypes.MakeBigFloatWithOrig(value, original)
}

func (ReaderConstructionAdapter) RatioOrInt(value string, ratio *big.Rat) coretypes.Object {
	return coretypes.RatioOrIntWithOriginal(value, ratio)
}

func (ReaderConstructionAdapter) Comment(v string) coretypes.Object { return coretypes.Comment{C: v} }

func (ReaderConstructionAdapter) Regex(v *regexp.Regexp) coretypes.Object {
	return coretypes.MakeRegex(v)
}

func (ReaderConstructionAdapter) NumberFromToken(reader *Reader, token corereader.NumberToken) coretypes.Object {
	return numberFromToken(reader, token)
}

func (ReaderConstructionAdapter) MetadataFromObject(obj coretypes.Object) (*ArrayMap, bool) {
	return metadataFromObject(obj)
}

func (ReaderConstructionAdapter) WithMeta(obj coretypes.Object, meta *ArrayMap) (coretypes.Object, bool) {
	v, ok := obj.(Meta)
	if !ok {
		return nil, false
	}
	return deriveReadObject(obj, v.WithMeta(meta)), true
}

func (ReaderConstructionAdapter) SkipRedundantDoMeta() *ArrayMap {
	return collectionConstruction.NewEmptyArrayMap().Plus(MakeKeyword("skip-redundant-do"), coretypes.Boolean{B: true})
}

func (ReaderConstructionAdapter) LiteralExpr(obj coretypes.Object) *LiteralExpr {
	return NewLiteralExpr(obj)
}

func (ReaderConstructionAdapter) SurrogateExpr(obj coretypes.Object) *LiteralExpr {
	return NewSurrogateExpr(obj)
}

func (ReaderConstructionAdapter) VectorExpr(elements []Expr, pos coretypes.Position) *VectorExpr {
	return &VectorExpr{v: elements, Position: pos}
}

func (ReaderConstructionAdapter) MapExpr(size int, pos coretypes.Position) *MapExpr {
	return &MapExpr{keys: make([]Expr, size), values: make([]Expr, size), Position: pos}
}

func (ReaderConstructionAdapter) SetExpr(size int, pos coretypes.Position) *SetExpr {
	return &SetExpr{elements: make([]Expr, size), Position: pos}
}

func (ReaderConstructionAdapter) SetExprFrom(elements []Expr, pos coretypes.Position) *SetExpr {
	return &SetExpr{elements: elements, Position: pos}
}

func (ReaderConstructionAdapter) MapExprFrom(keys []Expr, values []Expr, pos coretypes.Position) *MapExpr {
	return &MapExpr{keys: keys, values: values, Position: pos}
}
