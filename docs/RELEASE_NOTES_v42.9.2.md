# Release Notes — v42.9.2

This patch release completes a release-grade correctness, concurrency, security, dependency, and tooling audit.

## Runtime correctness and concurrency

- Synchronized atom reads and updates, while keeping user callbacks outside atom locks to prevent callback-induced deadlocks.
- Made delay realization safe under concurrent `force` calls and retryable after a panic.
- Serialized function IR compilation and safely published cached programs.
- Protected mutable IR analysis, execution-failure, and native-helper state.
- Corrected parallel-worker completion so cleanup callbacks finish before return and cleanup panics are recovered.

## Security and resource handling

- Constrained remote dependency cache paths to a hashed cache namespace, rejected traversal, bounded downloads to 32 MiB, and committed downloads atomically.
- Limited notebook HTTP request bodies to 16 MiB and returns HTTP 413 for oversized requests.
- Reaped detached child processes instead of releasing them without waiting.
- Hardened environment parsing against malformed entries.

## Dependencies and toolchain

- Updated the release toolchain to Go 1.26.5.
- Updated `wazero` to v1.12.0 and current `golang.org/x` dependencies.
- `govulncheck ./...` reports no reachable vulnerabilities.

## CI and release hygiene

- Expanded CI tests and vet to `./...`.
- Made the benchmark driver buildable under repository-wide build checks.
- Added release-tag/runtime-version verification and removed an ineffective linker version override.
- Strengthened legacy module-import detection.
- Added regression coverage for atom updates, delay realization, parallel cleanup ordering, dependency cache traversal, and notebook body limits.

## Validation

The release passed:

- `go test ./... -count=1`
- `go vet ./...`
- `go build ./...`
- `staticcheck -checks=SA* ./...`
- `govulncheck ./...`
- `go test -race ./core ./std/runtime ./std/http ./std/pdf -count=1`
- release hygiene, generated-source, import-identity, and error-handling guards
