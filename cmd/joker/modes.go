package main

import (
	"fmt"
	"os"
	"strings"

	. "github.com/rcarmo/go-joker/core"
)

func runEmbeddedSource(src string) {
	GLOBAL_ENV.InitEnv(Stdin, Stdout, Stderr, os.Args[1:])
	ProcessCoreData()
	GLOBAL_ENV.ReferCoreToUser()
	GLOBAL_ENV.SetEnvArgs(os.Args[1:])
	reader := NewReader(strings.NewReader(src), "<standalone>")
	if err := ProcessReader(reader, "", EVAL); err != nil {
		ExitJoker(1)
	}
}

func initRuntime() {
	GLOBAL_ENV.InitEnv(Stdin, Stdout, Stderr, os.Args[1:])
	parseArgs(os.Args)
	saveForRepl = saveForRepl && (exitToRepl || errorToRepl)
	ProcessCoreData()
	GLOBAL_ENV.ReferCoreToUser()
	GLOBAL_ENV.SetEnvArgs(remainingArgs)
	GLOBAL_ENV.SetClassPath(classPath)
}

func dumpDebugState() {
	if debugOut == nil {
		return
	}
	fmt.Fprintf(debugOut, "debugOut=%v\n", debugOut)
	fmt.Fprintf(debugOut, "helpFlag=%v\n", helpFlag)
	fmt.Fprintf(debugOut, "versionFlag=%v\n", versionFlag)
	fmt.Fprintf(debugOut, "phase=%v\n", phase)
	fmt.Fprintf(debugOut, "lintFlag=%v\n", lintFlag)
	fmt.Fprintf(debugOut, "reportGloballyUnusedFlag=%v\n", reportGloballyUnusedFlag)
	fmt.Fprintf(debugOut, "dialect=%v\n", dialect)
	fmt.Fprintf(debugOut, "workingDir=%v\n", workingDir)
	fmt.Fprintf(debugOut, "HASHMAP_THRESHOLD=%v\n", HASHMAP_THRESHOLD)
	fmt.Fprintf(debugOut, "eval=%v\n", eval)
	fmt.Fprintf(debugOut, "replFlag=%v\n", replFlag)
	fmt.Fprintf(debugOut, "replSocket=%v\n", replSocket)
	fmt.Fprintf(debugOut, "classPath=%v\n", classPath)
	fmt.Fprintf(debugOut, "noReadline=%v\n", noReadline)
	fmt.Fprintf(debugOut, "noReplHistory=%v\n", noReplHistory)
	fmt.Fprintf(debugOut, "filename=%v\n", filename)
	fmt.Fprintf(debugOut, "remainingArgs=%v\n", remainingArgs)
	fmt.Fprintf(debugOut, "exitToRepl=%v\n", exitToRepl)
	fmt.Fprintf(debugOut, "errorToRepl=%v\n", errorToRepl)
	fmt.Fprintf(debugOut, "saveForRepl=%v\n", saveForRepl)
}

func validateRemainingArgs() {
	if len(remainingArgs) == 0 {
		return
	}
	if lintFlag {
		fmt.Fprintf(Stderr, "Error: Cannot provide arguments to code while linting it.\n")
		ExitJoker(4)
	}
	if phase != EVAL && phase != PRINT_IF_NOT_NIL {
		fmt.Fprintf(Stderr, "Error: Cannot provide arguments to code without evaluating it.\n")
		ExitJoker(5)
	}
}

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
