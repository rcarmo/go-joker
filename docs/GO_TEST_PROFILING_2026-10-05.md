# Go test profiling and first allocation pass

All Go test targets now capture CPU and heap profiles, retain binaries/logs and generate cumulative CPU, allocated-byte and allocated-object reports. Python checks are unchanged. Profiles are diagnostic evidence; every run also needs an engineering review.

## Running tests

```sh
make test-repro
make test-core
make race
PROFILE_MEM_RATE=1 scripts/test-profile.sh ./core/runtime -- -run 'TestGoID' -bench BenchmarkGoID -benchtime=10000x
```

`PROFILE_ROOT` defaults to `.cache/test-profiles`. Each run has its own directory, an invocation record, package list and status table. Packages run separately because Go cannot write independent profiles for multiple packages in one invocation. No test-result cache is used. Coverage merges per-package coverage files. CI retains profile artifacts, including failed runs, under the existing retention policy.

The runner analyses profiles after failures too. Missing profiles cause a nonzero result. Build-only packages and benchmark-only packages in a full test run are reported separately from test execution; a focused pattern with no matches fails. Short tests can produce a valid CPU profile with zero samples; use a longer matching workload for CPU attribution. Heap sampling defaults to 512 KiB; use rate 1 for focused allocation attribution, not latency comparisons.

Nested external Go consumer tests also use the runner. Python tooling, native-library allocations and child CLI processes are not measured by these Go test profiles. This change does not add Python profiling or modify third-party code.

## First full run

Baseline: `a9d1cc6c`, Go 1.26.5, Linux amd64, Intel Core i7-12700. Environment and raw evidence are retained in `.cache/profiling-pass-20261005/`; all package profiles are in the recorded run directories.

The baseline ran all 57 packages and analysed profiles for every package with test files. The first runner incorrectly rejected `std/html`, which contains benchmarks but no test functions. Its profile was present; no assertion failed. The runner now distinguishes this legitimate full-suite case from an empty focused selection. The subsequent full run passed, as did the profiled full pretag/browser gate and race suite.

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
* Reader/type/collection and standard-library test packages were reviewed separately in the retained top tables. Many short-package profiles are dominated by bootstrap, profiling buffers, compression, scheduler or GC allocations. Changing those tests or dependencies to improve profiling totals would obscure application behaviour; no safe runtime change was inferred from those small samples.
* CLI/notebook/HTTP/integration profiles mix startup, parsing, JSON, generated metadata and test scaffolding. Optimise a repeatable script or request benchmark before changing these paths. Profiles of the parent Go test do not cover child-process allocations.
* The benchmark-only HTML package does not establish throughput in a full test run. Use a dedicated profiled benchmark and a matched uninstrumented measurement before making performance claims.

No release or push is part of this pass. Curl/SDL ABI qualification resumes using the same profiling rule for its Go tests.
