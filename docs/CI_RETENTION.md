# CI retention and cleanup policy

GitHub Actions retention is managed in one place: `.github/workflows/actions-cleanup.yml`. It runs weekly and can also be dispatched manually.

## Retention windows

| Resource | Policy |
| --- | --- |
| CI benchmark results | 7 days; also limited to artifacts belonging to the ten newest runs of each workflow. |
| Release build staging artifacts | 7 days; published GitHub Release assets are separate and are not removed by Actions cleanup. |
| Benchmark comparison evidence | 30 days; also limited to artifacts belonging to the ten newest benchmark workflow runs. |
| Other Actions artifacts | 7 days and the ten-newest-runs window. |
| Actions caches | Seven days since last access and at most the 100 most recently used caches. |
| Completed workflow runs/logs | 30 days, while preserving at least the ten newest completed runs per workflow. |
| GitHub Release binaries, SBOMs, checksums, and attestations | Not managed by the Actions cleanup workflow. They follow GitHub Release and attestation retention. |

The artifact upload steps declare the same seven- or 30-day window as the cleanup workflow. Cleanup uses the artifact name `benchmark-comparison` to select the longer evidence window; all other transient artifacts use the standard window.

## Failure and concurrency behavior

Only one cleanup run operates at a time (`actions-cleanup`, without cancellation). Deletions treat an already absent resource as success, retry transient server failures up to three times, and report unsuccessful rate-limited or non-retryable deletions as warnings. The next weekly run or a manual dispatch retries remaining stale resources.

The workflow never deletes its currently running cleanup execution. Workflow-run pruning requires both age beyond 30 days and exclusion from the ten-newest-runs set.

## Validation

`actionlint` runs in the main CI workflow before build and test work. Local validation is available with:

```bash
make workflow-lint
make workflow-policy-check
```

`workflow-policy-check` is part of the canonical `make release-check`; it guards the weekly and manual cleanup triggers, matching retention declarations, least-privilege cleanup permissions, and the actionlint CI step. `workflow-lint` runs the pinned actionlint release directly and may download it through the Go toolchain on first use.
