package core

import (
	"io"

	corereader "github.com/rcarmo/go-joker/core/reader"
)

type (
	Reader struct {
		*corereader.RuneStream
		filename *string
	}
)

func NewReader(runeReader io.RuneReader, filename string) *Reader {
	return &Reader{
		RuneStream: corereader.NewRuneStream(runeReader, func(err error) {
			panic(RT.NewError(err.Error()))
		}),
		filename: STRINGS.Intern(filename),
	}
}
