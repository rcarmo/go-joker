# Release Audit Report — 2026-07-09

## Scope

A release-grade review of runtime correctness, concurrency, security boundaries, dependency health, error handling, tests, static analysis, CI coverage, and release hygiene for v42.9.2.

## Confirmed findings and remediation

### Concurrency and correctness

1. Atom value reads raced with mutation. Atom state is now synchronized.
2. Atom callbacks ran while holding the value lock, allowing validator or swap callbacks that dereference the atom to deadlock. Updates now evaluate callbacks outside the lock and atomically retry swaps when the value changes.
3. Atom validator and watch side tables were unsynchronized. Their state is now protected and watch callbacks run from a snapshot outside the lock.
4. Delay realization could execute repeatedly and race on its result. Realization is now serialized, published only after success, and retried after panic.
5. Function IR compilation and several IR program caches/flags raced. Compilation uses one-time publication; mutable program state uses locks or atomic flags.
6. Parallel worker cleanup ran after completion was reported, and cleanup panics escaped recovery. Cleanup now precedes completion and remains inside the recovery boundary.

### Security and resources

1. Remote dependency URLs and library paths could influence cache traversal. Cache namespaces are URL-hashed and final paths are containment-checked.
2. Dependency downloads were unbounded and written directly to final paths. Downloads are capped at 32 MiB and atomically renamed from temporary files.
3. Notebook mutation/evaluation bodies were unbounded. Requests are capped at 16 MiB and oversized bodies return HTTP 413.
4. Started child processes were released without reaping. A background waiter now reaps them.
5. Environment parsing assumed every entry contained `=`. Malformed entries are ignored safely.

### Tooling and release controls

- Removed two static-analysis defects: an empty branch and an ineffective loop `break`.
- Corrected a benchmark call that supplied a nil context.
- Expanded repository tests, vet, and build coverage to `./...`.
- Strengthened module-import identity checks without treating upstream URLs in comments as imports.
- Release workflow now verifies that the tag exactly matches the runtime version.
- Updated Go and vulnerable dependencies; no reachable vulnerabilities remain.

## Verification evidence

Passed on Go 1.26.5:

- `go test ./... -count=1`
- `go vet ./...`
- `go build ./...`
- `staticcheck -checks=SA* ./...`
- `govulncheck ./...` — 0 reachable vulnerabilities
- `go test -race ./core ./std/runtime ./std/http ./std/pdf -count=1`
- focused race tests for runtime atoms, delays, dependency resolution, and notebook handlers
- `tests/release_hygiene_guard.sh .`
- `tests/generated_guard.sh .`
- `tests/import_identity_guard.sh .`
- `tests/error_handling_guard.sh .`
- `git diff --check`

## Residual risk

- Atom validator replacement and atom value mutation are individually race-free but are not a single cross-object transaction; this matches the existing side-table architecture.
- The imaging module has no newer tagged release; current reachable-code analysis reports no vulnerability through imported call paths.
- Broader fuzzing of remote dependency responses and notebook endpoints would add defense-in-depth beyond the bounded regression tests.
