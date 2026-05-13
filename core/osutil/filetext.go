package osutil

import (
	"io"
	"os"
	"path/filepath"
)

// OpenRuneFile opens a file and returns both the open handle and a rune reader
// view suitable for Joker reader setup.
func OpenRuneFile(path string) (*os.File, io.RuneReader, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	return f, AsRuneReader(f), nil
}

// ExistingChild joins dir/name and returns the path only if it exists.
func ExistingChild(dir, name string) string {
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	return ""
}
