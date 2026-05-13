#!/usr/bin/env bash
set -euo pipefail

root=${1:-.}
cd "$root"

status=0
fail() { echo "layout guard: $*" >&2; status=1; }

[[ -f cmd/joker/main.go ]] || fail "missing CLI entrypoint cmd/joker/main.go"
[[ -d core/internal/trace ]] || fail "missing core/internal/trace"
[[ -d core/internal/ir ]] || fail "missing core/internal/ir"
[[ -d core/internal/wasm ]] || fail "missing core/internal/wasm"
[[ -d core/internal/generated ]] || fail "missing core/internal/generated"
[[ -f std/http/router/router.joke ]] || fail "missing std/http/router/router.joke"

if [[ -d lib/joker/http ]]; then
  fail "loose HTTP libraries under lib/joker/http are not allowed; place joker.http.* resources under std/http/"
fi

if find . -maxdepth 1 -type f -name '*.go' | grep -q .; then
  find . -maxdepth 1 -type f -name '*.go' -print >&2
  fail "root package Go files are not allowed; CLI belongs in cmd/joker and runtime code belongs in packages"
fi

if ! grep -qx 'module github.com/rcarmo/go-joker' go.mod; then
  fail "go.mod module path is not github.com/rcarmo/go-joker"
fi

exit "$status"
