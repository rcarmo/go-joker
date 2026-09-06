## Execution-tier audit, September 2026

The audit began with the native integer arity defect and expanded into numeric representation, control flow, Var replacement, captures and error recovery. Comparing optimised execution with an ordinary function call was not enough: that call can itself enter another optimised path. The numeric primitives and existing promotion tests provided a separate contract for disputed arithmetic results.

This is a progress report, not a claim that all execution tiers are equivalent. The remaining problems below prevent full sign-off.

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
| Error recovery | Zero-divisor errors, explicit exceptions, callable-origin type errors and errors after callable entry do not replay prior calls. The unsupported typed `zero?` case also has a regression. |

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

Each listed comparison passed the repository regression policy. The Fibonacci/Tak comparison includes all intervening audit changes, so it does not isolate the guard cost or justify attributing the small Tak improvement to one patch. The multiplication result is a primitive microbenchmark, not a whole-program speedup. An earlier generic numeric IR prototype increased Binary Trees allocations by 43% and was rejected.

The live Joker `pidigits` fixture was incorrect: it used `/`, while the Python and JavaScript references used integer quotient. Its old expected value was a large floating-point result rather than a digit checksum. The corrected fixture uses arbitrary-precision integers and `quot`; its 27-digit checksum is 129, independently matched against the JavaScript reference. Measurements of the corrected fixture must not be compared directly with the old fixture. Historical charts and their source data were not overwritten.

## Validation performed

Isolated correctness changes passed `make pretag-check`, including repository-wide tests, vet, generated/documentation guards, example smoke tests and notebook checks. Focused race tests covered native arithmetic/rebinding, IR numeric behaviour, WASM boundaries and NaN-boxing. Browser smoke is optional in the local gate and was not included in these audit runs.

## Remaining work

* Float-mode WASM lacks per-slot numeric types. Calls containing integer slots now take IR before execution, which avoids the demonstrated precision losses at a performance cost for mixed numeric workloads. Float modules with integer-valued internal constants still need broader type-flow analysis.
* General unsupported-result fallback is not proven side-effect-safe for every opcode. Callable-origin errors and failures after callable entry propagate; a missing typed `zero?` handler was fixed after its unsupported result repeated a callback. The remaining speculative Number/Fn mismatch in Binary Trees needs diagnosis rather than a blanket removal of fallback.
* Public-call promotion tests now pass, but source and generated numeric docstrings still say ordinary arithmetic wraps. Regeneration is blocked by bootstrap type aliases, optimisation during source loading, nil reflection metadata and malformed generated identifiers for extracted packages. The investigation patch is retained separately; committed generated files were restored and build successfully.
* Linux/386 tests passed for `core/ir` and `core/types`. Full `core` compilation on 386 is blocked by an out-of-range generated bitmap literal and a 64-bit handle constant stored in `int`. ARM and Windows execution checks were not run.
* Bounded fuzz runs passed approximately 1.74 million numeric boxing cases and 1.32 million multiplication cases on amd64, plus 2.35 million multiplication cases on 386 (without coverage guidance). This is not exhaustive input coverage.
* There has been no independent model review: attempted delegation was blocked by the configured provider/model approval policy. The complete exception/transient matrix and final sign-off remain pending.

No release was published as part of this audit. The open items need implementation and validation before the plan can be marked complete.
