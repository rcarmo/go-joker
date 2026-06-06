# Release Notes — v42.8.10

## Changes

### Runtime startup

- Fixed a critical startup performance regression in the main goroutine fast path.
- `core/runtime.GoRTPool.Current()` now skips expensive goroutine ID extraction when no worker goroutines have been spawned.
- This removes a major source of interpreter overhead during normal single-threaded script loading.

### `joker.term`

- Fixed `term/read-key` timeouts on real terminals and PTYs.
- Replaced the broken `os.Stdin.SetReadDeadline(...)` approach with a background stdin reader and timeout-aware polling.
- Restored the expected `:none` timeout behavior used by terminal apps and games.

### `examples/games/tetris.joke`

- `examples/games/tetris.joke` now starts promptly and responds to input correctly again.

## Verified

```bash
go test ./core/runtime ./std/term -count=1
go build -o .cache/tmp/joker ./cmd/joker
```
