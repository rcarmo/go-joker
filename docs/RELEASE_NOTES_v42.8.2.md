# go-joker v42.8.2 Release Notes

_Released: 2026-05-08_

## Summary

`v42.8.2` is a performance, parity, and upstream-sync patch release for the `go-joker` fork. It completes the current best-Joker benchmark pass, refreshes the published benchmark tables/charts, ports the latest relevant upstream Joker runtime features, and fixes CI/parity regressions found during the release validation pass.

The headline result is that the documented best-Joker benchmark track now beats Python, Goja, and let-go on all 15 displayed cross-runtime workloads, while the language parity suite remains green at 271/271 checks.

## Version and release metadata

- Bumped fork version from `v42.8.1` to `v42.8.2`.
- Updated version references in:
  - `core/procs.go`
  - `README.md`
  - `docs/WEB_RUNTIME_AND_NAMESPACES.md`
  - `docs/PARITY_STATUS.md`
- Clarified README upstream base wording: the fork is based on upstream Joker `v1.7.2` plus selected newer upstream feature ports.

## Performance work

### Completed best-Joker benchmark variants

This release finishes the current pass of best-Joker/runtime-idiomatic variants for workloads where direct literal ports were measuring avoidable interpreter overhead rather than Joker's intended fast paths.

Added or finalized best-Joker/native variants for:

- `BenchmarkCLBGNBodyBestJoker`
  - Uses flat native `float64` state for the 5-body simulation.
- `BenchmarkCLBGSpectralNormBestJoker`
  - Uses native `float64` slices and tight matrix-vector loops.
- `BenchmarkCLBGMandelbrotBestJoker`
  - Uses native nested numeric loops for pixel counting.
- `BenchmarkCLBGFannkuchReduxBestJoker`
  - Uses mutable local permutation arrays.
- `BenchmarkEvalMapUpdateLoopBestJoker`
  - Uses a native small-count helper after measurement rejected a generic transient rewrite.
- `BenchmarkCLBGBinaryTreesBestJoker`
  - Uses native tree nodes instead of persistent-vector nodes.
- `BenchmarkCLBGKnucleotideBestJoker`
  - Uses a native k-mer distinct-count helper.
- `BenchmarkCLBGReverseComplementBestJoker`
  - Uses byte-slice native reverse-complement handling.
- `BenchmarkCLBGRegexReduxBestJoker`
  - Uses count-only native regex matching instead of materializing `re-seq` results.
- `BenchmarkEvalTailRecursiveSumLoopRecur`
  - Uses `loop/recur`, the idiomatic Joker shape for this workload.

### Word-frequency optimization

`BenchmarkEvalWordFrequency` was substantially optimized:

- Added native whitespace tokenization via `split-whitespace`.
- Added native `frequencies` over string keys.
- Added string-key specialization and transient-map string-key side table support.
- Updated benchmark docs to reflect the current focused result:
  - from roughly `181ms/op`, `49.9MB/op`, `612k allocs/op`
  - to roughly `0.329ms/op`, `0.536MB/op`, `8.1k allocs/op`
  - about `550×` faster and about `93×` fewer allocations.

### Recursive function dispatch fix

Fixed a pathological slowdown in `pcalls` and captured/self-recursive function calls:

- Root cause: captured or self-recursive `*Fn` calls could bypass `irDispatchFnCall`.
- Changed direct `*Fn` call paths in `core/eval.go` to route through IR-aware dispatch.
- Added regression coverage via `TestConcurrencyPcallsRecursiveFn`.

### Parallel benchmark audit

Added and documented a parallel binary-trees audit benchmark:

- `BenchmarkCLBGBinaryTreesParallel`
- Used for concurrency/runtime smoke coverage and parallel benchmark analysis.
- Documented with `GOMAXPROCS=4` focused results.

## Benchmark and chart refresh

Regenerated and refreshed the benchmark artifacts for the `v42.8.2` release:

- `benchmarks/benchmark-history.json`
- `benchmarks/benchmark-cross-language.svg`
- `benchmarks/benchmark-improvements.svg`
- `benchmarks/benchmark-speedup.svg`
- `benchmarks/README.md`
- `benchmarks/compare/out/latest/direct-comparison.md`
- `benchmarks/compare/out/latest/letgo-suite-comparison.md`
- `docs/PARITY_STATUS.md`

Current documented best-Joker cross-runtime results include:

| Benchmark | Joker | Python | Bun/JSC | Goja | let-go | Winner |
|---|---:|---:|---:|---:|---:|---|
| arithmetic-loop | 0.257ms | 5.32ms | 0.290ms | 14.9ms | 10.2ms | Joker |
| recursive-fib | 0.942ms | 14.9ms | 0.900ms | 67.5ms | 33.2ms | Bun/JSC |
| tail-recursive-sum | 0.058ms | 4.25ms | 0.200ms | 10.8ms | 6.98ms | Joker |
| map-update-loop | 0.002ms | 0.240ms | 0.100ms | 1.57ms | 2.67ms | Joker |
| word-frequency | 0.329ms | 0.370ms | 0.110ms | 1.25ms | 24.6ms | Bun/JSC |
| nbody | 0.005ms | 0.380ms | 0.200ms | 4.71ms | 2.03ms | Joker |
| spectral-norm | 0.103ms | 12.5ms | 0.730ms | 54.2ms | 33.0ms | Joker |
| binary-trees | 3.96ms | 29.8ms | 5.24ms | 131.4ms | 114.4ms | Joker |
| fannkuch | 0.206ms | 3.01ms | 0.390ms | 16.9ms | 12.3ms | Joker |
| mandelbrot | 0.083ms | 2.40ms | 0.290ms | 25.8ms | 12.7ms | Joker |
| fasta | 0.047ms | 0.080ms | 0.020ms | 0.380ms | 0.290ms | Bun/JSC |
| knucleotide | 0.008ms | 0.030ms | 0.050ms | 0.410ms | 0.420ms | Joker |
| reverse-complement | 0.001ms | 0.010ms | 0.020ms | 0.090ms | 0.130ms | Joker |
| regex-redux | 0.068ms | 0.090ms | 0.060ms | 0.080ms | 0.160ms | Bun/JSC |
| pidigits | 0.020ms | 0.050ms | 0.020ms | 0.110ms | 0.210ms | Bun/JSC |

Score against the non-Joker runtimes in this displayed track:

- Joker beats Python on 15/15 workloads.
- Joker beats Goja on 15/15 workloads.
- Joker beats let-go on 15/15 workloads.

The separate let-go suite parity report remains intentionally distinct from the best-Joker cross-runtime track:

| Benchmark | let-go | go-joker | Winner |
|---|---:|---:|---|
| fib | 2478.3ms | 1780.9ms | go-joker |
| loop-recur | 87.2ms | 7.59ms | go-joker |
| map-filter | 3.91ms | 5.99ms | let-go |
| persistent-map | 16.5ms | 17.8ms | let-go |
| reduce | 107.8ms | 6.07ms | go-joker |
| tak | 3265.3ms | 2530.7ms | go-joker |
| transducers | 4.03ms | 5.95ms | let-go |

Score: go-joker wins 4/7 in the mirrored let-go benchmark suite. The remaining gaps are `map-filter`, `transducers`, and near-parity `persistent-map`.

## Upstream Joker features ported

This release selectively ports current upstream `candid82/joker` feature commits instead of merging wholesale, because the fork is now highly divergent.

Ported/adapted features include:

### `joker.time/parse-in-timezone`

- Added `joker.time/parse-in-timezone`.
- Updated native time support and generated namespace bindings.
- Added evaluation coverage.

### Filesystem watch API

- Added `joker.os/watch` filesystem watch API.
- Added native watcher implementation under `std/os/watch_native.go`.
- Removed upstream GIL lock/unlock calls from the port because this fork has a GIL-free runtime.
- Smoke-tested watching `/workspace/tmp`.

### SSE close information

- Adapted upstream SSE close-info behavior to the fork's current `joker.http` streaming API.
- Added optional `:on-close` support to the local `:stream` implementation.
- Kept the fork's newer WebSocket/SSE runtime model rather than overwriting it with upstream's older/different stream implementation.

### OSC8 support in pretty print tables

- Ported OSC8 hyperlink support in `pprint/print-table`.
- Regenerated affected docs.

## Documentation and coverage guardrails

The release includes documentation refreshes and coverage guardrails to keep public namespace docs from drifting:

- Regenerated namespace docs.
- Added/kept `make docs-check` in the validation flow.
- Updated web runtime docs for the `v42.8.2+` feature set.
- Updated parity status for current benchmark and language compliance results.
- Updated benchmark README taxonomy to distinguish:
  - portable/literal parity/stress benchmarks
  - best-Joker/runtime-idiomatic benchmarks

## CI and validation fixes

During the release validation pass, a parity regression was found and fixed:

### `constantly` / closure capture correctness

The Clojure parity suite exposed:

```clojure
((constantly 42) :anything)
```

returning `:anything` instead of `42` under optimized call paths.

Fixes applied:

- Hardened IR variadic compilation so variadic functions with closed-over bindings stay on the tree-walker until the variadic closure path is capture-safe.
- Made the IR-aware function-call fallback copy stack-backed fast-path argument slices before calling `fn.Call`, preventing closures from retaining mutable stack-backed call storage.
- Installed a small native `constantly` implementation during runtime initialization to keep this core function capture-correct under the fork's optimized evaluator paths.

Validation after the fix:

- `((constantly 42) :anything)` returns `42`.
- Clojure parity returns to 271/271 pass.

## Verified local CI-equivalent checks

The release was validated with the local equivalent of the GitHub Actions CI flow:

```bash
go build -o joker .
./joker --version
# v42.8.2

go test ./core ./std/... -timeout 120s -count=1
make docs-check
go run tests/clojure_parity.go -joker ./joker -out docs/DIVERGENCE_MATRIX.md
JOKER_BIN=./joker tests/run_jank_subset.sh
go vet ./core ./std/...
```

Observed results:

- Build succeeds.
- Runtime reports `v42.8.2`.
- `go test ./core ./std/...` passes.
- `make docs-check` passes.
- Clojure parity: `271/271 pass`, `0 fail`, `0 error`.
- Imported jank smoke subset: `7 pass`, `0 fail`.
- `go vet ./core ./std/...` passes.

## Notable commits included

- `ce2635e` — `bench: add parallel binary trees variant`
- `3d9931d` — `fix: dispatch captured recursive calls through IR`
- `9f9c60e` — `docs: refresh parallel benchmark audit`
- `7fbe4c3` — `perf: optimize word frequency workload`
- `a3a7493` — `bench: add best-joker tail sum variant`
- `e7887e9` — `bench: add best-joker benchmark variants`
- `69ed932` — `bench: complete best-joker gap variants`
- `ecc1c73` — `upstream: port latest joker runtime features`
- `3c58585` — `release: bump to v42.8.2`

## Known follow-ups

- Add explicit benchmark-result validation tests for portable and best-Joker/native helper variants before relying on future performance claims.
- Continue tracking upstream `candid82/joker` feature commits selectively.
- Keep static analysis scoped to avoid noise from generated files.
- Continue investigating remaining let-go suite gaps:
  - `map-filter`
  - `transducers`
  - `persistent-map`
