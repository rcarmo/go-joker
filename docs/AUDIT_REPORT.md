# AUDIT_REPORT

Date: 2026-05-07

## Scope

Full pass over core/runtime-critical paths and std namespaces with emphasis on:

- `core/` parser/eval/IR/WASM execution safety
- `std/` error semantics and resource handling
- static analysis findings (`vet`, `staticcheck`, `golangci-lint`)
- dependency vulnerability posture (`govulncheck`)

## Verification matrix

All checks were executed and are green:

- `go test ./...`
- `go vet ./...`
- `staticcheck -checks=SA* ./...`
- `golangci-lint run --disable-all -E govet -E staticcheck` (generated files excluded)
- `govulncheck ./...` → **No vulnerabilities found**

Race/fuzz:

- `go test -race ./core ./std/runtime ./std/http ./std/pdf` → pass
- `std/http` includes request-map fuzz coverage.
- `std/edn` and `std/transit` include bounded decode fuzz smoke targets for malformed input hardening.

## High-value issues found and fixed

1. **Potential benchmark helper hang** (`std/runtime/procBenchmark`)
   - Previous calibration loop could repeatedly grow iterations and stall tests.
   - Added bounded calibration strategy and max-iteration cap.

2. **Reader conditional logic bug + panic** (then `core/read.go`, now coalesced into `core/runtime_kernel.go`)
   - `forms` queue was shadowed inside `readCondList`, causing incorrect state handling.
   - `readMulti` could panic on empty splice (`#?@`) expansion due to pop from empty queue.
   - Fixed queue handling and added loop to keep reading until queue is populated.

3. **Unsafe pointer smell in `noescape` helper** (`core/noescape.go`)
   - `go vet` flagged unsafe pointer misuse.
   - Replaced with identity helper to keep call sites intact and maintain vet-clean builds.

4. **Ignored errors in data/resource paths**
   - `core/deps.go`: now checks `MkdirAll` and `os.Create` ordering/error flow.
   - `std/bolt`: now propagates `Update`/`View` errors instead of silently ignoring top-level DB errors.
   - `std/filepath`: now checks `filepath.Walk` return value.
   - `std/pdf`: now checks and reports errors from `SetFontSize`, `Cell`, `MultiCell`, `Image`.

5. **Deprecated APIs and analyzer noise cleanup**
   - Replaced `ioutil` usages in core/std (`os.ReadFile`, `io.ReadAll`, `os.ReadDir`, etc.).
   - Removed/cleaned unreachable or ineffectual analyzer findings in IR and helper paths.

6. **Dependency security upgrades**
   - `github.com/go-git/go-git/v5` → `v5.17.1`
   - `golang.org/x/image` → `v0.39.0`
   - Toolchain directive updated to `go1.25.9`
   - Result: `govulncheck` clean.

## Benchmark sanity check

Executed a CLBG sanity subset (`-benchtime=1x`, 3 runs each):

- `BenchmarkCLBGNBody`
- `BenchmarkCLBGSpectralNorm`
- `BenchmarkCLBGBinaryTrees`
- `BenchmarkCLBGFannkuchRedux`
- `BenchmarkCLBGMandelbrot`

Observed results stayed within expected variance for short benchtime runs, with no catastrophic regression from audit fixes.

## Residual items

- Existing fuzz targets cover HTTP request-map conversion plus bounded EDN and Transit decode inputs; broader reader/evaluator fuzzing would still improve future resilience.
- Generated files still produce style noise under broad linters; these remain intentionally excluded from style-only checks.
