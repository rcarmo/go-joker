package term

import (
	"fmt"
	"os"
	"time"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"

	"golang.org/x/term"
)

// Terminal state saved when entering raw mode.
var oldState *term.State

var procRawMode ProcFn = func(args []coretypes.Object) coretypes.Object {
	state, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		panic(RT.NewError("term/raw-mode!: " + err.Error()))
	}
	oldState = state
	return coretypes.Boolean{B: true}
}

var procRestore ProcFn = func(args []coretypes.Object) coretypes.Object {
	if oldState != nil {
		term.Restore(int(os.Stdin.Fd()), oldState)
		oldState = nil
	}
	return coretypes.Boolean{B: true}
}

var procClear ProcFn = func(args []coretypes.Object) coretypes.Object {
	fmt.Print("\033[2J\033[H")
	return NIL
}

var procAltScreen ProcFn = func(args []coretypes.Object) coretypes.Object {
	fmt.Print("\033[?1049h")
	return NIL
}

var procMainScreen ProcFn = func(args []coretypes.Object) coretypes.Object {
	fmt.Print("\033[?1049l")
	return NIL
}

var procHideCursor ProcFn = func(args []coretypes.Object) coretypes.Object {
	fmt.Print("\033[?25l")
	return NIL
}

var procShowCursor ProcFn = func(args []coretypes.Object) coretypes.Object {
	fmt.Print("\033[?25h")
	return NIL
}

var procMoveTo ProcFn = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureArgIsInt(args, 0).I
	y := coretypes.EnsureArgIsInt(args, 1).I
	// ANSI uses 1-based row;col
	fmt.Printf("\033[%d;%dH", y+1, x+1)
	return NIL
}

var procSize ProcFn = func(args []coretypes.Object) coretypes.Object {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		panic(RT.NewError("term/size: " + err.Error()))
	}
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "cols"), coretypes.MakeInt(w))
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "rows"), coretypes.MakeInt(h))
	return m
}

var procPrint ProcFn = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.ExtractString(args, 0)
	fmt.Print(s)
	return NIL
}

var procFlush ProcFn = func(args []coretypes.Object) coretypes.Object {
	os.Stdout.Sync()
	return NIL
}

var procFg ProcFn = func(args []coretypes.Object) coretypes.Object {
	r, g, b := extractRGB(args, 0)
	return coretypes.String{S: fmt.Sprintf("\033[38;2;%d;%d;%dm", r, g, b)}
}

var procBg ProcFn = func(args []coretypes.Object) coretypes.Object {
	r, g, b := extractRGB(args, 0)
	return coretypes.String{S: fmt.Sprintf("\033[48;2;%d;%d;%dm", r, g, b)}
}

var procResetStyle ProcFn = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: "\033[0m"}
}

var procReadKey ProcFn = func(args []coretypes.Object) coretypes.Object {
	timeoutMs := 50
	if len(args) > 0 {
		timeoutMs = coretypes.EnsureArgIsInt(args, 0).I
	}

	buf := make([]byte, 1)
	os.Stdin.SetReadDeadline(time.Now().Add(time.Duration(timeoutMs) * time.Millisecond))
	n, err := os.Stdin.Read(buf)
	os.Stdin.SetReadDeadline(time.Time{})

	if n == 0 || err != nil {
		return coretypes.MakeKeyword(STRINGS.Intern, "none")
	}

	ch := buf[0]

	// ESC sequence
	if ch == 27 {
		buf2 := make([]byte, 2)
		os.Stdin.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
		n2, _ := os.Stdin.Read(buf2)
		os.Stdin.SetReadDeadline(time.Time{})

		if n2 >= 2 && (buf2[0] == '[' || buf2[0] == 'O') {
			switch buf2[1] {
			case 'A':
				return coretypes.MakeKeyword(STRINGS.Intern, "up")
			case 'B':
				return coretypes.MakeKeyword(STRINGS.Intern, "down")
			case 'C':
				return coretypes.MakeKeyword(STRINGS.Intern, "right")
			case 'D':
				return coretypes.MakeKeyword(STRINGS.Intern, "left")
			}
		}
		return coretypes.MakeKeyword(STRINGS.Intern, "esc")
	}

	switch ch {
	case ' ':
		return coretypes.MakeKeyword(STRINGS.Intern, "space")
	case '\r', '\n':
		return coretypes.MakeKeyword(STRINGS.Intern, "enter")
	case 3, 4: // Ctrl-C, Ctrl-D
		return coretypes.MakeKeyword(STRINGS.Intern, "eof")
	default:
		return coretypes.String{S: string(ch)}
	}
}

var procSleep ProcFn = func(args []coretypes.Object) coretypes.Object {
	ms := coretypes.EnsureArgIsInt(args, 0).I
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return NIL
}

var procMillis ProcFn = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Int{I: int(time.Now().UnixMilli())}
}

// extractRGB gets r,g,b from args[index] which should be a vector [r g b].
func extractRGB(args []coretypes.Object, index int) (int, int, int) {
	v := coretypes.EnsureArgIsSeqable(args, index)
	seq := v.Seq()
	r := seq.First().(coretypes.Int).I
	seq = seq.Rest()
	g := seq.First().(coretypes.Int).I
	seq = seq.Rest()
	b := seq.First().(coretypes.Int).I
	return r, g, b
}
