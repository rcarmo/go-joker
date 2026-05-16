package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/rcarmo/go-joker/core"
)

func handleCompile(args []string) {
	var sourceFile, outputFile string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--output":
			if i+1 < len(args) {
				i++
				outputFile = args[i]
			} else {
				fmt.Fprintln(Stderr, "Error: -o requires an argument")
				ExitJoker(1)
			}
		default:
			if sourceFile == "" {
				sourceFile = args[i]
			} else {
				fmt.Fprintf(Stderr, "Error: unexpected argument: %s\n", args[i])
				ExitJoker(1)
			}
		}
	}

	if sourceFile == "" {
		fmt.Fprintln(Stderr, "Usage: joker compile <source.clj> -o <output>")
		ExitJoker(1)
	}

	if outputFile == "" {
		// Default: strip extension and add platform suffix
		ext := filepath.Ext(sourceFile)
		base := strings.TrimSuffix(sourceFile, ext)
		outputFile = base
		if runtime.GOOS == "windows" {
			outputFile += ".exe"
		}
	}

	if err := compileStandalone(sourceFile, outputFile); err != nil {
		fmt.Fprintf(Stderr, "Error: %v\n", err)
		ExitJoker(1)
	}

	// Report size when available; do not panic if the output vanished or stat fails.
	fi, err := os.Stat(outputFile)
	if err != nil {
		fmt.Fprintf(Stdout, "Compiled %s → %s\n", sourceFile, outputFile)
		fmt.Fprintf(Stderr, "Warning: could not stat output file %s: %v\n", outputFile, err)
		return
	}
	fmt.Fprintf(Stdout, "Compiled %s → %s (%s)\n", sourceFile, outputFile, humanSize(fi.Size()))
}

func humanSize(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}
