package reader

import "io"

// RuneStream owns the root-independent rune window and source position
// mechanics used by the reader. Root core still owns filename interning and
// conversion of read errors into core exceptions.
type RuneStream struct {
	runeReader     io.RuneReader
	rw             *Window
	line           int
	prevLineLength int
	column         int
	isEof          bool
	rewind         int
	onError        func(error)
}

func NewRuneStream(runeReader io.RuneReader, onError func(error)) *RuneStream {
	return &RuneStream{
		line:       1,
		runeReader: runeReader,
		rw:         &Window{},
		rewind:     -1,
		onError:    onError,
	}
}

func (s *RuneStream) Line() int   { return s.line }
func (s *RuneStream) Column() int { return s.column }

func (s *RuneStream) Get() rune {
	if s.isEof {
		return EOF
	}
	if s.rewind > -1 {
		r := s.rw.Top(s.rewind)
		s.rewind--
		s.advancePosition(r)
		return r
	}
	r, _, err := s.runeReader.ReadRune()
	switch {
	case err == io.EOF:
		s.isEof = true
		return EOF
	case err != nil:
		if s.onError != nil {
			s.onError(err)
		}
		panic("reader stream error: " + err.Error())
	case r == '\n':
		s.advancePosition(r)
		s.rw.Add(r)
		return r
	default:
		s.advancePosition(r)
		s.rw.Add(r)
		return r
	}
}

func (s *RuneStream) Unget() {
	if s.isEof {
		return
	}
	s.rewind++
	if s.column == 0 {
		s.line--
		s.column = s.prevLineLength
	} else {
		s.column--
	}
}

func (s *RuneStream) Peek() rune {
	if s.isEof {
		return EOF
	}
	r := s.Get()
	s.Unget()
	return r
}

func (s *RuneStream) advancePosition(r rune) {
	if r == '\n' {
		s.line++
		s.prevLineLength = s.column
		s.column = 0
		return
	}
	s.column++
}
