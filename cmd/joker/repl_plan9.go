package main

import (
	"bufio"
	"fmt"
	corereader "github.com/rcarmo/go-joker/core/reader"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	. "github.com/rcarmo/go-joker/core"
)

func repl(phase corereader.Phase) {
	ProcessReplData()
	GLOBAL_ENV.FindNamespace(coretypes.MakeSymbol(STRINGS.Intern, "user")).ReferAll(GLOBAL_ENV.FindNamespace(coretypes.MakeSymbol(STRINGS.Intern, "joker.repl")))
	fmt.Printf("Welcome to joker %s. Use '(exit)', %s to exit.\n", VERSION, EXITERS)
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
