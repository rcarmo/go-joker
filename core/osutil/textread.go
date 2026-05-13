package osutil

import (
	"bufio"
	"io"
	"strings"
)

type StringLineReader interface {
	ReadString(byte) (string, error)
}

// AsRuneReader upgrades an io.Reader to an io.RuneReader when needed.
func AsRuneReader(r io.Reader) io.RuneReader {
	if rr, ok := r.(io.RuneReader); ok {
		return rr
	}
	return bufio.NewReader(r)
}

// StringRuneReader returns a rune reader over a string.
func StringRuneReader(s string) io.RuneReader {
	return strings.NewReader(s)
}

// ReadLine reads a single line and strips trailing CRLF/LF terminators.
func ReadLine(r StringLineReader) (string, error) {
	s, e := r.ReadString('\n')
	if e == nil {
		l := len(s)
		if s[l-1] == '\n' {
			l--
			if l > 0 && s[l-1] == '\r' {
				l--
			}
		}
		s = s[:l]
	} else if s != "" && e == io.EOF {
		e = nil
	}
	return s, e
}
