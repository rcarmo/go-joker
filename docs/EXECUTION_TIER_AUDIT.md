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
| Float-mode WASM inputs | Wide integer inputs avoid lossy conversion; all-integer inputs use IR when float constants would otherwise coerce the entire computation |
| Numeric error recovery | A compiled loop that mutates an atom before dividing by zero mutates it once, not three times through fallback retries |

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

* Float-mode WASM still lacks per-slot numeric types. Mixed integer/double inputs and integer intermediates inside a floating computation need a complete contract; the input guards fix specific demonstrated losses, not every mixed expression.
* General executor error classification is incomplete. Broadly rethrowing all failures exposed an existing Binary Trees speculative Number/Fn mismatch; only the zero-divisor replay case is fixed. Other failures can still be confused with unsupported-execution signals.
* Core numeric docstrings still describe ordinary arithmetic as wrapping, while the numeric primitives and promotion tests require `BigInt`. Those source/generated contracts need reconciliation, including all optimised entry points.
* No 32-bit, ARM or Windows execution checks were run. NaN-boxed arithmetic uses machine-sized Go operations after decoding 32-bit payloads, so 32-bit behaviour needs explicit validation.
* There has been no independent model review: attempted delegation was blocked by the configured provider/model approval policy. Fuzz/property generation and a complete exception/transient matrix have not been completed.

No release was published as part of this audit. The open items need implementation and validation before the plan can be marked complete.
