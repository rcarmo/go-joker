# Release Notes -- v42.11.2

This patch release accelerates ASCII classification and reduces whitespace-token vector allocations, with scalar fallbacks retained on every supported architecture.

## Changes

* Added runtime-dispatched AVX2 scanning on amd64 and NEON scanning on arm64. CPU feature checks protect baseline machines; `-tags purego` forces the portable scalar implementation.
* SIMD kernels use bounded vector loads and scalar tails. Differential, guard-page, Unicode, invalid UTF-8, cache and concurrency tests cover the classification and public string APIs.
* Preallocated whitespace output vectors from the known token count, avoiding repeated growth without changing tokenisation.
* Fixed the benchmark regression checker to parse throughput rows and reject comparisons containing no parsed samples.
* Updated repository development criteria for correctness, profiling, allocations, SIMD and full regression validation.

## Measurements

On the audited amd64 runner, word frequency improved from 323.1 to 295.1 us/op (-8.65%), with 22.66% fewer bytes/op. Cold ASCII classification showed 20–67% lower median latency; smaller cases were noisy and are not general application-speed claims.

Native Orange Pi ARM64 record batches improved from 548.9 to 244.9 us/batch (-55.39%) using ten matched fresh-process samples pinned to one Cortex-A720. Both modes recorded 302 allocations/batch in every sample; the unchanged regression policy passed. Earlier failed cache-inclusive allocation comparisons are retained in the audit report rather than discarded.

See [SIMD and allocation audit](SIMD_ALLOCATION_AUDIT_2026-09-21.md) for scope, methodology and limitations. Historical charts were not refreshed.

## Validation

The audit passed the full pre-tag gate with Chromium browser smoke, repository race target, full purego test suite, Linux/386 core/string tests and cross-builds for all six release platforms plus Linux/386. Native ARM64 validation covered NEON selection, scalar comparisons, guard pages, concurrency, race checks and public Unicode semantics. Approximately 1.47 million bounded ASCII fuzz cases passed; an independent static kernel review found no concrete correctness defect in the reviewed files.

The existing ASCII classification cache retention policy is unchanged. This bounded audit does not establish equivalence for every interpreter execution path.
