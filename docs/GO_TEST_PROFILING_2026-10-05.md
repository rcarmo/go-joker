# Go test profiling and first allocation pass

Updated lifecycle: pre-release profiling remains mandatory; ordinary development tests need not profile. Raw profiles, matching binaries and completed logs from this report have been disposed after analysis. Only the conclusions below remain. Use `PROFILE_TESTS=1` for capture under the canonical `runs/profiles`, then `scripts/dispose-profiles.sh <run> "concise findings"`.

All Go test targets now capture CPU and heap profiles, retain binaries/logs and generate cumulative CPU, allocated-byte and allocated-object reports. Python checks are unchanged. Profiles are diagnostic evidence; every run also needs an engineering review.

## Running tests

```sh
make test-repro
make test-core
make race
PROFILE_MEM_RATE=1 scripts/test-profile.sh ./benchmarks/core -- -run '^$' -bench BenchmarkGoID -benchtime=10000x
```

`PROFILE_ROOT` now defaults to the canonical project `runs/profiles` directory. Each run has its own directory, an invocation record, package list and status table. Packages run separately because Go cannot write independent profiles for multiple packages in one invocation. No test-result cache is used. Coverage merges per-package coverage files. CI retains profile artifacts, including failed runs, under the existing retention policy.

The runner analyses profiles after failures too. Missing profiles cause a nonzero result. Build-only packages and benchmark-only packages in a full test run are reported separately from test execution; a focused pattern with no matches fails. Short tests can produce a valid CPU profile with zero samples; use a longer matching workload for CPU attribution. Heap sampling defaults to 512 KiB; use rate 1 for focused allocation attribution, not latency comparisons.

Nested external Go consumer tests also use the runner. Python tooling, native-library allocations and child CLI processes are not measured by these Go test profiles. This change does not add Python profiling or modify third-party code.

## First full run

Baseline: `a9d1cc6c`, Go 1.26.5, Linux amd64, Intel Core i7-12700. Environment and raw evidence are retained in `.cache/profiling-pass-20261005/`; all package profiles are in the recorded run directories.

The baseline ran all 57 packages and analysed profiles for every package with test files. The first runner incorrectly rejected `std/html`, which contains benchmarks but no test functions. Its profile was present; no assertion failed. The runner now distinguishes this legitimate full-suite case from an empty focused selection. The subsequent full run passed. The first pretag attempt then rejected the new lookup benchmarks under `core/runtime`; they were moved to `benchmarks/core` as required. The corrected profiled full pretag/browser gate and race suite passed. Failed gate logs and profiles remain retained.

Core allocated-byte samples attributed about 101 MB (31.7%) to the escaping 64-byte stack-header buffer in `core/runtime.GoID`. CPU attribution was more severe: `runtime.Stack` through interpreter-state lookup accounted for about 95% of core CPU samples. Per-goroutine lookup becomes active when worker runtime states exist. That CPU result is specific to this suite's concurrency/state lifecycle, not a typical application speed claim.

## Allocation change

A private `sync.Pool` now supplies the stack-header buffer. Each call checks out a distinct buffer and returns it after parsing, preserving IDs and state lookup rules without undocumented runtime offsets or unsafe goroutine pointers.

Six matched focused samples with allocation sampling rate 1 showed:

| Workload | Baseline | Candidate |
| --- | --- | --- |
| GoID | 64 B/op, 1 alloc/op | 0 B/op, 0 allocs/op |
| Registered interpreter-state lookup | 64 B/op, 1 alloc/op | 0 B/op, 0 allocs/op |

Concurrent ID stability/uniqueness and focused race checks passed. A discarded first benchmark registered the pool's main goroutine and incorrectly expected a worker state; that test mistake and its profiles are retained. The corrected benchmark creates the pool on a separate goroutine.

Core full-suite allocated-byte samples fell from 318.98 MB to 220.29 MB; allocated-object samples fell from 6.32 million to 4.70 million. Sampling and concurrency change whole-suite totals, so the focused per-operation measurements establish the reduction. Instrumented benchmark latency improved, but no uninstrumented speedup is claimed. Pool reuse is not a hard guarantee of zero allocations after GC: the runtime may clear pooled buffers.

## Profile review and next candidates

* Core still spends roughly 96% of CPU samples in runtime-state lookup/stack extraction after the change. Pooling removes allocation, not stack formatting or runtime synchronization. Replacing this mechanism needs an explicit state-passing/runtime ownership design and concurrency/fallback tests; a fragile runtime-offset trick is not justified.
* Vector construction and IR execution now lead core allocations (`BuildVector` about 58 MB, `irExec` about 34.5 MB flat in the candidate sample). `BuildVector` deliberately copies temporary IR input; removing that copy would alias mutable executor memory. Keep it until a measured ownership-transfer design proves collection immutability.
* String tests allocate about 193 MB, of which 186.5 MB (96.5%) is the differential-test fixture construction. Rewriting that fixture would improve test totals without reducing interpreter allocations. Preserve its independent/reference coverage. Collection profiles are about 2.35 MB and largely profiling/runtime overhead; no collection change is justified by that sample.
* Notebook profiles allocate about 60 MB, led by `io.ReadAll` (32.8 MB) and byte/string copying (16 MB). These include serving embedded assets and test scaffolding. A repeated notebook/request workload can determine whether static-asset caching or bounded copy reduction would benefit users before changing resource ownership.
* CLI, HTTP, PDF and integration packages allocate roughly 5.7–6.9 MB each, with compression/profiling buffers, bootstrap and runtime allocations prominent. No third-party optimization was attempted. Profiles of the parent Go test do not cover child-process allocations.
* The benchmark-only HTML package does not establish throughput in a full test run. Use a dedicated profiled benchmark and a matched uninstrumented measurement before making performance claims.

No release or push is part of this pass. Curl/SDL ABI qualification resumes using the same profiling rule for its Go tests.
