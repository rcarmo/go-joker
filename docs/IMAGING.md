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

- `(imaging/open path)` — open PNG/JPEG/GIF/BMP/TIFF files supported by the backend.
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

## Bun Image parity notes

Compared with Bun's chainable `Bun.Image` API, `joker.imaging` now covers the low-friction metadata, terminal, and similarity helpers:

- metadata map;
- bytes terminal;
- base64 terminal;
- data URI terminal;
- perceptual average/difference hashes.

Known gaps:

- no WebP/AVIF/HEIC encoder yet;
- no Blob abstraction;
- no lazy image pipeline object;
- no option-map variants for resize/encode yet.

The functional Joker style remains the preferred surface for now; threading macros already provide a compact pipeline form.
