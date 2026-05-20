package types

// MetadataFactory builds metadata values for documented runtime entities.
type MetadataFactory func(doc string, name string) any

func TypeMetadataDoc(kind Kind, doc string) string {
	if doc != "" {
		doc = "\n  " + doc
	}
	return kind.DocumentationPrefix() + doc
}

func WithInfo(obj Object, info *ObjectInfo) Object {
	if h, ok := obj.(interface {
		WithInfo(*ObjectInfo) Object
	}); ok {
		return h.WithInfo(info)
	}
	return obj
}

func RootObject(obj Object) Object { return obj.(Object) }

// Position describes source coordinates for reader/parser/runtime diagnostics.
type Position struct {
	EndLine     int
	EndColumn   int
	StartLine   int
	StartColumn int
	Filename    *string
}

func (pos Position) FilenameOrUnknown() string {
	if pos.Filename == nil {
		return "<unknown>"
	}
	return *pos.Filename
}

func (pos Position) Pos() Position { return pos }

// ObjectInfo stores source metadata plus optional reader prefix text.
type ObjectInfo struct {
	Prefix string
	Position
}

// InfoHolder embeds optional source metadata in runtime objects.
type InfoHolder struct {
	Info *ObjectInfo
}

func (h InfoHolder) GetInfo() *ObjectInfo { return h.Info }
