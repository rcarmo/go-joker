# Release Notes — v42.8.8

## Changes

### `joker.imaging`

- **WebP decode support** — `(imaging/open "photo.webp")` now works out of the box.
  Registered `golang.org/x/image/webp` decoder (pure Go, no cgo, zero new dependencies).
  WebP encode is not yet supported (would require cgo + libwebp).

## Verified

```bash
go test ./std/imaging ./core/runtime -run 'TestVersion|Test.*' -count=1
go build ./core/... ./std/... ./cmd/...
```
