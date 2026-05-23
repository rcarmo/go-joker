package main

import (
	"bufio"
	"fmt"
	corereader "github.com/rcarmo/go-joker/core/reader"
	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"net"

	. "github.com/rcarmo/go-joker/core"
)

func srepl(port string, phase corereader.Phase) {
	ProcessReplData()
	referReplNamespace()
	GLOBAL_ENV.CoreNamespace.Resolve("*repl*").Value = coretypes.Boolean{B: true}
	l, err := net.Listen("tcp", replSocket)
	if err != nil {
		fmt.Fprintf(Stderr, "Cannot start srepl listening on %s: %s\n",
			replSocket, err.Error())
		corert.ExitJoker(12)
	}
	defer func() {
		if err := l.Close(); err != nil {
			fmt.Fprintf(Stderr, "WARNING: could not close srepl listener: %v\n", err)
		}
	}()

	fmt.Printf("Joker repl listening at %s...\n", l.Addr())
	conn, err := l.Accept() // Wait for a single connection
	if err != nil {
		fmt.Fprintf(Stderr, "Cannot start repl accepting on %s: %s\n",
			l.Addr(), err.Error())
		corert.ExitJoker(13)
	}

	oldStdIn := Stdin
	oldStdOut := Stdout
	oldStdErr := Stderr
	oldStdinValue, oldStdoutValue, oldStderrValue := GLOBAL_ENV.StdIO()
	Stdin = conn
	Stdout = conn
	Stderr = conn
	newIn := corert.MakeBufferedReader(conn)
	newOut := corert.MakeIOWriter(conn)
	GLOBAL_ENV.SetStdIO(newIn, newOut, newOut)
	defer func() {
		if err := conn.Close(); err != nil {
			fmt.Fprintf(Stderr, "WARNING: could not close srepl connection: %v\n", err)
		}
		Stdin = oldStdIn
		Stdout = oldStdOut
		Stderr = oldStdErr
		GLOBAL_ENV.SetStdIO(oldStdinValue, oldStdoutValue, oldStderrValue)
	}()

	fmt.Printf("Joker repl accepting client at %s...\n", conn.RemoteAddr())

	runeReader := bufio.NewReader(conn)
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	replContext := NewReplContext(parseContext.GlobalEnv)
	reader := NewReader(runeReader, "<srepl>")

	fmt.Fprintf(Stdout, "Welcome to joker %s, client at %s. Use '(exit)', or close the connection, to exit.\n",
		corert.VERSION, conn.RemoteAddr())

	for {
		fmt.Fprint(Stdout, GLOBAL_ENV.CurrentNamespace().Name.ToString(false)+"=> ")
		if processReplCommand(reader, phase, parseContext, replContext) {
			return
		}
	}
}
