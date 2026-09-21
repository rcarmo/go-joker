package string

import (
	"strings"
	"sync"
	"testing"
)

func TestASCIIScanDifferential(t *testing.T) {
	for offset := 0; offset < 32; offset++ {
		for n := 0; n <= 257; n++ {
			buf := make([]byte, offset+n)
			for i := range buf {
				buf[i] = 'a'
			}
			check := func() {
				s := string(buf)[offset:]
				if got, want := scanASCII(s), scanASCIIScalar(s); got != want {
					t.Fatalf("offset=%d len=%d got=%v want=%v", offset, n, got, want)
				}
			}
			check()
			for i := offset; i < len(buf); i++ {
				buf[i] = 0x80
				check()
				buf[i] = 'a'
			}
		}
	}
}

func FuzzASCIIScan(f *testing.F) {
	for _, s := range []string{"", "ascii", "é", "\xff", "0123456789012345678901234567890123"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if scanASCII(s) != scanASCIIScalar(s) {
			t.Fatalf("mismatch for %q", s)
		}
	})
}

func TestASCIIClassificationConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 128; n++ {
				for _, s := range []string{strings.Repeat("a", n), strings.Repeat("b", n) + "é"} {
					if IsASCII(s) != scanASCIIScalar(s) {
						t.Errorf("classification mismatch")
					}
				}
			}
		}()
	}
	wg.Wait()
}

func TestASCIIScanKernelsDoNotAllocate(t *testing.T) {
	samples := []string{strings.Repeat("a", 4096), strings.Repeat("a", 4095) + "é", "", "\xff"}
	for name, kernel := range map[string]func(string) bool{"scalar": scanASCIIScalar, "selected": scanASCII} {
		for _, s := range samples {
			want := scanASCIIScalar(s)
			allocations := testing.AllocsPerRun(1000, func() {
				if kernel(s) != want {
					panic("classification mismatch")
				}
			})
			if allocations != 0 {
				t.Fatalf("%s allocated %g times for %d bytes", name, allocations, len(s))
			}
		}
	}
}
