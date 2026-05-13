package main

import (
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"

	. "github.com/rcarmo/go-joker/core"
)

var (
	debugOut                 io.Writer
	helpFlag                 bool
	versionFlag              bool
	phase                    Phase = EVAL // --read, --parse, --evaluate
	workingDir               string
	lintFlag                 bool
	reportGloballyUnusedFlag bool
	dialect                  Dialect = UNKNOWN
	eval                     string
	replFlag                 bool
	replSocket               string
	classPath                string
	filename                 string
	remainingArgs            []string
	profilerType             string = "runtime/pprof"
	cpuProfileName           string
	cpuProfileRate           int
	cpuProfileRateFlag       bool
	memProfileName           string
	noReadline               bool
	noReplHistory            bool
	exitToRepl               bool
	errorToRepl              bool
	writeFlag                bool
)

func isNumber(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

func notOption(arg string) bool {
	return arg == "-" || !strings.HasPrefix(arg, "-") || isNumber(arg[1:])
}

func parseArgs(args []string) {
	if len(args) > 1 {
		// peek to see if the first arg is "--debug*"
		switch args[1] {
		case "--debug", "--debug=stderr":
			debugOut = Stderr
		case "--debug=stdout":
			debugOut = Stdout
		}
	}

	length := len(args)
	stop := false
	missing := false
	noFileFlag := false
	if v, ok := os.LookupEnv("JOKER_CLASSPATH"); ok {
		classPath = v
	} else {
		classPath = ""
	}
	var i int
	for i = 1; i < length; i++ { // shift
		if debugOut != nil {
			fmt.Fprintf(debugOut, "arg[%d]=%s\n", i, args[i])
		}
		switch args[i] {
		case "-": // denotes stdin
			stop = true
		case "--": // formally ends options processing
			stop = true
			noFileFlag = true
			i += 1 // do not include "--" in *command-line-args*
		case "--debug":
			debugOut = Stderr
		case "--debug=stderr":
			debugOut = Stderr
		case "--debug=stdout":
			debugOut = Stdout
		case "--verbose":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				verbosity, err := strconv.ParseInt(args[i], 10, 64)
				if err != nil {
					fmt.Fprintln(Stderr, "Error: ", err)
					return
				}
				if verbosity <= 0 {
					VerbosityLevel = 0
				} else {
					VerbosityLevel = int(verbosity)
				}
			} else {
				VerbosityLevel++
			}
		case "--help", "-h":
			helpFlag = true
			return // don't bother parsing anything else
		case "--version", "-v":
			versionFlag = true
		case "--format":
			phase = FORMAT
		case "--write":
			writeFlag = true
		case "--read":
			phase = READ
		case "--parse":
			phase = PARSE
		case "--evaluate":
			phase = EVAL
		case "--working-dir":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				workingDir = args[i]
			} else {
				missing = true
			}
		case "--report-globally-unused":
			reportGloballyUnusedFlag = true
		case "--lint":
			lintFlag = true
		case "--lintclj":
			lintFlag = true
			dialect = CLJ
		case "--lintcljs":
			lintFlag = true
			dialect = CLJS
		case "--lintjoker":
			lintFlag = true
			dialect = JOKER
		case "--lintedn":
			lintFlag = true
			dialect = EDN
		case "--dialect":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				dialect = dialectFromArg(args[i])
			} else {
				missing = true
			}
		case "--hashmap-threshold":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				thresh, err := strconv.ParseInt(args[i], 10, 64)
				if err != nil {
					fmt.Fprintln(Stderr, "Error: ", err)
					return
				}
				if thresh < 0 {
					HASHMAP_THRESHOLD = math.MaxInt64
				} else {
					HASHMAP_THRESHOLD = thresh
				}
			} else {
				missing = true
			}
		case "-e", "--eval":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				eval = args[i]
				phase = PRINT_IF_NOT_NIL
			} else {
				missing = true
			}
		case "--repl":
			replFlag = true
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				replSocket = args[i]
			}
		case "-c", "--classpath":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				classPath = args[i]
			} else {
				missing = true
			}
		case "--no-readline":
			noReadline = true
		case "--no-repl-history":
			noReplHistory = true
		case "--exit-to-repl":
			exitToRepl = true
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				replSocket = args[i]
			}
		case "--error-to-repl":
			errorToRepl = true
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				replSocket = args[i]
			}
		case "--file":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				filename = args[i]
			}
		case "--profiler":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				profilerType = args[i]
			} else {
				missing = true
			}
		case "--cpuprofile":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				cpuProfileName = args[i]
			} else {
				missing = true
			}
		case "--cpuprofile-rate":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				rate, err := strconv.Atoi(args[i])
				if err != nil {
					fmt.Fprintln(Stderr, "Error: ", err)
					return
				}
				if rate > 0 {
					cpuProfileRate = rate
					cpuProfileRateFlag = true
				}
			} else {
				missing = true
			}
		case "--memprofile":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				memProfileName = args[i]
			} else {
				missing = true
			}
		case "--memprofile-rate":
			if i < length-1 && notOption(args[i+1]) {
				i += 1 // shift
				rate, err := strconv.Atoi(args[i])
				if err != nil {
					fmt.Fprintln(Stderr, "Error: ", err)
					return
				}
				if rate > 0 {
					runtime.MemProfileRate = rate
				}
			} else {
				missing = true
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(Stderr, "Error: Unrecognized option '%s'\n", args[i])
				ExitJoker(2)
			}
			stop = true
		}
		if stop || missing {
			break
		}
	}
	if missing {
		fmt.Fprintf(Stderr, "Error: Missing argument for '%s' option\n", args[i])
		ExitJoker(3)
	}
	if i < length && !noFileFlag && filename == "" {
		if debugOut != nil {
			fmt.Fprintf(debugOut, "filename=%s\n", args[i])
		}
		filename = args[i]
		i += 1 // shift
	}
	if i < length {
		if debugOut != nil {
			fmt.Fprintf(debugOut, "remaining=%v\n", args[i:])
		}
		remainingArgs = args[i:]
	}
}
