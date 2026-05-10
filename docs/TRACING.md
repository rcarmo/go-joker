# Tracing and profile rendering

This repository has three complementary tracing paths for performance work.

## 1. Go CPU profiles

Use normal Go `pprof` to profile the Go runtime while Joker code runs:

```bash
TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' \
  -benchtime=3s -cpuprofile=/workspace/tmp/clbg.pprof
```

Render the pprof call-flow Sankey with the workspace skill:

```bash
bun /workspace/.pi/skills/go-pprof-sankey/pprof-sankey.ts \
  /workspace/tmp/clbg.pprof \
  --out-svg /workspace/tmp/clbg-go-sankey.svg \
  --out-html /workspace/tmp/clbg-go-sankey.html \
  --out-json /workspace/tmp/clbg-go-sankey.json \
  --top 60
```

## 2. IR opcode tracing

Enable optional IR opcode tracing with environment variables:

```bash
JOKER_IR_PROFILE=1 \
JOKER_IR_PROFILE_OUT=/workspace/tmp/clbg-ir-profile.json \
TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' -benchtime=1s -count=1
```

The JSON includes:

- total IR execution count
- opcode counts
- opcode transition counts

Render with the skill helper:

```bash
bun /workspace/.pi/skills/go-pprof-sankey/ir-profile-sankey.ts \
  /workspace/tmp/clbg-ir-profile.json \
  --out-svg /workspace/tmp/clbg-ir-sankey.svg
```

## 3. Joker function and symbol tracing

Function tracing:

```bash
JOKER_FUNCTION_TRACE=1 \
JOKER_FUNCTION_TRACE_OUT=/workspace/tmp/clbg-function-trace.json \
TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' -benchtime=1s -count=1
```

Symbol tracing:

```bash
JOKER_SYMBOL_TRACE=1 \
JOKER_SYMBOL_TRACE_OUT=/workspace/tmp/clbg-symbol-trace.json \
TMPDIR=/workspace/tmp GOTMPDIR=/workspace/tmp \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' -benchtime=1s -count=1
```

These are intentionally optional and disabled by default. They add overhead and are for diagnostics, not benchmark timing.

## Pure Joker trace renderer

`docs/render-trace-svg.clj` is a small Joker/Clojure renderer that turns any of these JSON outputs into a compact SVG bar report:

- pprof Sankey JSON (`nodes`/`links`)
- `go-joker-ir-profile`
- `go-joker-function-trace`
- `go-joker-symbol-trace`

Usage:

```bash
./joker docs/render-trace-svg.clj INPUT.json OUTPUT.svg "Optional title"
```

Examples:

```bash
./joker docs/render-trace-svg.clj \
  /workspace/tmp/clbg-go-sankey.json \
  /workspace/tmp/clbg-go-trace-clj.svg \
  "Go pprof trace — CLBG Binary Trees"

./joker docs/render-trace-svg.clj \
  /workspace/tmp/clbg-function-trace.json \
  /workspace/tmp/clbg-joker-function-trace-clj.svg \
  "Joker function trace — CLBG Binary Trees"

./joker docs/render-trace-svg.clj \
  /workspace/tmp/clbg-ir-profile.json \
  /workspace/tmp/clbg-ir-trace-clj.svg \
  "IR opcode trace — CLBG Binary Trees"
```

## Notes

- Go CPU profile sample averages reflect the Go profiler sample period. By default this is often 10ms/sample.
- For finer CPU sampling, use a custom harness that calls `runtime.SetCPUProfileRate` before starting CPU profiling.
- IR/function/symbol tracing counts events, not wall-clock duration.
- Do not compare benchmark timings with tracing enabled.
