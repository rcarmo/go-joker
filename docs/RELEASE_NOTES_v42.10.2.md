# Release Notes -- v42.10.2

This patch release corrects argument-count handling in native integer function dispatch.

## Correctness

* Native integer closures now reject invalid argument counts through Joker's existing language-level arity error path. Previously, too few arguments could cause a Go bounds panic, while extra arguments could be silently ignored.
* Added differential regression coverage for all supported native integer arities (one, two and three), with explicit native compilation/execution checks and interpreter exception comparisons.
* Valid integer calls retain native execution.

## Documentation

* Updated the architecture diagram to include the native integer execution tier.
* Recorded reproduction, root cause, before/after test evidence, profiling scope and limitations in `docs/NATIVE_INTEGER_ARITY_REGRESSION.md`.
* Historical benchmark charts remain unchanged. The diagnostic profile is not a speedup claim or a cross-tier comparison.

## Validation

The correctness change passed the full Go test suite, focused race tests, runtime contracts, example smoke tests, and representative notebook execution. Release validation uses the canonical `make pretag-check` gate, with browser smoke additionally run by the release workflow.
