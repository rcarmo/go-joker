package main

import "testing"

func TestHumanSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{bytes: 0, want: "0 B"},
		{bytes: 1023, want: "1023 B"},
		{bytes: 1024, want: "1.0 KB"},
		{bytes: 1536, want: "1.5 KB"},
		{bytes: 5 * 1024 * 1024, want: "5.0 MB"},
	}

	for _, tt := range tests {
		if got := humanSize(tt.bytes); got != tt.want {
			t.Fatalf("humanSize(%d) = %q, want %q", tt.bytes, got, tt.want)
		}
	}
}
