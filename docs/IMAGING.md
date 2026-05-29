# Joker imaging

`joker.imaging` is an eager, functional image API backed by Go's image packages and `disintegration/imaging`.

It is intentionally Joker-shaped rather than a clone of Bun's chainable `Bun.Image` pipeline, but the common decode/transform/encode workflow is covered:

```clojure
(require '[joker.imaging :as imaging])

(-> (imaging/open "photo.jpg")
    (imaging/fit 400 400)
    (imaging/save "thumb.jpg"))

(-> (imaging/open "photo.jpg")
    (imaging/fit 400 400)
    (imaging/data-uri :png))
```

## Input/output

- `(imaging/open path)` — open PNG/JPEG/GIF/BMP/TIFF/WebP files supported by the backend.
- `(imaging/save img path)` — save using the output extension.
- `(imaging/encode img format & quality)` — return encoded bytes as a Joker string.
- `(imaging/bytes img format & quality)` — explicit alias for `encode`.
- `(imaging/base64 img format & quality)` — return encoded image bytes as base64 text.
- `(imaging/data-uri img format & quality)` — return a `data:image/...;base64,...` URI.
- `(imaging/decode data)` — decode image bytes from a Joker string.

Supported encode formats today:

```clojure
:png :jpeg :jpg :gif :bmp :tiff
```

JPEG quality defaults to `90` when omitted.

**Input formats** (decode/open): PNG, JPEG, GIF, BMP, TIFF, **WebP**.
WebP encode is not yet supported (would require cgo + libwebp).

## Metadata

- `(imaging/width img)`
- `(imaging/height img)`
- `(imaging/bounds img)` — `[x y width height]`
- `(imaging/metadata img)` — `{:width ... :height ... :bounds [...] :color-model :nrgba}`

Images are normalized to NRGBA internally, so `metadata` reports the in-memory color model rather than the original file format.

## Geometry

- `(imaging/resize img width height)`
- `(imaging/fit img width height)`
- `(imaging/fill img width height & anchor)`
- `(imaging/crop img x y width height)`
- `(imaging/crop-center img width height)`
- `(imaging/rotate img degrees)`
- `(imaging/flip-h img)`
- `(imaging/flip-v img)`
- `(imaging/transpose img)`
- `(imaging/transverse img)`

Anchors for `fill` include `:center`, `:top-left`, `:top`, `:top-right`, `:left`, `:right`, `:bottom-left`, `:bottom`, and `:bottom-right`.

## Color, filters, and compositing

- `(imaging/grayscale img)`
- `(imaging/invert img)`
- `(imaging/brightness img amount)`
- `(imaging/contrast img amount)`
- `(imaging/saturation img amount)`
- `(imaging/gamma img value)`
- `(imaging/sigmoid img midpoint factor)`
- `(imaging/blur img sigma)`
- `(imaging/sharpen img sigma)`
- `(imaging/overlay base top x y & opacity)`
- `(imaging/paste base top x y)`

## Perceptual hashes

- `(imaging/average-hash img)` — 64-bit average hash as 16 lowercase hex characters.
- `(imaging/difference-hash img)` — 64-bit difference hash as 16 lowercase hex characters.
- `(imaging/hash img)` — default perceptual hash, currently `difference-hash`.

These hashes are intended for quick similarity checks, deduplication, and notebook/browser workflows. They are not cryptographic hashes.

## Pixel and procedural generation

- `(imaging/new width height & color)`
- `(imaging/pixel img x y)`
- `(imaging/set-pixel! img x y [r g b a])`
- `(imaging/from-rgba32 width height pixels)`
- `(imaging/from-rgba32-fn width height pixel-fn)`
- `(imaging/from-rgba32-domain-fn width height xmin ymin dx dy pixel-fn)`

The RGBA32 helpers are intended for notebooks and procedural demos. Pixels are packed as `0xRRGGBBAA`.

## Bun Image parity audit

Bun's `Bun.Image` is a lazy, chainable image pipeline: construct from a path/bytes/blob, chain transforms, choose a format, then await a terminal such as `write`, `bytes`, or `blob`. `joker.imaging` is eager and functional: each call returns an image or terminal value immediately and composes naturally with threading macros.

### Covered or close equivalents

| Bun Image area | `joker.imaging` equivalent | Notes |
|---|---|---|
| Path input | `(imaging/open path)` | File-backed decode. |
| Bytes input | `(imaging/decode data)` | Joker string carries bytes. |
| Metadata | `(imaging/metadata img)`, `width`, `height`, `bounds` | Does not retain original input format yet. |
| Resize | `resize`, `fit`, `fill` | Separate functions instead of `resize(..., {fit})`. |
| Crop | `crop`, `crop-center` | Explicit rectangle/center crop. |
| Rotate | `rotate` | Arbitrary degrees. |
| Flip | `flip-h`, `flip-v`, `transpose`, `transverse` | Covers common orientation transforms. |
| Modulate-style color changes | `brightness`, `contrast`, `saturation`, `gamma`, `sigmoid`, `grayscale`, `invert` | Separate operations instead of one option map. |
| Output write | `(imaging/save img path)` | Format inferred by backend from extension. |
| Output bytes | `(imaging/bytes img format & quality)` / `encode` | Returns encoded bytes as a Joker string. |
| Base64/data URL terminals | `base64`, `data-uri` | Added for notebook/browser workflows. |
| PNG/JPEG output | `encode`/`bytes`/`base64`/`data-uri` with `:png`, `:jpeg` | JPEG quality supported. |
| TIFF/BMP/GIF output | `:tiff`, `:bmp`, `:gif` | Available through current backend. |

### Joker-only additions

| Area | API | Notes |
|---|---|---|
| Pixel read/write | `pixel`, `set-pixel!` | Direct per-pixel access. |
| Blank images | `new` | Optional `[r g b a]` color. |
| Procedural raster generation | `from-rgba32`, `from-rgba32-fn`, `from-rgba32-domain-fn` | Useful for notebooks and numeric demos. |
| Perceptual hashes | `average-hash`, `difference-hash`, `hash` | Lightweight similarity/dedup helpers; not cryptographic. |
| Compositing | `overlay`, `paste` | Alpha and direct compositing helpers. |
| Filters | `blur`, `sharpen` | Classic image filters. |

### Remaining gaps versus Bun Image

| Gap | Impact | Likely next step |
|---|---|---|
| WebP decode | ✅ Done — registered via `golang.org/x/image/webp`. | |
| WebP encode | Medium; requires cgo (`libwebp`) for lossy output. | Defer unless strong use case. |
| AVIF/HEIC encode/decode | Medium; useful, but heavier/platform-sensitive. | Defer unless there is a strong use case. |
| Blob abstraction | Low in Joker today. | Only add if Joker gets a broader Blob/file API. |
| Lazy chain/pipeline object | Low-to-medium ergonomics gap. | Prefer functional API for now; threading macros cover most use. |
| Option-map resize/encode variants | Medium ergonomics/extensibility gap. | Add optional map overloads while preserving current positional APIs. |
| Original format retention in metadata | Medium for audits/conversion tools. | Store source format on `Image` during `open`/`decode`. |
| Orientation/EXIF handling | Medium for photos. | Consider EXIF-aware open/auto-orient if photo workflows matter. |
| SIMD/native backend performance parity | Medium-to-high for large batches. | Current implementation is pure Go and portable, not Bun's native/SIMD pipeline. |

### Recommended order

1. ~~Add WebP support~~ — **done** (decode via `golang.org/x/image/webp`; encode deferred pending cgo/libwebp decision).
2. Retain original decoded format in `Image` and expose it in `metadata`.
3. Add option-map overloads for `resize`/`encode` to mirror Bun's extensibility without replacing the functional API.
4. Consider EXIF auto-orientation for camera/photo workflows.

The functional Joker style remains the preferred surface; threading macros already provide compact pipeline composition without introducing a separate lazy pipeline object.
