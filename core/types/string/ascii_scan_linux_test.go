//go:build linux

package string

import (
	"golang.org/x/sys/unix"
	"testing"
	"unsafe"
)

func TestASCIIScanGuardPage(t *testing.T) {
	page := unix.Getpagesize()
	mem, err := unix.Mmap(-1, 0, page*2, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := unix.Munmap(mem); err != nil {
			t.Error(err)
		}
	}()
	if err = unix.Mprotect(mem[page:], unix.PROT_NONE); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < page; i++ {
		mem[i] = 'a'
	}
	for n := 1; n <= 257; n++ {
		s := unsafe.String(&mem[page-n], n)
		if !scanASCII(s) {
			t.Fatalf("ASCII rejected at len %d", n)
		}
		mem[page-1] = 0xff
		if scanASCII(s) {
			t.Fatalf("high byte accepted at len %d", n)
		}
		mem[page-1] = 'a'
	}
}
