from: Rui Carmo
date: 2026-06-05 08:05
content-type: text/x-markdown
title: About
tags: about, static-site

# About

This example ports the wiki/static-site subset of [`rcarmo/sushy`](https://github.com/rcarmo/sushy) from Hy to Joker.

It demonstrates:

- RFC2822-style front matter parsing
- folder-per-page content layout
- Markdown, HTML, and plaintext rendering
- dynamic rendering via `joker.http/start-server`
- static output for pages, an index, Atom feed, and sitemap
- simple alias and interwiki expansion
