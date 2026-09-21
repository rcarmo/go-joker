//go:build amd64 && !purego

package string

import (
	"golang.org/x/sys/cpu"
	"reflect"
	"testing"
)

func TestASCIIScanAVX2Dispatch(t *testing.T) {
	if !cpu.X86.HasAVX2 {
		t.Skip("AVX2 unavailable; scalar differential tests still run")
	}
	if reflect.ValueOf(scanASCII).Pointer() != reflect.ValueOf(scanASCIIAVX2).Pointer() {
		t.Fatal("AVX2 kernel was not selected")
	}
}
