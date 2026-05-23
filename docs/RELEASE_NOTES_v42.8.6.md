# go-joker v42.8.6 Release Notes

`v42.8.6` is a patch release focused on notebook rendering stability, faster bitmap notebook examples, and notebook runtime usability after `v42.8.5`.

## Version

- Bumped runtime version from `v42.8.5` to `v42.8.6`.
- `joker --version` now reports `v42.8.6`.

## Notebook runtime

- Added per-cell execution timing persisted as `:elapsed-ns` and shown in the notebook UI.
- Improved CodeMirror cursor visibility in notebook editors.
- Prevented overlapping notebook evaluations from racing the same notebook state, which could duplicate image outputs.
- Added regression coverage for concurrent notebook evaluation requests.

## Bitmap and WASM examples

- Added direct RGBA32 image generation helpers for procedural pixel functions and continuous-domain numeric kernels.
- Optimized pure numeric WASM calls to avoid unnecessary object-table setup.
- Updated the complex demo notebook to render Mandelbrot output through the direct domain raster path.

## Verified checks

Validated locally with targeted commands:

```bash
go test ./internal/notebook -count=1
go test -race ./internal/notebook -count=1
go test ./std/imaging -count=1
go test ./core -run TestWasm -count=1
git diff --check
```
