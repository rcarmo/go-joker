package term

import (
	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

var termNamespace = GLOBAL_ENV.EnsureSymbolIsLib(coretypes.MakeSymbol(STRINGS.Intern, "joker.term"))

func init() {
	termNamespace.Lazy = initTermNamespace
}

func initTermNamespace() {
	termNamespace.ResetMeta(MakeMeta(nil, "Terminal I/O: raw mode, ANSI escape sequences, key reading, screen manipulation.", "1.0"))

	procs := []struct {
		name string
		fn   ProcFn
		doc  string
	}{
		// Raw mode
		{"raw-mode!", procRawMode, "Enters raw terminal mode. Returns true. Call (restore!) to undo."},
		{"restore!", procRestore, "Restores terminal to its original mode (before raw-mode! was called)."},

		// Screen
		{"clear", procClear, "Clears the entire terminal screen."},
		{"alt-screen!", procAltScreen, "Switches to the alternate screen buffer."},
		{"main-screen!", procMainScreen, "Switches back to the main screen buffer."},
		{"hide-cursor!", procHideCursor, "Hides the terminal cursor."},
		{"show-cursor!", procShowCursor, "Shows the terminal cursor."},
		{"move-to", procMoveTo, "Moves cursor to column x, row y (0-based)."},
		{"size", procSize, "Returns terminal size as {:cols N :rows N}."},

		// Output
		{"print!", procPrint, "Prints string at current cursor position without newline."},
		{"flush!", procFlush, "Flushes stdout."},

		// Colors / styles
		{"fg", procFg, "Returns ANSI escape string to set foreground to [r g b] or a named color keyword."},
		{"bg", procBg, "Returns ANSI escape string to set background to [r g b] or a named color keyword."},
		{"reset-style", procResetStyle, "Returns ANSI escape string to reset all styles."},

		// Input
		{"read-key", procReadKey, "Reads one keypress with optional timeout-ms (default 50). Returns a keyword (:up :down :left :right :space :enter :esc :none :eof) or a single-char string."},

		// Utility
		{"sleep", procSleep, "Sleeps for the given number of milliseconds."},
		{"millis", procMillis, "Returns current time in milliseconds (monotonic-ish)."},
	}

	for _, p := range procs {
		termNamespace.InternVar(p.name, Proc{Fn: p.fn, Name: "term/" + p.name},
			MakeMeta(nil, p.doc, "1.0"))
	}
}
