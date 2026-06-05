# Sushy-style static site example

This example ports the static-site subset of [`rcarmo/sushy`](https://github.com/rcarmo/sushy) from Hy to Joker.

Sushy is a wiki/blog engine whose core content model is:

- a filesystem tree of pages;
- one folder per page;
- `index.md`, `index.html`, `index.txt`, etc. as the page source;
- RFC2822-style front matter before a blank line;
- content-type based rendering;
- metadata-driven listings, feeds, and site maps.

The original Sushy includes dynamic HTTP serving, SQLite full-text indexing, live file watching, thumbnailing, page aliases, interwiki maps, and HTML transforms. This Joker example focuses on the static generator path: scan, parse, render, transform links, copy static assets, and write static output.

## Run

From the repository root:

```bash
joker examples/sushy-static.joke
```

Or choose custom paths:

```bash
joker examples/sushy-static.joke \
  examples/sushy-static/pages \
  .cache/sushy-site \
  examples/sushy-static/theme
```

Output:

```text
.cache/sushy-site/index.html
.cache/sushy-site/pages.html
.cache/sushy-site/feed.xml
.cache/sushy-site/sitemap.xml
.cache/sushy-site/static/site.css
```

## Content format

Each page lives in a folder and has an `index.*` file:

```text
pages/about/index.md
pages/blog/2026/06/hello/index.md
```

Front matter is parsed like RFC2822 headers:

```text
From: Rui Carmo
Date: 2026-06-05 08:05
Content-Type: text/x-markdown
Title: About
Tags: about, static-site

# About

Page body goes here.
```

Supported content types in the example:

- `text/x-markdown` / `text/markdown` → `joker.markdown/convert-string`
- `text/html` → passed through
- `text/plain` → escaped into `<pre>`

## Sushy features represented

- `store.hy` → `scan-pages`, `load-page`, `parse-front-matter`
- `render.hy` → `render-page-body`
- `transform.hy` → minimal local-link and interwiki rewrite
- `feeds.hy` → Atom feed writer
- sitemap/static export → static output files

Metadata pages under `pages/meta/` are used for mappings and are not rendered as site pages:

- `meta/aliases/index.txt`
- `meta/interwikimap/index.txt`

## Scope

This is deliberately a compact example, not a full Sushy replacement. It omits dynamic serving, SQLite FTS, live file watching, thumbnailing, rich HTML post-processing, and cache headers so the complete pipeline remains easy to read in a single Joker file.
