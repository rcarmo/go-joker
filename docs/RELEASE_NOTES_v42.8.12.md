# Release Notes — v42.8.12

## Version

- Bumped runtime version from `v42.8.11` to `v42.8.12`.
- `joker --version` now reports `v42.8.12`.

## WASM bridge fixes

- Fixed WASM generation for value-producing `if` expressions in numeric loops, e.g.:

  ```clojure
  (let [a (if (< x 0.0) (- 0.0 x) x)]
    ...)
  ```

- Fixed fn-level WASM loops with initialization stores by placing the WASM `loop` at the recur target rather than before init stores.
- Fixed `compile-wasm` fallback flow so an “eligible for pure WASM backend” diagnostic is not treated as an error when falling through from the loop-wrapper path.
- Relaxed WASM eligibility for single-loop non-zero recur targets used by fn-level loop bodies.

## Examples

- Added/updated `examples/fractal-flame.joke`: high-resolution procedural raster generation in Joker code via:
  - `joker.jit/compile-wasm`
  - `joker.imaging/from-rgba32-domain-fn`
  - Mandelbrot, Tricorn, and cubic flame-style kernels

## Imaging API cleanup

- Removed the native Go `joker.imaging/fractal-flame` proc. Fractal rendering remains in Joker/WASM example code instead of expanding the imaging namespace with a special-purpose native helper.

## Verified

```bash
go test ./core -run 'TestWasm|TestIr|TestRuntime|TestJit' -count=1
go build -o .cache/joker ./cmd/joker
.cache/joker examples/fractal-flame.joke 1024 .cache/flame-mandel.png
git diff --check
```
