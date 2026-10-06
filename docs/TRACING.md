# Tracing and profiling

Use these tools for targeted diagnostics and pre-release review. Ordinary local tests do not need profiles unless you are investigating a problem. Pre-release verification does: capture CPU plus heap/allocation behaviour, inspect cumulative CPU together with `alloc_space` and `alloc_objects`, write down the findings, then delete the raw captures and matching binaries/logs.

Start by sourcing the project environment so scratch paths resolve under the project-owned temporary root:

```bash
source scripts/project-env.sh
```

`PROFILE_ROOT` defaults to `$PROJECT_TMP_ROOT/runs/profiles`. Use a fresh run directory for each capture.

## 1. Go CPU and heap/allocation profiles

```bash
source scripts/project-env.sh
run="$PROFILE_ROOT/manual-clbg-$(date +%Y%m%d%H%M%S)"
mkdir -p "$run"
go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' \
  -benchtime=3s -count=1 \
  -cpuprofile="$run/cpu.pprof" \
  -memprofile="$run/heap.pprof" \
  -o="$run/core.test"
go tool pprof -top "$run/core.test" "$run/cpu.pprof" > "$run/cpu.txt"
go tool pprof -top -alloc_space "$run/core.test" "$run/heap.pprof" > "$run/alloc_space.txt"
go tool pprof -top -alloc_objects "$run/core.test" "$run/heap.pprof" > "$run/alloc_objects.txt"
```

Render the CPU profile with the repository's pure Joker SVG renderer:

```bash
source scripts/project-env.sh
make cli
"$CLI_BIN" docs/render-trace-svg.clj \
  "$run/cpu.pprof" "$run/clbg-go-sankey.svg" \
  "Go pprof trace — CLBG Binary Trees"
```

For allocation attribution, rerun with `-memprofilerate=1`. Do not use that setting for latency comparisons.

## 2. WASM engine context

Record the actual engine whenever the workload uses `joker.jit/compile-wasm`. `auto` may select different backends on different hosts.

```bash
source scripts/project-env.sh
make cli
"$CLI_BIN" --wasm-engine=compiler examples/wasm/native-sum.joke
"$CLI_BIN" --wasm-engine=interpreter examples/wasm/native-sum.joke
"$CLI_BIN" compile --native examples/wasm/native-sum.joke \
  -o "$PROJECT_TMP_ROOT/build/native-sum"
"$PROJECT_TMP_ROOT/build/native-sum" --wasm-engine=interpreter
```

`joker.jit/wasm-engine` reports the actual shared engine: `compiler` or `interpreter`. `compile` bundles the Joker runtime and source into a platform executable. `--native` saves `compiler` as the embedded engine choice. Runtime `--wasm-engine` flags or `JOKER_WASM_ENGINE` can still override that saved default.

## 3. IR opcode tracing

Enable optional IR opcode tracing with environment variables:

```bash
source scripts/project-env.sh
run="$PROFILE_ROOT/manual-clbg-$(date +%Y%m%d%H%M%S)"
mkdir -p "$run"
JOKER_IR_PROFILE=1 \
JOKER_IR_PROFILE_OUT="$run/clbg-ir-profile.json" \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' -benchtime=1s -count=1
```

The JSON includes total IR execution count, opcode counts, opcode transition counts, and elapsed nanosecond totals/averages.

Render it with the same repository script:

```bash
source scripts/project-env.sh
make cli
"$CLI_BIN" docs/render-trace-svg.clj \
  "$run/clbg-ir-profile.json" "$run/clbg-ir-sankey.svg" \
  "IR opcode trace — CLBG Binary Trees"
```

## 4. Joker function and symbol tracing

Function tracing:

```bash
source scripts/project-env.sh
run="$PROFILE_ROOT/manual-clbg-$(date +%Y%m%d%H%M%S)"
mkdir -p "$run"
JOKER_FUNCTION_TRACE=1 \
JOKER_FUNCTION_TRACE_OUT="$run/clbg-function-trace.json" \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' -benchtime=1s -count=1
```

Symbol tracing:

```bash
source scripts/project-env.sh
run="$PROFILE_ROOT/manual-clbg-$(date +%Y%m%d%H%M%S)"
mkdir -p "$run"
JOKER_SYMBOL_TRACE=1 \
JOKER_SYMBOL_TRACE_OUT="$run/clbg-symbol-trace.json" \
  go test ./core -run '^$' -bench 'BenchmarkCLBGBinaryTrees$' -benchtime=1s -count=1
```

These traces are optional and disabled by default. They add overhead and are for diagnostics, not benchmark timing. Function and IR traces include elapsed nanosecond totals and averages, so the Sankey width can represent measured time instead of counts. Symbol tracing is count-only.

## 5. Pure Joker Sankey renderer

`docs/render-trace-svg.clj` turns supported JSON outputs and raw `pprof` files into compact SVG Sankey diagrams. IR opcode data is cyclic, so it is rendered as a two-column transition Sankey (`from/opcode -> to/opcode`) rather than as a fake acyclic progression.

Supported inputs:

- pprof Sankey JSON (`nodes`/`links`)
- `go-joker-ir-profile`
- `go-joker-function-trace`
- `go-joker-symbol-trace`

Usage:

```bash
source scripts/project-env.sh
make cli
"$CLI_BIN" docs/render-trace-svg.clj INPUT.{pprof,json} OUTPUT.svg "Optional title"
```

When `INPUT` is not JSON, the Joker script runs `go tool pprof -raw` and follows the existing pprof pipeline: parse raw sections, simplify function names, reverse leaf-first stacks, squash adjacent frames, accumulate node and edge time, compute stack-derived depths and render the static SVG layout.

## 6. Disposal after review

Keep raw profiles, traces, matching test binaries and disposable logs only while the analysis is in progress. After the review, record a short conclusion and delete the run directory.

```bash
scripts/dispose-profiles.sh "$run" \
  "core CLBG; Go <toolchain>; engine=<compiler|interpreter>; note top CPU and allocation hotspots here"
```

`scripts/dispose-profiles.sh` writes a compact note under `.cache/profile-conclusions` and removes the raw run directory. If you captured ad hoc files outside `PROFILE_ROOT`, remove them at the same point.

## Notes

- Go CPU sample averages reflect the profiler sample period. The default is often about 10 ms per sample.
- Short workloads can produce a valid CPU profile with zero samples. Use a longer representative workload for CPU attribution.
- For WASM-backed workloads, compare equivalent runs under the same actual engine.
- Do not compare benchmark timings with tracing enabled.
