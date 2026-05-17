package types

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
