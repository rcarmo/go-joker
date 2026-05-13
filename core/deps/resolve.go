package deps

import (
	"path/filepath"

	corestr "github.com/rcarmo/go-joker/core/string"
)

// ResolveRelativeLibPath resolves a dotted library name relative to the file
// that is currently being evaluated in the given namespace.
func ResolveRelativeLibPath(currentFile, currentNamespace, libName string) string {
	base := currentFile
	levels := len(corestr.Split(currentNamespace, '.'))
	base = filepath.Clean(base)
	for range levels {
		base = filepath.Dir(base)
	}
	return ResolveLibPath(base, libName)
}
