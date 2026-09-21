# SIMD and allocation audit: 2026-09-21

The word-frequency profile identified allocation growth before SIMD work began. Token splitting accounted for about 31% of allocated bytes, vector growth another 31%, and sequence traversal 25%. Preallocating the result vector from the already-known token count removes repeated growth without changing tokenisation.

The second change accelerates ASCII classification on cache misses. AMD64 selects AVX2 only when `golang.org/x/sys/cpu` reports support; ARM64 selects NEON when ASIMD is available. Other architectures and `-tags purego` use the scalar implementation. Small strings retain the existing inline scalar path. The classification cache and its retention policy are unchanged.

## Measurements

Host: Intel Core i7-12700, six visible CPUs, Linux amd64, Go 1.26.5, GOAMD64=v1. CPU profiles were separate from timing runs. Baseline and candidate ran sequentially on the same machine with matched workloads. No historical charts were changed.

| Workload | Baseline | Candidate | Allocation result |
| --- | --- | --- | --- |
| Word frequency, ten 500ms samples | 323.1 us/op | 295.1 us/op (-8.65%) | 751.8 to 581.5 KiB/op (-22.66%); 8074 to 8060 allocs/op |
| Cold ASCII classification, 64-byte payload | 210.2 ns/op | 168.1 ns/op (-20.0%) | Two allocations/op on both sides |
| Cold ASCII classification, 1024-byte payload | 492.6 ns/op | 266.0 ns/op (-46.0%) | Two allocations/op on both sides |
| Cold ASCII classification, 16384-byte payload | 5.250 us/op | 1.733 us/op (-67.0%) | Two allocations/op on both sides |
| Public string-count batch, 128 fresh ASCII records | 247.2 us/batch | 110.8 us/batch (-55.2%) | No significant bytes/allocation difference |

ASCII classification used ten samples of 1000 unique inputs; record batches used ten samples of 20 iterations. The unique suffixes keep cache hits from substituting for scanning. Reported payload sizes exclude those suffixes. An earlier pointer-derived identifier allowed cache reuse; those samples were discarded.

The 64-byte, 1024-byte and record-batch timings were noisy under the repository CV rule. Benchstat found reductions, but they are not stable timing-gate claims or general application speedups. The 16KiB classification and word-frequency comparisons were stable. Allocation regression checks passed.

Benchmark validation itself had a correctness defect: rows containing MB/s were ignored, and two empty parsed result sets could pass. The parser now accepts throughput rows and rejects empty comparisons, with unit tests. All claimed comparisons were rerun through that corrected checker.

## Correctness and execution evidence

Both kernels read complete vector blocks only, followed by a bytewise tail. AVX2 uses 32-byte loads and a high-bit mask; NEON uses 16-byte loads, high-bit shifts and lane reduction. AMD64 clears upper vector state after YMM use. No input alignment requirement is imposed.

Tests cover offsets 0–31, lengths 0–257, every possible single high-byte position, inaccessible pages immediately after the input, Unicode, invalid UTF-8, NUL bytes, repeated cache hits and concurrent classification. Public `String.Count` and `Nth` agree with Go rune decoding. About 1.47 million bounded fuzz cases passed. Scalar-forced, AVX2-disabled and purego paths also passed.

A separate model statically reviewed assembly ABI offsets, feature dispatch, reductions and bounds. It found no concrete correctness defect in the reviewed kernels; it did not execute tests or evaluate workload performance.

## Regression validation

* Full `PRETAG_BROWSER_SMOKE=1 make pretag-check` passed, including repository-wide tests/vet, docs/generated guards, examples, notebooks and Chromium smoke.
* `make race` passed for core, strings, runtime, HTTP and PDF. Additional scanner/public-string race tests passed.
* Full `go test -tags purego ./...` passed.
* Linux/386 core and string tests passed. All six release platforms plus Linux/386 cross-built with CGO disabled.
* ARM64 tests executed under QEMU, including explicit NEON dispatch, guard pages, differential and concurrent tests. This is emulated correctness coverage, not native hardware performance validation.

## Native ARM64 validation

Sigma permission refresh requests timed out; no agent request was delivered. At Rui's direction, validation instead ran in an isolated directory on Orange Pi 6 Plus (`orangepi6plus`, CIX P1 CD8160, Cortex-A520/A720, Linux arm64, Go 1.26.5). Pre-run idle was 99–100%.

Native NEON dispatch, differential, guard-page, concurrent classification, scalar/purego, race and public Unicode string tests passed. Ten-sample cold-classification runs showed 49.9%, 53.1% and 67.9% lower median times for 64-byte, 1KiB and 16KiB payloads. Their allocation counts were unchanged and the regression policy passed, but timing CV was high.

The initial record batch improved 1012.1 to 333.9 us. A follow-up pinned to Cortex-A720 CPU 11 with GOMAXPROCS=1 and ten 100-iteration samples improved 579.5 to 274.8 us (-52.6%, stable timing). However, the strict allocation policy failed: median allocations increased from 307.5 to 308, despite no statistically significant difference (p=0.895). These initial results did not pass the regression policy and remain retained. Allocation profiles at sampling rate 1 locate the variation in Go's sync.Map internals: both modes allocated exactly 12,928 keys and 12,928 entry nodes, with slightly different internal map allocations. No scanner allocation was observed.

The closing protocol uses ten fresh processes per mode, alternating scalar/NEON, each pinned to CPU 11 with GOMAXPROCS=1 and 500 batches. This prevents cache accumulation across samples and improves per-operation allocation resolution. Scalar measured 548.9 us/batch versus NEON 244.9 us/batch (-55.39%, p=0.000, approximately 1% variation); every sample recorded 302 allocations/batch, with no significant bytes/op difference. The unchanged regression policy passed. Earlier failed comparisons were not discarded or relabelled as passing.

Direct `testing.AllocsPerRun` checks confirm zero allocations in both scalar and selected SIMD kernels on native amd64 and ARM64, for empty, long ASCII and non-ASCII inputs. The fresh-process workload comparison above resolves the acceptance question without waiving or weakening the earlier failed gates. The final pre-tag gate with browser smoke and full repository race target passed again after adding these checks.

Other limitations: this pass focuses on string scanning and the profiled allocation path, not a fresh proof of every execution tier. The pre-existing unbounded ASCII classification cache remains unchanged. No SSE2 intermediate kernel is provided on non-AVX2 amd64; scalar is the safe fallback. The existing general bootstrap generator limitations are outside this patch.

No release, push or chart refresh is part of this audit. Native ARM64 correctness and the controlled scalar/NEON workload comparison are verified on Orange Pi. This completes the bounded string/SIMD and allocation audit; it does not claim a new proof of equivalence for all interpreter tiers.
