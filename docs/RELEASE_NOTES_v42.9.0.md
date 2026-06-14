# Release Notes — v42.9.0

This minor release rolls up the contributor-predictability and root-kernel maintainability work completed after `v42.8.13`.

## Version

- Bumped runtime version from `v42.8.13` to `v42.9.0`.
- `joker --version` now reports `v42.9.0`.

## Contributor and release hygiene

- Added and documented `make pretag-check` as the local pre-tag release gate.
- Tightened contributor entry-point guidance so `docs/START_HERE.md` is the primary path through build, focused validation, API-stability, fuzz-smoke, and release-check expectations.
- Clarified that refactor docs are boundary inventory/historical context, while active work should live in issue/plan tooling unless durable user-facing documentation is needed.

## API stability and input hardening

- Completed public namespace/API stability classifications for the current generated namespace surface.
- Added bounded fuzz smoke coverage for reader string literals, EDN decoding, Transit decoding, and HTTP request-map conversion.
- Documented when to run the fuzz smoke targets for parser/codec boundary changes.

## Runtime maintainability

- Extracted focused same-package runtime helper clusters from `core/runtime_kernel.go` without behavior changes:
  - WASM compile/runtime helpers.
  - `IntRange` fast-range and hot-reducer helpers.
  - runtime error/frame/stacktrace helpers.
  - transducer compatibility helpers.
- Added or reused focused contract validation around WASM metadata, range/reduce behavior, error/stacktrace behavior, and transducer/reduce semantics.

## Validation

Before tagging, run:

```bash
make pretag-check
```

Use browser smoke locally when browser dependencies are installed:

```bash
PRETAG_BROWSER_SMOKE=1 make pretag-check
```
