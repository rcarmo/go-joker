package main

import (
	"strings"
	"testing"
)

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return len(p) - 1, nil
}

func TestWriteStandaloneChunkDetectsShortWrite(t *testing.T) {
	err := writeStandaloneChunk(shortWriter{}, "test", []byte("abc"))
	if err == nil || !strings.Contains(err.Error(), "short write") {
		t.Fatalf("writeStandaloneChunk error = %v, want short write", err)
	}
}

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
