# Tracing and profile rendering

This repository has three complementary tracing paths for performance work.

## 1. Go CPU profiles

Use normal Go `pprof` to profile the Go runtime while Joker code runs:

```bash
mkdir -p .cache/tmp .cache/gotmp
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/gotmp \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' \
  -benchtime=3s -cpuprofile=$PWD/.cache/tmp/clbg.pprof
```

Render the pprof call flow with the repository's pure Joker SVG renderer:

```bash
make cli
.cache/tmp/joker docs/render-trace-svg.clj \
  .cache/tmp/clbg.pprof .cache/tmp/clbg-go-sankey.svg \
  "Go pprof trace — CLBG Binary Trees"
```

## 2. IR opcode tracing

Enable optional IR opcode tracing with environment variables:

```bash
JOKER_IR_PROFILE=1 \
JOKER_IR_PROFILE_OUT=$PWD/.cache/tmp/clbg-ir-profile.json \
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/gotmp \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' -benchtime=1s -count=1
```

The JSON includes:

- total IR execution count
- opcode counts
- opcode transition counts
- opcode/transition elapsed nanoseconds and average nanoseconds

Render it with the same repository script:

```bash
.cache/tmp/joker docs/render-trace-svg.clj \
  .cache/tmp/clbg-ir-profile.json .cache/tmp/clbg-ir-sankey.svg \
  "IR opcode trace — CLBG Binary Trees"
```

## 3. Joker function and symbol tracing

Function tracing:

```bash
JOKER_FUNCTION_TRACE=1 \
JOKER_FUNCTION_TRACE_OUT=$PWD/.cache/tmp/clbg-function-trace.json \
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/gotmp \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' -benchtime=1s -count=1
```

Symbol tracing:

```bash
JOKER_SYMBOL_TRACE=1 \
JOKER_SYMBOL_TRACE_OUT=$PWD/.cache/tmp/clbg-symbol-trace.json \
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/gotmp \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' -benchtime=1s -count=1
```

These are intentionally optional and disabled by default. They add overhead and are for diagnostics, not benchmark timing. Function and IR traces include elapsed nanosecond totals/averages, so the Sankey width can represent measured time instead of just counts.

## Pure Joker Sankey renderer

`docs/render-trace-svg.clj` is a small Joker/Clojure renderer that turns these JSON outputs into compact SVG Sankey diagrams. IR opcode data is cyclic, so it is rendered as a two-column transition Sankey (`from/opcode -> to/opcode`) rather than as a fake acyclic progression.

- pprof Sankey JSON (`nodes`/`links`)
- `go-joker-ir-profile`
- `go-joker-function-trace`
- `go-joker-symbol-trace`

Usage:

```bash
.cache/tmp/joker docs/render-trace-svg.clj INPUT.{pprof,json} OUTPUT.svg "Optional title"
```

When `INPUT` is not JSON, the Joker script runs `go tool pprof -raw` itself and follows the TypeScript renderer's pprof pipeline: parse raw sections, simplify function names, reverse leaf-first stacks, squash adjacent frames, accumulate node/edge time, compute stack-derived depths, and render the same static SVG layout.

Examples:

```bash
.cache/tmp/joker docs/render-trace-svg.clj \
  .cache/tmp/clbg.pprof \
  .cache/tmp/clbg-go-trace-clj.svg \
  "Go pprof trace — CLBG Binary Trees"

.cache/tmp/joker docs/render-trace-svg.clj \
  .cache/tmp/clbg-function-trace.json \
  .cache/tmp/clbg-joker-function-trace-clj.svg \
  "Joker function trace — CLBG Binary Trees"

.cache/tmp/joker docs/render-trace-svg.clj \
  .cache/tmp/clbg-ir-profile.json \
  .cache/tmp/clbg-ir-trace-clj.svg \
  "IR opcode trace — CLBG Binary Trees"
```

## Notes

- Go CPU profile sample averages reflect the Go profiler sample period. By default this is often 10ms/sample.
- For finer CPU sampling, use a custom harness that calls `runtime.SetCPUProfileRate` before starting CPU profiling.
- IR/function traces now include elapsed nanoseconds; symbol tracing remains count-only.
- Do not compare benchmark timings with tracing enabled.
