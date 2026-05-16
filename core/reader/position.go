package reader

// Position is the reader package's root-independent source location snapshot.
type Position struct {
	Line   int
	Column int
}

// PositionStack tracks start positions for nested reader forms without knowing
// about root core Object metadata.
type PositionStack struct {
	items []Position
}

func NewPositionStack(capacity int) PositionStack {
	if capacity < 0 {
		capacity = 0
	}
	return PositionStack{items: make([]Position, 0, capacity)}
}

func (s *PositionStack) Push(p Position) {
	s.items = append(s.items, p)
}

func (s *PositionStack) Pop() (Position, bool) {
	if len(s.items) == 0 {
		return Position{}, false
	}
	p := s.items[len(s.items)-1]
	s.items = s.items[:len(s.items)-1]
	return p, true
}

func (s *PositionStack) Len() int { return len(s.items) }
