//go:build amd64 && !purego

package string

import "golang.org/x/sys/cpu"

func init() {
	if cpu.X86.HasAVX2 {
		scanASCII = scanASCIIAVX2
	}
}

//go:noescape
func scanASCIIAVX2(s string) bool
