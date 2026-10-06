package main

import (
	"fmt"
	corert "github.com/rcarmo/go-joker/v42/core/runtime"
	corewasm "github.com/rcarmo/go-joker/v42/core/wasm"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	. "github.com/rcarmo/go-joker/v42/core"
)

func handleCompile(args []string) { handleCompileEngine(args, "") }
func handleCompileEngine(args []string, engine string) {
	run := false
	var scriptArgs []string
	var sourceFile, outputFile string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--native":
			engine = "compiler"
		case "--run":
			run = true
		case "--":
			scriptArgs = args[i+1:]
			i = len(args)
		case "-o", "--output":
			if i+1 < len(args) {
				i++
				outputFile = args[i]
			} else {
				fmt.Fprintln(Stderr, "Error: -o requires an argument")
				corert.ExitJoker(1)
			}
		default:
			if sourceFile == "" {
				sourceFile = args[i]
			} else {
				fmt.Fprintf(Stderr, "Error: unexpected argument: %s\n", args[i])
				corert.ExitJoker(1)
			}
		}
	}

	if sourceFile == "" {
		fmt.Fprintln(Stderr, "Usage: joker compile <source.clj> -o <output>")
		corert.ExitJoker(1)
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

	if engine == "" {
		engine = os.Getenv("JOKER_WASM_ENGINE")
	}
	if engine != "" {
		var err error
		engine, err = corewasm.NormalizeEngine(engine)
		if err != nil {
			fmt.Fprintln(Stderr, "Error:", err)
			corert.ExitJoker(1)
		}
	}
	if err := compileStandaloneEngine(sourceFile, outputFile, engine); err != nil {
		fmt.Fprintf(Stderr, "Error: %v\n", err)
		corert.ExitJoker(1)
	}

	// Report size when available; do not panic if the output vanished or stat fails.
	fi, err := os.Stat(outputFile)
	if err != nil {
		fmt.Fprintf(Stdout, "Compiled %s → %s\n", sourceFile, outputFile)
		fmt.Fprintf(Stderr, "Warning: could not stat output file %s: %v\n", outputFile, err)
		return
	}
	fmt.Fprintf(Stdout, "Compiled %s → %s (%s)\n", sourceFile, outputFile, humanSize(fi.Size()))
	fmt.Fprintln(Stdout, "Standalone native runtime; eligible WASM functions use the selected engine. Source is bundled, not whole-program AOT.")
	if run {
		path, err := filepath.Abs(outputFile)
		if err != nil {
			fmt.Fprintln(Stderr, err)
			corert.ExitJoker(1)
		}
		if len(scriptArgs) > 0 {
			scriptArgs = append([]string{"--"}, scriptArgs...)
		}
		cmd := exec.Command(path, scriptArgs...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = Stdout
		cmd.Stderr = Stderr
		if err = cmd.Run(); err != nil {
			if e, ok := err.(*exec.ExitError); ok {
				corert.ExitJoker(e.ExitCode())
			}
			fmt.Fprintln(Stderr, err)
			corert.ExitJoker(1)
		}
	}
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
