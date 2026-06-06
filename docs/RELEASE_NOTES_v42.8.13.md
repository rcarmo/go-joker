# Release Notes — v42.8.13

## Version

- Bumped runtime version from `v42.8.12` to `v42.8.13`.
- `joker --version` now reports `v42.8.13`.

## Highlights

- Reorganized runnable examples into purpose-specific folders and renamed the former Sushy static-site sample to the neutral wiki/static example.
- Added guardrails for examples, stale documentation paths, API stability classifications, release hygiene, runtime contracts, and docs-check coverage.
- Added `docs/START_HERE.md` as a concise onboarding guide for new contributors.
- Added bounded fuzz smoke targets for EDN and Transit decode inputs, complementing the existing HTTP request-map fuzz target.
- Added named WASM regression tests for recent bridge fixes around value-producing `if`, function-level loop initialization, and eligibility fallback diagnostics.
- Hardened notebook HTTP evaluation by reporting request-body read failures as `400 Bad Request` instead of silently evaluating stale source.
- Made hardening/test guard temp files repo-local and removed workspace-specific temp paths from compatibility tests.
- Aligned the tag-triggered release workflow with CI/docs expectations by running a broader package test set plus `make docs-check`.

## Validation

Validated during the release-prep series with:

```bash
make docs-check
go test ./core ./std/... ./cmd/joker ./internal/notebook ./tests -timeout 10m -count=1
go test ./std/edn -run '^$' -fuzz=FuzzEDNDecodeAll -fuzztime=2s
go test ./std/transit -run '^$' -fuzz=FuzzTransitDecodeValue -fuzztime=2s
git diff --check
```

`make release-hygiene-check` passes for `v42.8.13` before tagging; it may warn that `HEAD` is ahead of the tag until the tag is created.
