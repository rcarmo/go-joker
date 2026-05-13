# Std resource layout

Updated: 2026-05-13

This note records the intended repository layout for std namespaces and pure Joker resource sub-namespaces.

## Layout rule

Use this structure consistently:

```text
std/<namespace>.joke
std/<namespace>/*.go
std/<namespace>/<subns>/...
```

Meaning:

- `std/<namespace>.joke` is the root std namespace source consumed by `std/generate-std.joke`.
- `std/<namespace>/*.go` contains the Go package that implements native helpers and generated namespace installers for that std namespace.
- `std/<namespace>/<subns>/...` contains pure Joker resource namespaces that conceptually live under that std namespace tree.

## Example

`joker.http` now follows the intended shape:

```text
std/http.joke
std/http/http_native.go
std/http/a_http.go
std/http/router/router.joke
```

This means:

- `joker.http` is defined by `std/http.joke`
- native/runtime support for `joker.http` lives in `std/http/*.go`
- pure Joker sub-namespace resources such as `joker.http.router` live in `std/http/router/...`

## Rules

- Do not place std resource namespaces under `lib/`.
- Do not place pure Joker std resources in the repository root.
- Nested std Joker resources must live at least one level below the std package directory, e.g. `std/http/router/router.joke`, not `std/http/router.joke`.
- Every nested std resource tree must have a matching root namespace file such as `std/http.joke`.

## Guardrail

`make layout-check` enforces the current structural minimums:

- required extraction-target directories under `core/`
- rejection of `lib/joker/http` drift
- presence of `std/http/router/router.joke`
- nested std resource shape (`std/<namespace>/<subns>/...`) and matching `std/<namespace>.joke`

## Non-goal

This layout rule is about repository structure, not compatibility wrappers. If a namespace belongs under a different std tree, move it and update all callers/docs/tests directly.
