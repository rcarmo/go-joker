package main

import (
	"fmt"
	"os"
	"strings"

	. "github.com/rcarmo/go-joker/core"
	_ "github.com/rcarmo/go-joker/std/base64"
	_ "github.com/rcarmo/go-joker/std/bolt"
	_ "github.com/rcarmo/go-joker/std/crypto"
	_ "github.com/rcarmo/go-joker/std/csv"
	_ "github.com/rcarmo/go-joker/std/edn"
	_ "github.com/rcarmo/go-joker/std/filepath"
	_ "github.com/rcarmo/go-joker/std/git"
	_ "github.com/rcarmo/go-joker/std/hex"
	_ "github.com/rcarmo/go-joker/std/html"
	_ "github.com/rcarmo/go-joker/std/http"
	_ "github.com/rcarmo/go-joker/std/imaging"
	_ "github.com/rcarmo/go-joker/std/io"
	_ "github.com/rcarmo/go-joker/std/jit"
	_ "github.com/rcarmo/go-joker/std/json"
	_ "github.com/rcarmo/go-joker/std/log"
	_ "github.com/rcarmo/go-joker/std/markdown"
	_ "github.com/rcarmo/go-joker/std/math"
	_ "github.com/rcarmo/go-joker/std/os"
	_ "github.com/rcarmo/go-joker/std/pdf"
	_ "github.com/rcarmo/go-joker/std/pods"
	_ "github.com/rcarmo/go-joker/std/random"
	_ "github.com/rcarmo/go-joker/std/runtime"
	_ "github.com/rcarmo/go-joker/std/strconv"
	_ "github.com/rcarmo/go-joker/std/string"
	_ "github.com/rcarmo/go-joker/std/svg"
	_ "github.com/rcarmo/go-joker/std/system"
	_ "github.com/rcarmo/go-joker/std/time"
	_ "github.com/rcarmo/go-joker/std/transit"
	_ "github.com/rcarmo/go-joker/std/url"
	_ "github.com/rcarmo/go-joker/std/uuid"
	_ "github.com/rcarmo/go-joker/std/yaml"
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
		GLOBAL_ENV.InitEnv(Stdin, Stdout, Stderr, os.Args[1:])
		ProcessCoreData()
		GLOBAL_ENV.ReferCoreToUser()
		GLOBAL_ENV.SetEnvArgs(os.Args[1:])
		reader := NewReader(strings.NewReader(src), "<standalone>")
		if err := ProcessReader(reader, "", EVAL); err != nil {
			ExitJoker(1)
		}
		return
	}

	GLOBAL_ENV.InitEnv(Stdin, Stdout, Stderr, os.Args[1:])

	parseArgs(os.Args) // Do this early enough so --verbose can show joker.core being processed.

	saveForRepl = saveForRepl && (exitToRepl || errorToRepl) // don't bother saving stuff if no repl

	ProcessCoreData()

	GLOBAL_ENV.ReferCoreToUser()
	GLOBAL_ENV.SetEnvArgs(remainingArgs)
	GLOBAL_ENV.SetClassPath(classPath)

	if debugOut != nil {
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

	if helpFlag {
		usage(Stdout)
		return
	}

	if versionFlag {
		println(VERSION)
		return
	}

	if len(remainingArgs) > 0 {
		if lintFlag {
			fmt.Fprintf(Stderr, "Error: Cannot provide arguments to code while linting it.\n")
			ExitJoker(4)
		}
		if phase != EVAL && phase != PRINT_IF_NOT_NIL {
			fmt.Fprintf(Stderr, "Error: Cannot provide arguments to code without evaluating it.\n")
			ExitJoker(5)
		}
	}

	if err := startProfiling(); err != nil {
		fmt.Fprintln(Stderr, err)
		ExitJoker(96)
	}

	if eval != "" {
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
		} else {
			if !exitToRepl {
				return
			}
		}
	}

	if lintFlag {
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
		return
	}

	if workingDir != "" {
		fmt.Fprintf(Stderr, "Error: Cannot specify --working-dir option when not linting.\n")
		ExitJoker(11)
	}

	if filename != "" {
		if err := processFile(filename, phase); err != nil {
			if !errorToRepl {
				ExitJoker(1)
			}
		} else {
			if !exitToRepl {
				return
			}
		}
	}

	if replSocket != "" {
		srepl(replSocket, phase)
		return
	}

	repl(phase)
	return
}
