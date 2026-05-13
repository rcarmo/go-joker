package main

import (
	"fmt"

	. "github.com/rcarmo/go-joker/core"
)

func runLintMode() bool {
	if !lintFlag {
		return false
	}
	if replFlag {
		fmt.Fprintf(Stderr, "Error: Cannot combine --lint and --repl.\n")
		ExitJoker(10)
	}
	if exitToRepl {
		fmt.Fprintf(Stderr, "Error: Cannot combine --lint and --exit-to-repl.\n")
		ExitJoker(14)
	}
	if errorToRepl {
		fmt.Fprintf(Stderr, "Error: Cannot combine --lint and --error-to-repl.\n")
		ExitJoker(15)
	}
	if dialect == UNKNOWN {
		dialect = detectDialect(filename)
	}
	if filename != "" {
		lintFile(filename, dialect, workingDir)
	} else if workingDir != "" {
		lintDir(workingDir, dialect, reportGloballyUnusedFlag)
	} else {
		fmt.Fprintf(Stderr, "Error: Missing --file or --working-dir argument.\n")
		ExitJoker(16)
	}
	if PROBLEM_COUNT > 0 {
		ExitJoker(1)
	}
	return true
}

func runFileMode() bool {
	if workingDir != "" {
		fmt.Fprintf(Stderr, "Error: Cannot specify --working-dir option when not linting.\n")
		ExitJoker(11)
	}
	if filename == "" {
		return false
	}
	if err := processFile(filename, phase); err != nil {
		if !errorToRepl {
			ExitJoker(1)
		}
	} else if !exitToRepl {
		return true
	}
	return false
}
