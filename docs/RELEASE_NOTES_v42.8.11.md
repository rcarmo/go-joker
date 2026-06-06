# Release Notes — v42.8.11

## Version

- Bumped runtime version from `v42.8.10` to `v42.8.11`.
- `joker --version` now reports `v42.8.11`.

## Included fixes

This patch level includes the recent runtime startup, `joker.term`, and Makefile usability fixes already committed on `master`.

## Current master additions

- WASM bridge fixes for value-producing `if` expressions in numeric loops, e.g. `(let [a (if (< x 0.0) (- 0.0 x) x)] ...)`.
- Fn-level WASM loops with initialization stores now place the WASM loop at the recur target, avoiding re-running init stores on every iteration.
- `examples/graphics/fractal-flame.joke` demonstrates high-resolution procedural raster generation in Joker code via `joker.jit/compile-wasm` and `joker.imaging/from-rgba32-domain-fn`.
- Removed the earlier native `joker.imaging/fractal-flame` proc; fractal rendering now stays in Joker/WASM code rather than adding a Go-specific imaging API.

## Verified

```bash
go test ./core/runtime -count=1
go test ./core -run 'TestWasm|TestIr|TestRuntime|TestJit' -count=1
go build ./cmd/joker
joker examples/graphics/fractal-flame.joke 1024 .cache/flame-mandel.png
```
