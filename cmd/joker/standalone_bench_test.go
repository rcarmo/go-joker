package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Measures the public standalone-packaging path, including read/write/metadata.
// Keep fixed counts bounded: the runtime is a large executable, not a tiny fixture.
func BenchmarkStandalonePackaging(b *testing.B) {
	dir := b.TempDir()
	source := filepath.Join(dir, "program.joke")
	output := filepath.Join(dir, "program")
	if err := os.WriteFile(source, []byte("(println 42)"), 0600); err != nil {
		b.Fatal(err)
	}
	for _, engine := range []string{"legacy", "compiler"} {
		b.Run(engine, func(b *testing.B) {
			mode := engine
			if mode == "legacy" {
				mode = ""
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := compileStandaloneEngine(source, output, mode); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
