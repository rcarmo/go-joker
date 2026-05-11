#!/usr/bin/env bash
set -euo pipefail

root=${1:-.}
cd "$root"

if grep -R "github.com/candid82/joker" -n \
  --include='*.go' --include='go.mod' --include='*.joke' \
  cmd core std tests benchmarks tools Makefile .github .circleci build.sh build-arm.sh docs/generate-docs.joke docs/joker.xml 2>/dev/null; then
  echo "import identity guard: internal package/template references still use github.com/candid82/joker" >&2
  exit 1
fi
