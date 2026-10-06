# Go profiling and allocation review

Pre-release checks must capture CPU and heap profiles, inspect application costs and tune measured bottlenecks. Ordinary development tests need not profile. Delete raw captures, matching binaries, traces and disposable logs immediately after analysis/use; keep concise findings and measurements.

## Commands and disposal

```sh
source scripts/project-env.sh
make test-repro                       # ordinary development tests
PROFILE_TESTS=1 make test-core         # diagnostic profiling
PRETAG_BROWSER_SMOKE=1 make pretag-check
```

Profiles use `$PROJECT_TMP_ROOT/runs/profiles`. The runner tests packages separately, producing cumulative CPU, `alloc_space` and `alloc_objects` reports. Heap sampling defaults to 512 KiB; `PROFILE_MEM_RATE=1` provides exact allocation attribution with greater instrumentation cost. Empty CPU samples in short tests are not hotspot evidence. Parent profiles do not cover child CLI processes or native-library heap activity.

After reviewing a capture:

```sh
scripts/dispose-profiles.sh "$run" "workload, result, hotspot, change and limitations"
```

The helper saves compact conclusions under `.cache/profile-conclusions` and removes the raw run. CI keeps compact diagnostic findings and disposes raw captures. Do not upload raw profile archives or move them into report/export directories.

Use matched workloads/toolchains/flags before attributing an improvement. Instrumented benchmark latency is diagnostic; measure uninstrumented throughput separately. Coverage and `-benchmem` alone do not replace CPU and allocation profiles.

## Goroutine-state lookup

The first profiled core suite attributed about 101 MB of allocation to the escaping 64-byte stack-header buffer in `core/runtime.GoID`. Pooling that private buffer reduced both GoID and registered-state lookup from 64 B/op and one allocation to zero in six matched focused samples. GC can clear the pool, so zero steady-state allocation is not a lifetime guarantee.

Core allocated-byte samples fell from about 319 MB to 220 MB, but concurrent suite totals are noisy. The focused per-operation comparison establishes the change. `runtime.Stack` still dominates CPU when spawned interpreter states exist; replacing it requires explicit state ownership and concurrency/fallback tests, not undocumented runtime offsets.

## Standalone packaging

The WASM/CLI follow-up profiled two packaging variants on Go 1.27.1, Linux amd64, Intel Core i7-12700. Six samples per variant used two builds each, including source/metadata and filesystem writes. Reading the complete runtime dominated allocated bytes (over 99% of the sampled baseline).

Streaming the runtime and reusing the footer reduced:

| Variant | Baseline bytes/op | Streaming bytes/op | Allocations/op |
| --- | --- | --- | --- |
| Legacy source payload | 46,177,632 | 7,536 | 74 -> 74 |
| Saved compiler mode | 46,179,152 | about 8,612 | 86 -> 85 |

This removes about 99.98% of Go allocation bytes in the measured packaging workload. Timings were instrumented and filesystem-noisy; no general startup/execution speedup is claimed. Metadata, malformed-length, safe replacement, source-deleted execution and argument/engine-override tests pass. Raw baseline, intermediate and candidate captures were removed after comparison.

## Other observed costs

Vector construction and IR execution lead core allocations after GoID pooling. `BuildVector` copies temporary IR input deliberately; removing that copy without ownership transfer could break immutable collection behaviour. String differential-test fixture creation accounts for most allocations in that test package and is not an application optimization target.

Notebook profiles highlight `io.ReadAll` and byte/string copying around embedded assets and test scaffolding. Repeat a realistic request workload before changing ownership or asset caching. Short CLI/HTTP/PDF/collection captures often show bootstrap, runtime/profiler and compression overhead; no third-party changes are justified by those samples.

The SDL fluid solver uses persistent arrays and records zero steady-state allocations for step/RGBA conversion. Its iterative relaxation dominates CPU. Reducing solver passes changes the simulation; it was not used to improve a benchmark. Native SDL allocations are outside Go heap profiling.
