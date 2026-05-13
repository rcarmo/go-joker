package osutil

import "path/filepath"

// ClassPathElements splits a Go/Joker classpath string using the host path-list
// separator and returns a default empty element when no entries are present.
func ClassPathElements(cp string) []string {
	elems := filepath.SplitList(cp)
	if len(elems) == 0 {
		return []string{""}
	}
	return elems
}
