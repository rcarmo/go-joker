# Release Notes — v42.8.9

## New: `joker.term` namespace

Full terminal I/O for building TUI applications and games:

- **Raw mode**: `raw-mode!`, `restore!`
- **Screen control**: `alt-screen!`, `main-screen!`, `hide-cursor!`, `show-cursor!`, `clear`, `move-to`, `size`
- **Output**: `print!`, `flush!`
- **Buffered frames** (flicker-free): `begin-frame!`, `buf-print!`, `end-frame!`
- **24-bit color**: `fg`, `bg`, `reset-style`
- **Text styles**: `bold`, `dim`, `italic`, `underline`, `inverse`, `strikethrough`
- **Input**: `read-key` with timeout (arrow keys, special keys, characters)
- **Utility**: `sleep`, `millis`

Backed by `golang.org/x/term` for portable raw mode.

See [`docs/TERM.md`](TERM.md) for full API reference.

## New: `examples/tetris.joke`

Terminal Tetris — a full port of babashka's JLine-based Tetris to native Joker:

```bash
joker examples/tetris.joke
```

7 tetrominoes, rotation, hard/soft drop, line clearing, scoring, pause/reset.
Uses `joker.term` buffered frame rendering for smooth output.

## `joker.imaging`

- **WebP decode support** — `(imaging/open "photo.webp")` works out of the box
  (registered `golang.org/x/image/webp`, pure Go, no cgo)

## Verified

```bash
go test ./std/term ./std/imaging ./core/runtime -count=1
go build ./core/... ./std/... ./cmd/...
```
