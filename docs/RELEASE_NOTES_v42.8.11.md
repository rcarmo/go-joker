# Release Notes — v42.8.11

## Version

- Bumped runtime version from `v42.8.10` to `v42.8.11`.
- `joker --version` now reports `v42.8.11`.

## Included fixes

This patch level includes the recent runtime startup, `joker.term`, and Makefile usability fixes already committed on `master`.

## Verified

```bash
go test ./core/runtime -count=1
go build ./cmd/joker
```
