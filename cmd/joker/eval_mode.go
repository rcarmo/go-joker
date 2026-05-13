package main

import (
	"fmt"
	"strings"

	. "github.com/rcarmo/go-joker/core"
)

func runEvalMode() bool {
	if eval == "" {
		return false
	}
	if lintFlag {
		fmt.Fprintf(Stderr, "Error: Cannot combine --eval/-e and --lint.\n")
		ExitJoker(6)
	}
	if replFlag {
		fmt.Fprintf(Stderr, "Error: Cannot combine --eval/-e and --repl.\n")
		ExitJoker(7)
	}
	if workingDir != "" {
		fmt.Fprintf(Stderr, "Error: Cannot combine --eval/-e and --working-dir.\n")
		ExitJoker(8)
	}
	if reportGloballyUnusedFlag {
		fmt.Fprintf(Stderr, "Error: Cannot combine --eval/-e and --report-globally-unused.\n")
		ExitJoker(17)
	}
	if filename != "" {
		fmt.Fprintf(Stderr, "Error: Cannot combine --eval/-e and a <filename> argument.\n")
		ExitJoker(9)
	}
	reader := NewReader(strings.NewReader(eval), "<expr>")
	if saveForRepl {
		reader = NewReader(&replayable{reader}, "<replay>")
	}
	if err := ProcessReader(reader, "", phase); err != nil {
		if !errorToRepl {
			ExitJoker(1)
		}
	} else if !exitToRepl {
		return true
	}
	return false
}
