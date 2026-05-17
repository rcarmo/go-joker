#!/usr/bin/env bash
set -euo pipefail

root=${1:-.}
cd "$root"

status=0
fail() { echo "layout guard: $*" >&2; status=1; }

[[ -f cmd/joker/main.go ]] || fail "missing CLI entrypoint cmd/joker/main.go"
for dir in trace ir wasm generated runtime collections reader string cursor types objectmodel evaluator bootstrap procsystem; do
  [[ -d "core/${dir}" ]] || fail "missing core/${dir}"
done
for dir in runtime collections reader ir wasm generated string cursor types objectmodel evaluator bootstrap procsystem; do
  [[ -f "core/${dir}/doc.go" ]] || fail "missing core/${dir}/doc.go"
done
[[ -f std/http/router/router.joke ]] || fail "missing std/http/router/router.joke"

if [[ -d lib/joker/http ]]; then
  fail "loose HTTP libraries under lib/joker/http are not allowed; place joker.http.* resources under std/http/"
fi

while IFS= read -r joke; do
  rel=${joke#std/}
  ns=${rel%%/*}
  if [[ ! -f "std/${ns}.joke" ]]; then
    fail "nested std resource ${joke} requires root namespace file std/${ns}.joke"
  fi
  if [[ ${rel#*/} != */* ]]; then
    fail "nested std resource ${joke} must live under std/${ns}/<subns>/..."
  fi
done < <(find std -mindepth 2 -type f -name '*.joke' | sort)

if find . -maxdepth 1 -type f -name '*.go' | grep -q .; then
  find . -maxdepth 1 -type f -name '*.go' -print >&2
  fail "root package Go files are not allowed; CLI belongs in cmd/joker and runtime code belongs in packages"
fi

while IFS= read -r file; do
  if grep -q '^package core$' "$file"; then
    fail "package core file in subpackage directory is not a real package move: $file"
  fi
done < <(find core -mindepth 2 -type f -name '*.go' ! -path 'core/gen/*' | sort)

if grep -R '^func Benchmark' core --include='*_test.go' >/dev/null; then
  grep -R '^func Benchmark' core --include='*_test.go' >&2
  fail "core package benchmark functions belong under benchmarks/core, not root core"
fi

for pkg in runtime collections reader cursor; do
  if grep -R 'github.com/rcarmo/go-joker/core"' "core/${pkg}" --include='*.go' >/dev/null; then
    fail "core/${pkg} must not import root core; define an adapter contract before moving coupled code"
  fi
done

for artifact in core.test joker transit.test; do
  if [[ -e "$artifact" ]]; then
    fail "stale root build artifact present: $artifact"
  fi
done

if ! grep -qx 'module github.com/rcarmo/go-joker' go.mod; then
  fail "go.mod module path is not github.com/rcarmo/go-joker"
fi

exit "$status"
