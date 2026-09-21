package string

import "testing"

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
