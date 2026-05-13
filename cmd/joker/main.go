package main

import (
	"fmt"
	"os"

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
