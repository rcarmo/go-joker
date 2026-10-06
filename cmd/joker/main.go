package main

import (
	"fmt"
	corert "github.com/rcarmo/go-joker/v42/core/runtime"
	"os"

	. "github.com/rcarmo/go-joker/v42/core"
)

func main() {
	corert.OnExit(finish)
	args, engine, err := extractWasmEngine(os.Args[1:])
	if err != nil {
		fmt.Fprintln(Stderr, "Error:", err)
		corert.ExitJoker(1)
	}
	os.Args = append([]string{os.Args[0]}, args...)

	// Standalone programs own their argv; host command names are script args.
	if src, embeddedEngine, ok := checkEmbeddedProgram(); ok {
		if engine == "" && os.Getenv("JOKER_WASM_ENGINE") == "" {
			engine = embeddedEngine
		}
		if err := applyWasmEngine(engine); err != nil {
			fmt.Fprintln(Stderr, "Error:", err)
			corert.ExitJoker(1)
		}
		runEmbeddedSource(src)
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "compile" {
		handleCompileEngine(os.Args[2:], engine)
		return
	}
	if err := applyWasmEngine(engine); err != nil {
		fmt.Fprintln(Stderr, "Error:", err)
		corert.ExitJoker(1)
	}
	if len(os.Args) >= 2 && os.Args[1] == "doc" {
		initRuntime()
		handleDocCommand(os.Args[2:])
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "notebook" {
		initRuntime()
		handleNotebookCommand(os.Args[2:])
		return
	}

	initRuntime()
	dumpDebugState()

	if helpFlag {
		usage(Stdout)
		return
	}

	if versionFlag {
		println(corert.VERSION)
		return
	}

	validateRemainingArgs()

	if err := startProfiling(); err != nil {
		fmt.Fprintln(Stderr, err)
		corert.ExitJoker(96)
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
