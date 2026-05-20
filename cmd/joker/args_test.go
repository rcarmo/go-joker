package main

import (
	"bytes"
	corereader "github.com/rcarmo/go-joker/core/reader"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
	"runtime"
	"strings"
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func resetArgsForTest(t *testing.T) {
	t.Helper()

	oldVerbosity := VerbosityLevel
	oldHashmapThreshold := corecollections.HASHMAP_THRESHOLD
	oldStderr := Stderr
	oldMemProfileRate := runtime.MemProfileRate
	t.Cleanup(func() {
		VerbosityLevel = oldVerbosity
		corecollections.HASHMAP_THRESHOLD = oldHashmapThreshold
		Stderr = oldStderr
		runtime.MemProfileRate = oldMemProfileRate
		resetParsedArgsForTest()
	})

	resetParsedArgsForTest()
	VerbosityLevel = 0
}

func resetParsedArgsForTest() {
	debugOut = nil
	helpFlag = false
	versionFlag = false
	phase = corereader.EvalPhase
	workingDir = ""
	lintFlag = false
	reportGloballyUnusedFlag = false
	dialect = corereader.UnknownDialect
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

	if phase != corereader.PrintIfNotNilPhase {
		t.Fatalf("phase = %v, want corereader.PrintIfNotNilPhase", phase)
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

func TestParseArgsRejectsNonPositiveProfileRates(t *testing.T) {
	resetArgsForTest(t)
	var stderr bytes.Buffer
	Stderr = &stderr

	parseArgs([]string{"joker", "--cpuprofile-rate", "0", "target.clj"})

	if cpuProfileRateFlag || cpuProfileRate != 0 {
		t.Fatalf("cpu profile rate accepted non-positive value: rate=%d flag=%v", cpuProfileRate, cpuProfileRateFlag)
	}
	if !strings.Contains(stderr.String(), "--cpuprofile-rate must be positive") {
		t.Fatalf("stderr = %q", stderr.String())
	}

	stderr.Reset()
	parseArgs([]string{"joker", "--memprofile-rate", "-1", "target.clj"})
	if runtime.MemProfileRate == -1 {
		t.Fatal("mem profile rate accepted negative value")
	}
	if !strings.Contains(stderr.String(), "--memprofile-rate must be positive") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestParseArgsLintDialectAndProfilerFlags(t *testing.T) {
	resetArgsForTest(t)

	parseArgs([]string{"joker", "--lintcljs", "--working-dir", "src", "--classpath", "lib:std", "--cpuprofile-rate", "250", "--memprofile", "mem.out", "target.cljs"})

	if !lintFlag {
		t.Fatal("lintFlag = false, want true")
	}
	if dialect != corereader.CLJSDialect {
		t.Fatalf("dialect = %v, want corereader.CLJSDialect", dialect)
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
