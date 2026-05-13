package osutil

import (
	"os"
	"path/filepath"
)

// ResolveSymlink returns the symlink destination when path is a symlink,
// otherwise it returns path unchanged.
func ResolveSymlink(path string) string {
	if linkDest, err := os.Readlink(path); err == nil {
		return linkDest
	}
	return path
}

// ParentDir climbs N directory levels from path.
func ParentDir(path string, levels int) string {
	for range levels {
		next, _ := filepath.Split(path)
		if len(next) == 0 {
			break
		}
		path = next[:len(next)-1]
	}
	return path
}
