# Release Notes -- v42.11.0

This minor release brings the execution-tier correctness audit and a fresh, output-validated benchmark refresh together. It fixes observable differences between native integer closures, IR executors and WASM, while keeping remaining backend restrictions explicit.

## Execution correctness

* Ordinary integer arithmetic promotes overflow to `BigInt` across the tested public/native/IR paths. Native recursive recovery is restricted to pure arithmetic expressions; general IR promotes in place rather than replaying loops.
* Exact division, remainder and zero predicates preserve numeric types and error behaviour. NaN-boxing preserves wide integers and NaNs, including equality at object/numeric boundaries.
* Native and IR dispatch respect core Var replacement. Regression tests cover first use inside `with-redefs`, cached calls and restoration.
* Callback exceptions, numeric failures and several unsupported-operation paths no longer repeat prior mutations. Unsupported `try/catch` is rejected before IR execution. Sequence lookup, non-associative `get` and vector bounds follow the public contracts.
* Retained closures preserve their captures across executions. Mixed-vector conversion and transient bounds semantics are corrected.
* Integer WASM traps arithmetic overflow and recovers eligible import-free computations through promoted IR. The homogeneous float backend routes integer-containing calls to IR before execution; double-only WASM remains supported. This restriction favours correctness over mixed-workload acceleration.
* Fixed-width hash-trie bitmaps, WASM handle construction and word-size-aware arithmetic restore the tested Linux/386 core packages.

## Measurements and documentation

* Checked small-integer multiplication avoids unnecessary `big.Int` work: the primitive microbenchmark measured 45.92 to 20.29 ns/op, 64 to 16 bytes/op and three to two allocations/op. A validated invoice workload showed 6.6% fewer bytes and 3.9% fewer allocations, without a significant timing change.
* Refreshed 26 Joker benchmarks with five one-second samples each, all four comparison runtimes, and the mirrored let-go suite. The best-Joker table records nine of fifteen workload wins; these are selected portable/native-helper workloads, not a WASM-specific or general application claim.
* Corrected `pidigits` to arbitrary-precision integer quotient and checksum 129. Incompatible historical speedup comparisons are excluded, and the previous JSON/charts are archived.
* Updated benchmark charts, current documentation, arithmetic API metadata and HTML. Full results and raw samples are retained under `benchmarks/results/2026-09-06/`.

## Validation and limitations

The audited code passed the full pre-tag gate, focused race suites, Linux/386 core/IR/types/collections suites, and bounded numeric fuzz runs totalling about 5.4 million cases. Release CI additionally runs browser smoke tests and verifies six-platform binaries, SBOMs and checksums.

This is not proof of equivalence for every Joker program. General bootstrap regeneration still has extracted generic/private-field serialization limitations; a narrow checked generator synchronizes the arithmetic docstrings. Broader speculative fallback analysis, additional platform execution and independent model review remain limitations documented in `docs/EXECUTION_TIER_AUDIT.md`. Benchmark results include methodology and comparability caveats in `docs/BENCHMARK_RESULTS_2026-09-06.md`.
