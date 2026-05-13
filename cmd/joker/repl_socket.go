package main

import (
	"bufio"
	"fmt"
	"io"
	"net"

	. "github.com/rcarmo/go-joker/core"
)

var dataRead = []rune{}
var saveForRepl = true

type replayable struct {
	reader *Reader
}

func (r *replayable) ReadRune() (ch rune, size int, err error) {
	ch = r.reader.Get()
	if ch == EOF {
		err = io.EOF
		size = 0
	} else {
		dataRead = append(dataRead, ch)
		size = 1
	}
	return
}

type (
	ReplContext struct {
		first  *Var
		second *Var
		third  *Var
		exc    *Var
	}
)

func NewReplContext(env *Env) *ReplContext {
	first, _ := env.Resolve(MakeSymbol("joker.core/*1"))
	second, _ := env.Resolve(MakeSymbol("joker.core/*2"))
	third, _ := env.Resolve(MakeSymbol("joker.core/*3"))
	exc, _ := env.Resolve(MakeSymbol("joker.core/*e"))
	first.Value = NIL
	second.Value = NIL
	third.Value = NIL
	exc.Value = NIL
	return &ReplContext{
		first:  first,
		second: second,
		third:  third,
		exc:    exc,
	}
}

func (ctx *ReplContext) PushValue(obj Object) {
	ctx.third.Value = ctx.second.Value
	ctx.second.Value = ctx.first.Value
	ctx.first.Value = obj
}

func (ctx *ReplContext) PushException(exc Object) {
	ctx.exc.Value = exc
}

func skipRestOfLine(reader *Reader) {
	for {
		switch reader.Get() {
		case EOF, '\n':
			return
		}
	}
}

func processReplCommand(reader *Reader, phase Phase, parseContext *ParseContext, replContext *ReplContext) (exit bool) {

	defer func() {
		if r := recover(); r != nil {
			switch r := r.(type) {
			case *ParseError:
				replContext.PushException(r)
				fmt.Fprintln(Stderr, r)
			case *EvalError:
				replContext.PushException(r)
				fmt.Fprintln(Stderr, r)
			case Error:
				replContext.PushException(r)
				fmt.Fprintln(Stderr, r)
				// case *runtime.TypeAssertionError:
				// 	fmt.Fprintln(Stderr, r)
			default:
				panic(r)
			}
		}
	}()

	obj, err := TryRead(reader)
	if err == io.EOF {
		return true
	}
	if err != nil {
		fmt.Fprintln(Stderr, err)
		skipRestOfLine(reader)
		return
	}

	if phase == READ {
		fmt.Println(obj.ToString(true))
		return false
	}

	expr := Parse(obj, parseContext)
	if phase == PARSE {
		fmt.Println(expr)
		return false
	}

	res := Eval(expr, nil)
	replContext.PushValue(res)
	PrintObject(res, Stdout)
	fmt.Fprintln(Stdout, "")
	return false
}

func srepl(port string, phase Phase) {
	ProcessReplData()
	GLOBAL_ENV.FindNamespace(MakeSymbol("user")).ReferAll(GLOBAL_ENV.FindNamespace(MakeSymbol("joker.repl")))
	GLOBAL_ENV.CoreNamespace.Resolve("*repl*").Value = Boolean{B: true}
	l, err := net.Listen("tcp", replSocket)
	if err != nil {
		fmt.Fprintf(Stderr, "Cannot start srepl listening on %s: %s\n",
			replSocket, err.Error())
		ExitJoker(12)
	}
	defer l.Close()

	fmt.Printf("Joker repl listening at %s...\n", l.Addr())
	conn, err := l.Accept() // Wait for a single connection
	if err != nil {
		fmt.Fprintf(Stderr, "Cannot start repl accepting on %s: %s\n",
			l.Addr(), err.Error())
		ExitJoker(13)
	}

	oldStdIn := Stdin
	oldStdOut := Stdout
	oldStdErr := Stderr
	oldStdinValue, oldStdoutValue, oldStderrValue := GLOBAL_ENV.StdIO()
	Stdin = conn
	Stdout = conn
	Stderr = conn
	newIn := MakeBufferedReader(conn)
	newOut := MakeIOWriter(conn)
	GLOBAL_ENV.SetStdIO(newIn, newOut, newOut)
	defer func() {
		conn.Close()
		Stdin = oldStdIn
		Stdout = oldStdOut
		Stderr = oldStdErr
		GLOBAL_ENV.SetStdIO(oldStdinValue, oldStdoutValue, oldStderrValue)
	}()

	fmt.Printf("Joker repl accepting client at %s...\n", conn.RemoteAddr())

	runeReader := bufio.NewReader(conn)

	/* The rest of this code comes from repl(), below: */

	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	replContext := NewReplContext(parseContext.GlobalEnv)

	reader := NewReader(runeReader, "<srepl>")

	fmt.Fprintf(Stdout, "Welcome to joker %s, client at %s. Use '(exit)', or close the connection, to exit.\n",
		VERSION, conn.RemoteAddr())

	for {
		fmt.Fprint(Stdout, GLOBAL_ENV.CurrentNamespace().Name.ToString(false)+"=> ")
		if processReplCommand(reader, phase, parseContext, replContext) {
			return
		}
	}
}
