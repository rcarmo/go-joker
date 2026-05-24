# go-joker v42.8.7 Release Notes

`v42.8.7` is a patch release focused on small `joker.imaging` API parity improvements after `v42.8.6`.

## Version

- Bumped runtime version from `v42.8.6` to `v42.8.7`.
- `joker --version` now reports `v42.8.7`.

## Imaging API

Added low-friction image metadata and terminal helpers inspired by Bun's Image runtime API while keeping Joker's eager functional style.

New functions:

```clojure
(imaging/metadata img)
(imaging/bytes img :png)
(imaging/base64 img :png)
(imaging/data-uri img :png)
```

`metadata` returns:

```clojure
{:width  ...
 :height ...
 :bounds [x y width height]
 :color-model :nrgba}
```

`bytes` is an explicit alias for the existing encoded-bytes behavior from `encode`. `base64` and `data-uri` make notebook/browser/agent workflows easier without hand-rolling base64 wrappers.

Supported encode formats remain:

```clojure
:png :jpeg :jpg :gif :bmp :tiff
```

JPEG quality remains optional and defaults to `90`.

## Documentation

Added [`docs/IMAGING.md`](IMAGING.md), covering:

- input/output helpers;
- metadata;
- geometry transforms;
- color/filter/compositing operations;
- pixel/procedural image generation;
- Bun Image parity notes and known gaps.

Updated the README documentation index to link the imaging docs.

## Verified checks

Validated locally with targeted commands:

```bash
go test ./std/imaging -count=1
go test ./std/imaging ./cmd/joker -run 'TestMetadataAndTerminals|TestRenderDoc|TestQueryDocs' -count=1
git diff --check
```

Observed results:

- Imaging metadata/terminal tests pass.
- Docs command smoke tests pass.
