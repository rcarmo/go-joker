package main

import (
	"bufio"
	"fmt"
	corereader "github.com/rcarmo/go-joker/v42/core/reader"
	corert "github.com/rcarmo/go-joker/v42/core/runtime"
	"io"

	. "github.com/rcarmo/go-joker/v42/core"
)

func repl(phase corereader.Phase) {
	ProcessReplData()
	referReplNamespace()
	fmt.Printf("Welcome to joker %s. Use '(exit)', %s to exit.\n", corert.VERSION, EXITERS)
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	replContext := NewReplContext(parseContext.GlobalEnv)

	var runeReader io.RuneReader
	runeReader = bufio.NewReader(Stdin)
	reader := NewReader(runeReader, "<repl>")

	for {
		print(GLOBAL_ENV.CurrentNamespace().Name.ToString(false) + "=> ")
		if processReplCommand(reader, phase, parseContext, replContext) {
			return
		}
	}
}
