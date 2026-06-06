# Go-Joker examples

This directory contains runnable Joker examples grouped by purpose.

## Graphics

### Fractal flame / procedural raster

```bash
joker examples/graphics/fractal-flame.joke 1024 .cache/flame.png
```

Demonstrates high-resolution procedural image generation with:

- `joker.jit/compile-wasm`
- `joker.imaging/from-rgba32-domain-fn`
- packed RGBA32 pixel output

The example renders Mandelbrot, Tricorn, and cubic flame-style kernels entirely from Joker code compiled to WASM.

## Games / terminal UI

### Tetris

```bash
joker examples/games/tetris.joke
```

Demonstrates `joker.term` raw terminal mode, alternate screen buffer, key input, ANSI color, and buffered frame rendering.

## Wiki / static site

### Wiki-style generator and server

```bash
joker examples/wiki/static.joke build
joker examples/wiki/static.joke serve
```

Demonstrates a compact wiki/static-site pipeline based on `rcarmo/sushy`:

- folder-per-page content tree;
- lowercase RFC2822-style front matter;
- Markdown/plain/HTML rendering;
- local link and interwiki transforms;
- static build output;
- dynamic serving with `joker.http/start-server`;
- Atom feed and sitemap generation.

See [`examples/wiki/README.md`](wiki/README.md).

## Notebooks

```bash
joker notebook examples/notebooks/rich-demo.edn
joker notebook examples/notebooks/complex-demo.edn
```

Notebook demos cover rich outputs, dependencies, charts/tables/HTML/SVG/images, and WASM-backed image generation.
