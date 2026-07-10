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

The canonical release gate used locally and by both GitHub workflows is:

```bash
make release-check
```

Before tagging, run its `pretag-check` wrapper:

```bash
make pretag-check
```

`release-check` runs release hygiene, whitespace checks for pending diffs, repository-wide vet and tests, and `make docs-check`. `pretag-check` runs that exact gate and optionally adds the Playwright browser smoke. By default it skips the browser smoke because that requires local browser dependencies; include it when those dependencies are installed with:

```bash
PRETAG_BROWSER_SMOKE=1 make pretag-check
```

The CI and tag-triggered release workflows call `make release-check` directly, so the non-browser release gate cannot drift from local validation. Both workflows run the browser smoke separately after installing its dependencies.

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
