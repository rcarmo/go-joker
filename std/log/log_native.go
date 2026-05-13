package log

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	. "github.com/rcarmo/go-joker/core"
)

var (
	logLevel int = 2 // 0=debug, 1=info, 2=warn, 3=error
	logMu    sync.Mutex
)

var levelNames = []string{"DEBUG", "INFO", "WARN", "ERROR"}

func parseLogLevel(obj Object, context string) int {
	switch s := obj.ToString(false); s {
	case ":debug", "debug", `"debug"`:
		return 0
	case ":info", "info", `"info"`:
		return 1
	case ":warn", "warn", `"warn"`:
		return 2
	case ":error", "error", `"error"`:
		return 3
	default:
		panic(RT.NewError(context + " expects :debug, :info, :warn, or :error"))
	}
}

func logMsg(level int, args []Object) {
	if level < logLevel {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	ts := time.Now().Format("2006-01-02T15:04:05.000")
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = a.ToString(false)
	}
	fmt.Fprintf(os.Stderr, "%s [%s] %s\n", ts, levelNames[level], strings.Join(parts, " "))
}

var logNamespace = GLOBAL_ENV.EnsureSymbolIsLib(MakeSymbol("joker.log"))

func init() {
	logNamespace.Lazy = initLogNamespace
}

func initLogNamespace() {
	logNamespace.ResetMeta(MakeMeta(nil, `Simple leveled logging to stderr.`, "1.0"))

	// debug
	logNamespace.InternVar("debug", Proc{Fn: func(args []Object) Object {
		logMsg(0, args)
		return NIL
	}, Name: "log-debug", Package: "std/log"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("&"), MakeSymbol("args"))),
			`Logs a DEBUG message to stderr.`, "1.0"))

	// info
	logNamespace.InternVar("info", Proc{Fn: func(args []Object) Object {
		logMsg(1, args)
		return NIL
	}, Name: "log-info", Package: "std/log"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("&"), MakeSymbol("args"))),
			`Logs an INFO message to stderr.`, "1.0"))

	// warn
	logNamespace.InternVar("warn", Proc{Fn: func(args []Object) Object {
		logMsg(2, args)
		return NIL
	}, Name: "log-warn", Package: "std/log"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("&"), MakeSymbol("args"))),
			`Logs a WARN message to stderr.`, "1.0"))

	// error
	logNamespace.InternVar("error", Proc{Fn: func(args []Object) Object {
		logMsg(3, args)
		return NIL
	}, Name: "log-error", Package: "std/log"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("&"), MakeSymbol("args"))),
			`Logs an ERROR message to stderr.`, "1.0"))

	// set-level! — set the minimum log level
	logNamespace.InternVar("set-level!", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		lvl := parseLogLevel(args[0], "set-level!")
		logMu.Lock()
		logLevel = lvl
		logMu.Unlock()
		return NIL
	}, Name: "log-set-level", Package: "std/log"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("level"))),
			`Sets the minimum log level (:debug, :info, :warn, :error). Default is :warn.`, "1.0"))

	// get-level — returns the current log level as a keyword
	logNamespace.InternVar("get-level", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 0, 0)
		logMu.Lock()
		l := logLevel
		logMu.Unlock()
		return MakeKeyword(strings.ToLower(levelNames[l]))
	}, Name: "log-get-level", Package: "std/log"},
		MakeMeta(NewListFrom(NewVectorFrom()),
			`Returns the current log level as a keyword.`, "1.0"))

	// logf — formatted log message
	logNamespace.InternVar("logf", Proc{Fn: func(args []Object) Object {
		CheckArity(args, 2, -1)
		lvl := parseLogLevel(args[0], "logf level")
		logMsg(lvl, args[1:])
		return NIL
	}, Name: "log-logf", Package: "std/log"},
		MakeMeta(NewListFrom(NewVectorFrom(MakeSymbol("level"), MakeSymbol("&"), MakeSymbol("args"))),
			`Logs a message at the specified level.`, "1.0"))
}
