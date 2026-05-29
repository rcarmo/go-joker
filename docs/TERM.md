# joker.term — Terminal I/O

Raw terminal mode, ANSI escape sequences, key reading, buffered screen rendering.

Backed by `golang.org/x/term` for portable raw mode on Linux/macOS/Windows.

## Quick start

```clojure
(require '[joker.term :as t])

(t/raw-mode!)
(t/alt-screen!)
(t/hide-cursor!)

;; game loop
(loop []
  (t/begin-frame!)
  (t/buf-print! (str (t/fg [255 100 0]) "Hello" (t/reset-style)))
  (t/end-frame!)
  (let [k (t/read-key 100)]
    (when-not (= k "q")
      (recur))))

(t/show-cursor!)
(t/main-screen!)
(t/restore!)
```

## API

### Terminal mode

| Function | Description |
|----------|-------------|
| `(term/raw-mode!)` | Enter raw terminal mode (no echo, no line buffering) |
| `(term/restore!)` | Restore terminal to original state |

### Screen control

| Function | Description |
|----------|-------------|
| `(term/alt-screen!)` | Switch to alternate screen buffer |
| `(term/main-screen!)` | Switch back to main screen buffer |
| `(term/hide-cursor!)` | Hide cursor |
| `(term/show-cursor!)` | Show cursor |
| `(term/clear)` | Clear entire screen and move to top-left |
| `(term/move-to x y)` | Move cursor to column x, row y (0-based) |
| `(term/size)` | Returns `{:cols N :rows N}` |

### Output

| Function | Description |
|----------|-------------|
| `(term/print! s)` | Print string immediately (no newline) |
| `(term/flush!)` | Flush stdout |

### Buffered frame rendering (flicker-free)

| Function | Description |
|----------|-------------|
| `(term/begin-frame!)` | Start accumulating output in buffer |
| `(term/buf-print! s)` | Append to frame buffer |
| `(term/end-frame!)` | Write entire buffer to stdout in one syscall, then flush |

Use `begin-frame!` / `buf-print!` / `end-frame!` for smooth game/TUI rendering.
The entire frame is written in a single `write()` call, eliminating flicker.

### Colors & styles

All style functions return ANSI escape **strings**. Concatenate them with your text:

```clojure
(str (t/bold) (t/fg [255 0 0]) "ERROR" (t/reset-style))
```

| Function | Description |
|----------|-------------|
| `(term/fg [r g b])` | 24-bit foreground color |
| `(term/bg [r g b])` | 24-bit background color |
| `(term/reset-style)` | Reset all attributes |
| `(term/bold)` | Bold |
| `(term/dim)` | Dim/faint |
| `(term/italic)` | Italic |
| `(term/underline)` | Underline |
| `(term/inverse)` | Reverse video |
| `(term/strikethrough)` | Strikethrough |

### Input

| Function | Description |
|----------|-------------|
| `(term/read-key)` | Read keypress with 50ms timeout |
| `(term/read-key timeout-ms)` | Read keypress with custom timeout |

Returns:
- Arrow keys: `:up`, `:down`, `:left`, `:right`
- Special: `:space`, `:enter`, `:esc`, `:eof`
- Timeout: `:none`
- Printable character: single-character string like `"a"`, `"q"`

### Utility

| Function | Description |
|----------|-------------|
| `(term/sleep ms)` | Sleep for N milliseconds |
| `(term/millis)` | Current wall-clock time in milliseconds |

## Examples

- `examples/tetris.joke` — full terminal Tetris (7 pieces, rotation, scoring, hard drop)

## Design notes

- **No dependency on go-te**: go-te is a terminal *emulator* (VT100 state machine for
  parsing incoming byte streams). `joker.term` needs the opposite direction: *producing*
  ANSI output and reading raw keystrokes. `golang.org/x/term` handles raw mode portably.

- **Buffered frames**: The `begin-frame!`/`end-frame!` pattern achieves the same
  flicker-free rendering as JLine's `Display` (used in babashka) or ncurses `refresh()`,
  without needing a full screen-diff library.

- **24-bit color only**: Modern terminals support truecolor. We don't fall back to
  256-color or 16-color palettes. If you need that, emit raw escape codes via `print!`.
