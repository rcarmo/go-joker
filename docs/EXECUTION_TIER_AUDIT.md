## Execution-tier audit, September 2026

The audit began with the native integer arity defect and expanded into numeric representation, control flow, Var replacement, captures and error recovery. Comparing optimised execution with an ordinary function call was not enough: that call can itself enter another optimised path. The numeric primitives and existing promotion tests provided a separate contract for disputed arithmetic results.

The bounded audit and remediation pass is complete for the tested matrix below. This is not a proof that all execution tiers are equivalent, nor an independent release approval. Residual risks and excluded work remain explicit below.

## Corrected behaviour

| Boundary | Regression coverage |
| --- | --- |
| Native integer calls | Wrong arity, core Var replacement, first use inside `with-redefs`, restoration, arithmetic overflow, intermediate promotion and recursive promotion |
| Boxed and typed IR | In-place arithmetic promotion, promoted operands on either side, promoted comparisons, exact division and remainder, zero-divisor errors |
| Inline typed executor | Promotion and exact division/remainder without converting unsupported numeric results into nil |
| NaN-boxed executor | Wide integer round trips, crossing the signed 32-bit payload boundary, object-table arithmetic, NaN tag collisions and equality |
| Captures and collections | Retained closures preserve separate captures across repeated IR execution; mixed vectors survive typed-vector conversion fallback |
| Integer WASM | Loop exit after `recur`; checked arithmetic traps; promoted recovery for import-free arithmetic traps |
| Float-mode WASM inputs | Integer-containing calls use IR before execution; double-only direct execution is tested. This is an explicit backend restriction until per-value typing exists. |
| Error recovery | Zero-divisor errors, explicit exceptions, callable-origin type errors and errors after callable entry do not replay prior calls. Unsupported typed `zero?` and late `try/catch` both reproduced callback replay; `zero?` now executes and unsupported `try/catch` is rejected before execution. |

Tests live in `core/runtime_kernel_contracts_test.go` and `core/ir/nanbox_test.go`. They require compilation or direct executor invocation where a tier is claimed. Unsupported direct-executor shapes are not counted as successful tier coverage.

Native overflow recovery re-evaluates only the compiler's restricted arithmetic expression tree. It cannot invoke arbitrary handlers or repeat I/O. General IR promotes at the current instruction instead of restarting the loop. WASM recovery is restricted to import-free modules and arithmetic traps; parameter-shape failures must not be replayed with incomplete state.

## Measurements, kept separate from historical charts

Runs used Linux amd64, Go 1.26.5 and an Intel Core i7-12700 with six visible CPUs. Builds used default Go flags and repository-local temporary directories. Instrumented profiles were not used as timings.

| Comparison | Samples | Time | Bytes/op | Allocs/op |
| --- | --- | --- | --- | --- |
| Checked small integer multiplication | 10 x 500 ms each | 45.92 to 20.29 ns | 64 to 16 | 3 to 2 |
| Portable Binary Trees, IR promotion candidate | 10 samples x 10 iterations each | 76.32 to 76.98 ms, not significant | No significant difference | No significant difference |
| Fibonacci 20, pre-audit release versus audit candidate | 10 x 500 ms each | 1.550 to 1.554 ms, not significant | 578 to 578 | 7 to 7 |
| Tak, same comparison | 10 x 500 ms each | 4.429 to 4.291 ms | No significant difference | 6 to 6 |
| Invoice totals, only multiplication changed | 10 samples x 20 iterations each | 2.332 to 2.372 ms, not significant | 283.6 to 264.8 KiB | 10,221 to 9,821 |

Each listed comparison passed the repository regression policy. The Fibonacci/Tak comparison includes all intervening audit changes, so it does not isolate the guard cost or justify attributing the small Tak improvement to one patch. The primitive multiplication result is not a whole-program speedup. The invoice workload separately validates checksum 470,400 and shows 6.6% fewer bytes and 3.9% fewer allocations, without a significant timing change. An earlier generic numeric IR prototype increased Binary Trees allocations by 43% and was rejected.

The live Joker `pidigits` fixture was incorrect: it used `/`, while the Python and JavaScript references used integer quotient. Its old expected value was a large floating-point result rather than a digit checksum. The corrected fixture uses arbitrary-precision integers and `quot`; its 27-digit checksum is 129, independently matched against the JavaScript reference. Measurements of the corrected fixture must not be compared directly with the old fixture. Historical charts and their source data were not overwritten.

## Validation performed

Isolated correctness changes passed `make pretag-check`, including repository-wide tests, vet, generated/documentation guards, example smoke tests and notebook checks. The final focused race sweep covered all TestAudit regressions, native arithmetic/rebinding, captures, transients and NaN-boxing and passed. The final pre-tag gate, focused race sweep, Linux/386 full core/IR/types/collections suite and arithmetic metadata check passed after the callback-result matrix was added. That matrix checks list access/count, vector append, map lookup and promoted equality after exactly one callback invocation. Browser smoke is optional in the local gate and was not included in these audit runs.

## Residual risks and excluded work

* Float-mode WASM lacks per-slot numeric types. Calls containing integer slots now take IR before execution, which avoids the demonstrated precision losses at a performance cost for mixed numeric workloads. Float modules with integer-valued internal constants still need broader type-flow analysis.
* General unsupported-result fallback is not proven side-effect-safe for every opcode. Callable-origin errors and failures after callable entry propagate; missing typed `zero?`, sequential `nth` and late `try/catch` support were addressed after their unsupported results repeated callbacks. The remaining speculative Number/Fn mismatch in Binary Trees needs diagnosis rather than a blanket removal of fallback.
* Public-call promotion tests pass, and the five arithmetic docstrings now agree in source, bootstrap metadata and generated HTML. `bun tools/codegen/sync-arithmetic-docs.ts --check` verifies this narrow synchronisation. Full bootstrap regeneration still fails on extracted generic collection types/private fields; the investigated repair is retained separately and is not claimed complete.
* Linux/386 full tests now pass for `core`, `core/ir`, `core/types` and `core/types/collections`. Fixes use fixed-width hash-trie bitmaps, width-independent WASM handles, architecture-aware promotion and test expectations. ARM and Windows execution checks were not run.
* Bounded fuzz runs passed approximately 1.74 million numeric boxing cases and 1.32 million multiplication cases on amd64, plus 2.35 million multiplication cases on 386 (without coverage guidance). This is not exhaustive input coverage.
* There has been no independent model review. Earlier delegation attempts were blocked by approval policy; the final attempt found azure-openai/gpt-5-4 available in Piclaw but not executable by its child CLI. A final automatic attempt was also blocked by the current model's missing ordered delegation rule. Independent sign-off remains pending.

No release was published as part of this audit. Closure covers the tested matrix, measured optimisation and documented limitations; it does not close the residual risks or substitute for independent review.

## Current operator-facing contract

Public ordinary integer arithmetic promotes on overflow. Integer division/remainder preserve numeric primitive types and zero-divisor failures. Direct tests cover native, boxed, typed, inline and NaN-boxed entry points, with backend restrictions where a representation cannot preserve those semantics. Source, bootstrap and generated HTML arithmetic docstrings match the verified promotion contract; full bootstrap generator repair is separate remaining work.

The audit produced concrete fixes and measured allocation improvements, but it did not prove equivalence for every Joker program. General generator repair, untested platforms and unproven speculative-execution cases remain follow-up work rather than completed fixes.

## Closing validation

The final current-tree sweep after the vector-bounds fixes passed `make pretag-check`, the focused race suite covering audit/native/numeric/capture/transient regressions, Linux/386 full core/IR/types/collections tests, and the arithmetic metadata synchronisation check. The expanded callback matrix includes non-associative `get`, sequential `nth`, vector bounds, collection updates and transient error behaviour. Bounds errors propagate without replaying earlier callbacks; transient `Nth` now matches persistent vectors while `TryNth` retains its default-value behaviour.

All implemented fixes are in reviewable commits. The audit itself did not publish a release or refresh historical charts. The subsequent operator-authorised v42.11.0 release includes these changes and a separate [fresh benchmark refresh](BENCHMARK_RESULTS_2026-09-06.md), retaining the prior snapshot. Independent model review remains unavailable under the configured delegation policy; the residual risks above remain applicable.
