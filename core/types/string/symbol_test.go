package string

import "testing"

func TestIsInteropName(t *testing.T) {
	cases := map[string]bool{
		".foo":    true,
		"foo.":    true,
		"Foo$Bar": true,
		"plain":   false,
	}
	for in, want := range cases {
		if got := IsInteropName(in); got != want {
			t.Fatalf("IsInteropName(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestIsRecordConstructorName(t *testing.T) {
	cases := map[string]bool{
		"->User":    true,
		"map->User": true,
		"User":      false,
	}
	for in, want := range cases {
		if got := IsRecordConstructorName(in); got != want {
			t.Fatalf("IsRecordConstructorName(%q) = %v, want %v", in, got, want)
		}
	}
}
