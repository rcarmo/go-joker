package term

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"

	"golang.org/x/term"
)

// Terminal state saved when entering raw mode.
var oldState *term.State

// stdin reader state for timeout-capable key reading across real TTYs/PTYS.
type stdinReadResult struct {
	b   byte
	err error
}

var stdinReadOnce sync.Once
var stdinReadCh chan stdinReadResult
var stdinPendingMu sync.Mutex
var stdinPending []byte

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

var procBold ProcFn = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: "\033[1m"}
}

var procDim ProcFn = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: "\033[2m"}
}

var procItalic ProcFn = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: "\033[3m"}
}

var procUnderline ProcFn = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: "\033[4m"}
}

var procInverse ProcFn = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: "\033[7m"}
}

var procStrikethrough ProcFn = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: "\033[9m"}
}

var procReadKey ProcFn = func(args []coretypes.Object) coretypes.Object {
	timeoutMs := 50
	if len(args) > 0 {
		timeoutMs = coretypes.EnsureArgIsInt(args, 0).I
	}

	return readKeyWithTimeout(time.Duration(timeoutMs) * time.Millisecond)
}

var procSleep ProcFn = func(args []coretypes.Object) coretypes.Object {
	ms := coretypes.EnsureArgIsInt(args, 0).I
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return NIL
}

var procMillis ProcFn = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Int{I: int(time.Now().UnixMilli())}
}

func makeTermKeyword(name string) coretypes.Object {
	return coretypes.MakeKeyword(STRINGS.Intern, name)
}

func ensureStdinReader() {
	stdinReadOnce.Do(func() {
		stdinReadCh = make(chan stdinReadResult, 128)
		go func() {
			buf := make([]byte, 1)
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					stdinReadCh <- stdinReadResult{b: buf[0]}
				}
				if err != nil {
					stdinReadCh <- stdinReadResult{err: err}
					return
				}
			}
		}()
	})
}

func unreadStdinByte(b byte) {
	stdinPendingMu.Lock()
	stdinPending = append(stdinPending, b)
	stdinPendingMu.Unlock()
}

func readStdinByte(timeout time.Duration) (byte, bool, error) {
	stdinPendingMu.Lock()
	if n := len(stdinPending); n > 0 {
		b := stdinPending[n-1]
		stdinPending = stdinPending[:n-1]
		stdinPendingMu.Unlock()
		return b, true, nil
	}
	stdinPendingMu.Unlock()

	ensureStdinReader()

	if timeout <= 0 {
		select {
		case res := <-stdinReadCh:
			if res.err != nil {
				return 0, false, res.err
			}
			return res.b, true, nil
		default:
			return 0, false, nil
		}
	}

	select {
	case res := <-stdinReadCh:
		if res.err != nil {
			return 0, false, res.err
		}
		return res.b, true, nil
	case <-time.After(timeout):
		return 0, false, nil
	}
}

func decodeReadByte(ch byte, next func(time.Duration) (byte, bool, error), unread func(byte)) coretypes.Object {
	if ch == 27 {
		b2, ok, err := next(10 * time.Millisecond)
		if err != nil || !ok {
			return makeTermKeyword("esc")
		}
		if b2 != '[' && b2 != 'O' {
			if unread != nil {
				unread(b2)
			}
			return makeTermKeyword("esc")
		}

		b3, ok, err := next(10 * time.Millisecond)
		if err != nil || !ok {
			return makeTermKeyword("esc")
		}
		switch b3 {
		case 'A':
			return makeTermKeyword("up")
		case 'B':
			return makeTermKeyword("down")
		case 'C':
			return makeTermKeyword("right")
		case 'D':
			return makeTermKeyword("left")
		default:
			return makeTermKeyword("esc")
		}
	}

	switch ch {
	case ' ':
		return makeTermKeyword("space")
	case '\r', '\n':
		return makeTermKeyword("enter")
	case 3, 4: // Ctrl-C, Ctrl-D
		return makeTermKeyword("eof")
	default:
		return coretypes.String{S: string(ch)}
	}
}

func readKey(timeout time.Duration, next func(time.Duration) (byte, bool, error), unread func(byte)) coretypes.Object {
	ch, ok, err := next(timeout)
	if err != nil {
		if err == io.EOF {
			return makeTermKeyword("eof")
		}
		return makeTermKeyword("none")
	}
	if !ok {
		return makeTermKeyword("none")
	}
	return decodeReadByte(ch, next, unread)
}

func readKeyWithTimeout(timeout time.Duration) coretypes.Object {
	return readKey(timeout, readStdinByte, unreadStdinByte)
}

// --- Buffered screen output for flicker-free rendering ---

var screenBuf []byte

var procBeginFrame ProcFn = func(args []coretypes.Object) coretypes.Object {
	screenBuf = screenBuf[:0]
	return NIL
}

var procEndFrame ProcFn = func(args []coretypes.Object) coretypes.Object {
	os.Stdout.Write(screenBuf)
	os.Stdout.Sync()
	return NIL
}

var procBufPrint ProcFn = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.ExtractString(args, 0)
	screenBuf = append(screenBuf, s...)
	return NIL
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
