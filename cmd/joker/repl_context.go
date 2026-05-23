package main

import (
	"fmt"
	corereader "github.com/rcarmo/go-joker/core/reader"
	corert "github.com/rcarmo/go-joker/core/runtime"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

	. "github.com/rcarmo/go-joker/core"
)

var dataRead = []rune{}
var saveForRepl = true

func findReplNamespace() *Namespace {
	replSym := coretypes.MakeSymbol(STRINGS.Intern, "joker.repl")
	replNs := GLOBAL_ENV.FindNamespace(replSym)
	if replNs != nil {
		return replNs
	}

	// Some generated bootstrap payloads still register this namespace as
	// joker.Repl even though the source namespace is joker.repl.
	replNs = GLOBAL_ENV.FindNamespace(coretypes.MakeSymbol(STRINGS.Intern, "joker.Repl"))
	if replNs != nil {
		corert.NamespaceMu.Lock()
		GLOBAL_ENV.Namespaces[replSym.NameKey()] = replNs
		corert.NamespaceMu.Unlock()
		return replNs
	}

	panic(coretypes.RuntimeError("missing generated REPL namespace joker.repl"))
}

func referReplNamespace() {
	userNs := GLOBAL_ENV.FindNamespace(coretypes.MakeSymbol(STRINGS.Intern, "user"))
	if userNs == nil {
		panic(coretypes.RuntimeError("missing user namespace"))
	}
	userNs.ReferAll(findReplNamespace())
}

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

type ReplContext struct {
	first  *Var
	second *Var
	third  *Var
	exc    *Var
}

func NewReplContext(env *Env) *ReplContext {
	first, _ := env.Resolve(coretypes.MakeSymbol(STRINGS.Intern, "joker.core/*1"))
	second, _ := env.Resolve(coretypes.MakeSymbol(STRINGS.Intern, "joker.core/*2"))
	third, _ := env.Resolve(coretypes.MakeSymbol(STRINGS.Intern, "joker.core/*3"))
	exc, _ := env.Resolve(coretypes.MakeSymbol(STRINGS.Intern, "joker.core/*e"))
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

func (ctx *ReplContext) PushValue(obj coretypes.Object) {
	ctx.third.Value = ctx.second.Value
	ctx.second.Value = ctx.first.Value
	ctx.first.Value = obj
}

func (ctx *ReplContext) PushException(exc coretypes.Object) {
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

func processReplCommand(reader *Reader, phase corereader.Phase, parseContext *ParseContext, replContext *ReplContext) (exit bool) {
	defer func() {
		if r := recover(); r != nil {
			switch r := r.(type) {
			case *ParseError:
				replContext.PushException(r)
				fmt.Fprintln(Stderr, r)
			case *corert.EvalError:
				replContext.PushException(r)
				fmt.Fprintln(Stderr, r)
			case coretypes.Error:
				replContext.PushException(r)
				fmt.Fprintln(Stderr, r)
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

	if phase == corereader.ReadPhase {
		fmt.Println(obj.ToString(true))
		return false
	}

	expr := Parse(obj, parseContext)
	if phase == corereader.ParsePhase {
		fmt.Println(expr)
		return false
	}

	res := Eval(expr, nil)
	replContext.PushValue(res)
	PrintObject(res, Stdout)
	fmt.Fprintln(Stdout, "")
	return false
}
