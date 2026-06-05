# Wiki-style site example

This example ports the wiki/static-site subset of [`rcarmo/sushy`](https://github.com/rcarmo/sushy) from Hy to Joker.

Sushy is a wiki/blog engine whose core content model is:

- a filesystem tree of pages;
- one folder per page;
- `index.md`, `index.html`, `index.txt`, etc. as the page source;
- RFC2822-style front matter before a blank line;
- content-type based rendering;
- metadata-driven listings, feeds, and site maps.

The original Sushy includes dynamic HTTP serving, SQLite full-text indexing, live file watching, thumbnailing, page aliases, interwiki maps, and HTML transforms. This Joker example focuses on a readable wiki core: scan, parse, render, transform links, copy static assets, write static output, and optionally serve the same pages dynamically.

## Run

From the repository root:

Build static output:

```bash
joker examples/wiki-static.joke build
```

Or choose custom paths:

```bash
joker examples/wiki-static.joke build \
  examples/wiki-static/pages \
  .cache/wiki-site \
  examples/wiki-static/theme
```

Serve dynamically from the content tree:

```bash
joker examples/wiki-static.joke serve
```

Or choose a custom bind address:

```bash
joker examples/wiki-static.joke serve \
  examples/wiki-static/pages \
  127.0.0.1:8080 \
  examples/wiki-static/theme
```

Output:

```text
.cache/wiki-site/index.html
.cache/wiki-site/pages.html
.cache/wiki-site/feed.xml
.cache/wiki-site/sitemap.xml
.cache/wiki-site/static/site.css
```

## Content format

Each page lives in a folder and has an `index.*` file:

```text
pages/about/index.md
pages/blog/2026/06/hello/index.md
```

Front matter is parsed like RFC2822 headers:

```text
from: Rui Carmo
date: 2026-06-05 08:05
content-type: text/x-markdown
title: About
tags: about, static-site

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
- `routes.hy`/dynamic serving idea → `joker.http/start-server` handler that renders on demand

Metadata pages under `pages/meta/` are used for mappings and are not rendered as site pages:

- `meta/aliases/index.txt`
- `meta/interwikimap/index.txt`

## Scope

This is deliberately a compact example, not a full Sushy replacement. It omits SQLite FTS, live file watching, thumbnailing, rich HTML post-processing, and cache headers so the complete pipeline remains easy to read in a single Joker file.
