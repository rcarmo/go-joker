# Native integer arity regression

## Baseline and scope

Tested at `dc1c29d0` on Linux/amd64, Go 1.26.5, Intel Core i7-12700, six visible CPUs. Builds used default Go flags with repository-local TMPDIR/GOTMPDIR. The existing architecture commit was preserved. No historical benchmark data or release metadata was changed.

`make test-short benchmark-docs-check` passed before the change.

## Reproduction and first divergence

Minimal example: `(do (defn f [x] (inc x)) (f 1 2))` incorrectly returns 2. Calling `(f)` instead panics with a Go bounds error rather than Joker's arity error.

`Fn.Call` attempts native integer dispatch before its normal arity checks. `callNativeRecursive` indexed the expected argument positions without first checking the supplied count. All supported native arities (one, two and three) could therefore ignore extra arguments or index missing arguments.

The regression constructs the same parsed function body and lexical environment for both paths. It requires successful native compilation, directly executes the resulting native entry for valid inputs, and compares invalid-call panic types and messages against an interpreter-only Fn without `defVar`. These straight-line bodies have no recursive or tail-call route into IR/WASM. The independent contract is Fn.Call's existing `PanicArityMinMax` path, not assumed Clojure/JVM behavior.

Before the patch, all three arities show both discrepancies. After the patch, the same test passes. The change declines native dispatch only for incorrect argument counts, allowing the established language-level error path to run. Valid integer calls still execute the compiled entry; this is not a general fallback workaround.

## Validation

- `go test ./core -run TestNativeIntegerArityMatchesInterpreter -count=1`: fails before, passes after.
- `make test-short runtime-contract-check examples-check notebook-check`: passes, including execution of rich-demo and complex-demo notebooks.
- `go test ./...`: passes.
- Focused race coverage for the new regression, typed executor tests and recursive Fibonacci regression: passes.
- `git diff --check`: passes.

## Bounded profiling, not a speedup claim

A one-second CPU profile of the portable `BenchmarkCLBGBinaryTrees` in `./benchmarks/core` showed boxed `core.irExec` at 28.68% flat CPU samples, with allocation and frame handling also significant. The instrumented run reported approximately 77.6 ms/op, 38 MB/op and 1.27 million allocations/op. These are diagnostic observations, not baseline/candidate performance results. An initial invocation against `./core` matched no benchmark and was discarded.

No performance change is justified by this sample alone. No charts were refreshed, and no native-helper or WASM attribution is made for this workload. A future optimisation needs uninstrumented repeated baseline/candidate samples and the documented regression/statistical checks.

## Limitations

This proves the native integer arity boundary, not semantic equivalence of all tiers. Overflow, mutable Var bindings, evaluation order and fallback transitions remain separate investigation targets. Existing IR/WASM tests and notebooks provide collateral coverage, not differential proof for these unsupported invalid-arity cases. No non-amd64 platform tests were run. Automatic delegated review was unavailable because the current model had no matching ordered delegate policy; no independent model-review claim is made.
