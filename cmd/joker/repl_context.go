package main

import (
	"fmt"
	corereader "github.com/rcarmo/go-joker/core/reader"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"

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

type ReplContext struct {
	first  *Var
	second *Var
	third  *Var
	exc    *Var
}

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
			case *EvalError:
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
