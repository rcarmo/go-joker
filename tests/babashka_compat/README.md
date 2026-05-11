# Babashka compatibility fixtures

This directory contains a deliberately small, portable fixture suite for Babashka-style scripts.

## Positive fixtures

The `positive/` scripts exercise the subset go-joker intends to support for portable automation:

- core Clojure data operations and EDN round-trips
- JSON/YAML/base64/hex codecs
- basic filesystem read/write/delete
- HTTP client requests against a local test server

## Expected-failure fixtures

The `expected_failure/` scripts pin explicit non-goal messages for features go-joker should not chase by default:

- arbitrary Java/JVM interop
- `bb.edn` task/deps/classpath assembly
- SCI analyzer/evaluator internals
- broad Babashka bundled library catalog APIs

These fixtures are not placeholders for future implementation; they are guardrails to keep the compatibility scope script-driven.

## Portable namespace notes

Prefer go-joker namespaces directly in portable scripts:

- `joker.edn` or `edn` for EDN data
- `joker.json` / `joker.yaml` for data codecs
- `joker.http` for HTTP client/server work
- `joker.os` and core `slurp`/`spit` for filesystem/process basics
- `pods` or `babashka.pods` for pod loading/invocation

Avoid JVM class symbols, Java constructors/static members, `bb.edn` task assumptions, and SCI implementation hooks.
