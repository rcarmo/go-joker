package main

import (
	"fmt"
	corert "github.com/rcarmo/go-joker/core/runtime"
	"strings"

	. "github.com/rcarmo/go-joker/core"
)

func runEvalMode() bool {
	if eval == "" {
		return false
	}
	if lintFlag {
		fmt.Fprintf(Stderr, "Error: Cannot combine --eval/-e and --lint.\n")
		corert.ExitJoker(6)
	}
	if replFlag {
		fmt.Fprintf(Stderr, "Error: Cannot combine --eval/-e and --repl.\n")
		corert.ExitJoker(7)
	}
	if workingDir != "" {
		fmt.Fprintf(Stderr, "Error: Cannot combine --eval/-e and --working-dir.\n")
		corert.ExitJoker(8)
	}
	if reportGloballyUnusedFlag {
		fmt.Fprintf(Stderr, "Error: Cannot combine --eval/-e and --report-globally-unused.\n")
		corert.ExitJoker(17)
	}
	if filename != "" {
		fmt.Fprintf(Stderr, "Error: Cannot combine --eval/-e and a <filename> argument.\n")
		corert.ExitJoker(9)
	}
	reader := NewReader(strings.NewReader(eval), "<expr>")
	if saveForRepl {
		reader = NewReader(&replayable{reader}, "<replay>")
	}
	if err := ProcessReader(reader, "", phase); err != nil {
		if !errorToRepl {
			corert.ExitJoker(1)
		}
	} else if !exitToRepl {
		return true
	}
	return false
}
