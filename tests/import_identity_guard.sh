#!/usr/bin/env bash
set -euo pipefail

root=${1:-.}
cd "$root"

legacy_import_re='github.com/[^/]+/joker(["[:space:]]|$)'

if grep -R -E "$legacy_import_re" -n \
  --include='*.go' --include='go.mod' --include='*.joke' \
  cmd core std tests benchmarks tools Makefile .github .circleci docs/generate-docs.joke docs/joker.xml 2>/dev/null; then
  echo "import identity guard: internal package/template references still use a legacy joker module path" >&2
  exit 1
fi
