package main

import (
	"fmt"
	"os"

	. "github.com/rcarmo/go-joker/core"
)

func main() {
	OnExit(finish)

	// Handle compile subcommand before embedded source check
	if len(os.Args) >= 2 && os.Args[1] == "compile" {
		handleCompile(os.Args[2:])
		return
	}

	// Check for embedded standalone payload before anything else
	if src, ok := checkEmbeddedSource(); ok {
		runEmbeddedSource(src)
		return
	}

	initRuntime()
	dumpDebugState()

	if helpFlag {
		usage(Stdout)
		return
	}

	if versionFlag {
		println(VERSION)
		return
	}

	validateRemainingArgs()

	if err := startProfiling(); err != nil {
		fmt.Fprintln(Stderr, err)
		ExitJoker(96)
	}

	if runEvalMode() {
		return
	}

	if runLintMode() {
		return
	}

	if runFileMode() {
		return
	}

	if replSocket != "" {
		srepl(replSocket, phase)
		return
	}

	repl(phase)
	return
}
