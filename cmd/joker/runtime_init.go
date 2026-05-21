package main

import (
	"fmt"
	corereader "github.com/rcarmo/go-joker/core/reader"
	corert "github.com/rcarmo/go-joker/core/runtime"
	"os"
	"strings"

	. "github.com/rcarmo/go-joker/core"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

func runEmbeddedSource(src string) {
	GLOBAL_ENV.InitEnv(Stdin, Stdout, Stderr, os.Args[1:])
	ProcessCoreData()
	GLOBAL_ENV.ReferCoreToUser()
	GLOBAL_ENV.SetEnvArgs(os.Args[1:])
	reader := NewReader(strings.NewReader(src), "<standalone>")
	if err := ProcessReader(reader, "", corereader.EvalPhase); err != nil {
		corert.ExitJoker(1)
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
	fmt.Fprintf(debugOut, "HASHMAP_THRESHOLD=%v\n", corecollections.HASHMAP_THRESHOLD)
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
		corert.ExitJoker(4)
	}
	if phase != corereader.EvalPhase && phase != corereader.PrintIfNotNilPhase {
		fmt.Fprintf(Stderr, "Error: Cannot provide arguments to code without evaluating it.\n")
		corert.ExitJoker(5)
	}
}
