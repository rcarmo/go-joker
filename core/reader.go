package core

import (
	"io"

	"github.com/rcarmo/go-joker/core/internal/runewindow"
)

type (
	Reader struct {
		runeReader     io.RuneReader
		rw             *runewindow.Window
		line           int
		prevLineLength int
		column         int
		isEof          bool
		rewind         int
		filename       *string
	}
)

func NewReader(runeReader io.RuneReader, filename string) *Reader {
	return &Reader{
		line:       1,
		runeReader: runeReader,
		rw:         &runewindow.Window{},
		rewind:     -1,
		filename:   STRINGS.Intern(filename),
	}
}

func (reader *Reader) Get() rune {
	if reader.isEof {
		return EOF
	}
	if reader.rewind > -1 {
		r := reader.rw.Top(reader.rewind)
		reader.rewind--
		if r == '\n' {
			reader.line++
			reader.prevLineLength = reader.column
			reader.column = 0
		} else {
			reader.column++
		}
		return r
	}
	r, _, err := reader.runeReader.ReadRune()
	switch {
	case err == io.EOF:
		reader.isEof = true
		return EOF
	case err != nil:
		panic(RT.NewError(err.Error()))
	case r == '\n':
		reader.line++
		reader.prevLineLength = reader.column
		reader.column = 0
		reader.rw.Add(r)
		return r
	default:
		reader.column++
		reader.rw.Add(r)
		return r
	}
}

func (reader *Reader) Unget() {
	if reader.isEof {
		return
	}
	reader.rewind++
	if reader.column == 0 {
		reader.line--
		reader.column = reader.prevLineLength
	} else {
		reader.column--
	}
}

func (reader *Reader) Peek() rune {
	if reader.isEof {
		return EOF
	}
	r := reader.Get()
	reader.Unget()
	return r
}
