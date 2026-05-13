package deps

import (
	"path/filepath"
	"strings"
)

// LibNamePath converts a dotted Joker lib name into a relative source path.
func LibNamePath(lib string) string {
	return filepath.Join(strings.Split(lib, ".")...) + ".joke"
}

// ResolveLibPath joins a base directory with a dotted Joker lib name.
func ResolveLibPath(base, lib string) string {
	if base == "" {
		return LibNamePath(lib)
	}
	return filepath.Join(base, LibNamePath(lib))
}
