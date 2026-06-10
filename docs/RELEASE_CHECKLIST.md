# Release checklist

Use this checklist before tagging a patch release.

## Version and notes

- [ ] Decide whether current `master` user-visible changes require a patch release.
- [ ] Update `core/runtime/version.go`.
- [ ] Update the README version sentence and release-notes link.
- [ ] Add `docs/RELEASE_NOTES_<version>.md`.
- [ ] Run `make release-hygiene-check`.
- [ ] Make sure release notes describe the tag being created, not an ambiguous moving `master` state.

## Validation

Recommended minimum before tagging:

```bash
make pretag-check
```

This runs release hygiene, whitespace checks for pending diffs, the CI/release package vet and test scope, and `make docs-check`. By default it skips the Playwright browser smoke because that requires local browser dependencies; include it when those dependencies are installed with:

```bash
PRETAG_BROWSER_SMOKE=1 make pretag-check
```

The tag-triggered release workflow also runs the broader package test set plus `make docs-check`, so failures in examples, docs paths, release hygiene, runtime contracts, or std native-boundary checks should be fixed before tagging.

For a broader pre-release audit:

```bash
make audit-fast
make race
make bench-sanity
```

## Tagging

```bash
git tag -a vX.Y.Z -m "vX.Y.Z: short summary"
git push
git push origin vX.Y.Z
```

## Post-tag checks

- [ ] Confirm GitHub Actions release workflow completed.
- [ ] Confirm release binaries are attached.
- [ ] Confirm `joker --version` reports the tagged version.
- [ ] Avoid editing the tagged release note with post-tag changes unless clearly marked as post-tag or moved into the next release notes file.
