package main

import (
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func resetArgsForTest(t *testing.T) {
	t.Helper()

	oldVerbosity := VerbosityLevel
	oldHashmapThreshold := HASHMAP_THRESHOLD
	t.Cleanup(func() {
		VerbosityLevel = oldVerbosity
		HASHMAP_THRESHOLD = oldHashmapThreshold
		resetParsedArgsForTest()
	})

	resetParsedArgsForTest()
	VerbosityLevel = 0
}

func resetParsedArgsForTest() {
	debugOut = nil
	helpFlag = false
	versionFlag = false
	phase = EVAL
	workingDir = ""
	lintFlag = false
	reportGloballyUnusedFlag = false
	dialect = UNKNOWN
	eval = ""
	replFlag = false
	replSocket = ""
	classPath = ""
	filename = ""
	remainingArgs = nil
	profilerType = "runtime/pprof"
	cpuProfileName = ""
	cpuProfileRate = 0
	cpuProfileRateFlag = false
	memProfileName = ""
	noReadline = false
	noReplHistory = false
	exitToRepl = false
	errorToRepl = false
	writeFlag = false
}

func TestParseArgsEvalStopsBeforeFileAndKeepsRemainingArgs(t *testing.T) {
	resetArgsForTest(t)
	t.Setenv("JOKER_CLASSPATH", "envcp")

	parseArgs([]string{"joker", "--eval", "(+ 1 2)", "script.clj", "a", "b"})

	if phase != PRINT_IF_NOT_NIL {
		t.Fatalf("phase = %v, want PRINT_IF_NOT_NIL", phase)
	}
	if eval != "(+ 1 2)" {
		t.Fatalf("eval = %q", eval)
	}
	if filename != "script.clj" {
		t.Fatalf("filename = %q", filename)
	}
	if classPath != "envcp" {
		t.Fatalf("classPath = %q", classPath)
	}
	if got := len(remainingArgs); got != 2 || remainingArgs[0] != "a" || remainingArgs[1] != "b" {
		t.Fatalf("remainingArgs = %#v", remainingArgs)
	}
}

func TestParseArgsDoubleDashSkipsFilenameParsing(t *testing.T) {
	resetArgsForTest(t)

	parseArgs([]string{"joker", "--", "--not-an-option", "file.clj"})

	if filename != "" {
		t.Fatalf("filename = %q, want empty", filename)
	}
	if got := len(remainingArgs); got != 2 || remainingArgs[0] != "--not-an-option" || remainingArgs[1] != "file.clj" {
		t.Fatalf("remainingArgs = %#v", remainingArgs)
	}
}

func TestParseArgsLintDialectAndProfilerFlags(t *testing.T) {
	resetArgsForTest(t)

	parseArgs([]string{"joker", "--lintcljs", "--working-dir", "src", "--classpath", "lib:std", "--cpuprofile-rate", "250", "--memprofile", "mem.out", "target.cljs"})

	if !lintFlag {
		t.Fatal("lintFlag = false, want true")
	}
	if dialect != CLJS {
		t.Fatalf("dialect = %v, want CLJS", dialect)
	}
	if workingDir != "src" {
		t.Fatalf("workingDir = %q", workingDir)
	}
	if classPath != "lib:std" {
		t.Fatalf("classPath = %q", classPath)
	}
	if cpuProfileRate != 250 || !cpuProfileRateFlag {
		t.Fatalf("cpu profile rate = %d flag=%v", cpuProfileRate, cpuProfileRateFlag)
	}
	if memProfileName != "mem.out" {
		t.Fatalf("memProfileName = %q", memProfileName)
	}
	if filename != "target.cljs" {
		t.Fatalf("filename = %q", filename)
	}
}
