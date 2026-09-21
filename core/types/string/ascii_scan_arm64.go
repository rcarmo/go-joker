//go:build arm64 && !purego

package string

import "golang.org/x/sys/cpu"

func init() {
	if cpu.ARM64.HasASIMD {
		scanASCII = scanASCIINEON
	}
}

//go:noescape
func scanASCIINEON(s string) bool
