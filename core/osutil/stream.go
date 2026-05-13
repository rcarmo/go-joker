package osutil

import (
	"bytes"
	"io"
)

// ByteRuneReader returns a rune reader over raw bytes.
func ByteRuneReader(data []byte) io.RuneReader {
	return bytes.NewReader(data)
}

// ReadAllString reads an io.Reader fully and returns a string.
func ReadAllString(r io.Reader) (string, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteString writes a string to an io.Writer.
func WriteString(w io.Writer, s string) error {
	_, err := io.WriteString(w, s)
	return err
}
