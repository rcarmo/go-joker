//go:build arm64 && !purego

package string

import (
	"golang.org/x/sys/cpu"
	"reflect"
	"testing"
)

func TestASCIIScanNEONDispatch(t *testing.T) {
	if !cpu.ARM64.HasASIMD {
		t.Skip("ASIMD unavailable; scalar differential tests still run")
	}
	if reflect.ValueOf(scanASCII).Pointer() != reflect.ValueOf(scanASCIINEON).Pointer() {
		t.Fatal("NEON kernel was not selected")
	}
}
