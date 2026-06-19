# Release Notes — v42.9.1

This patch release rolls up behavior-preserving root-kernel maintainability work completed after `v42.9.0`.

## Version

- Bumped runtime version from `v42.9.0` to `v42.9.1`.
- `joker --version` now reports `v42.9.1`.

## Runtime maintainability

- Continued reducing `core/runtime_kernel.go` by moving cohesive same-package seams into focused files:
  - tail-call and TCO rewrite helpers;
  - fast reduce / reducible range pipeline helpers;
  - fast seq operation wrappers;
  - frequencies and split-whitespace fast paths;
  - reader construction adapter;
  - tagged literal registration helpers;
  - IR call dispatch helper;
  - WASM loop wrapper helper;
  - function IR cache accessors.
- Normalized extracted runtime seam filenames to the dominant `<seam>_runtime.go` convention.
- Kept higher-risk native loop wrapper logic in the root kernel until a stable direct behavior fixture exists.

## Contracts and validation

- Added a focused IR dispatch fallback contract for closure argument stabilization.
- Tightened the WASM loop wrapper contract to assert the `jit-wasm-loop` path for value-producing loops.
- Repeatedly validated extraction slices with focused contracts, `go test ./core`, and `git diff --check`.

Before tagging, run:

```bash
make pretag-check
```
