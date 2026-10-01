#!/usr/bin/env bash
set -euo pipefail

root=${1:-.}
cd "$root"

legacy_import_re='"github.com/[^/]+/joker(/|"|[[:space:]]|$)|^module[[:space:]]+github.com/[^/]+/joker(/|[[:space:]]|$)'

# A major-version migration must cover generators and generated imports too.
# Match package paths, not HTTPS repository/source links.
old_module_re='"github[.]com/rcarmo/go-joker(/|"|[[:space:]]|$)'
if grep -R -E "$old_module_re" -n \
  --include='*.go' --include='*.joke' cmd core std tests benchmarks tools \
  | grep -v 'github.com/rcarmo/go-joker/v42/' \
  | grep -v 'const moduleBase ='; then
  echo "import identity guard: package/template reference lacks /v42" >&2
  exit 1
fi

if grep -R -E "$legacy_import_re" -n \
  --include='*.go' --include='go.mod' --include='*.joke' \
  cmd core std tests benchmarks tools Makefile .github .circleci docs/generate-docs.joke docs/joker.xml 2>/dev/null; then
  echo "import identity guard: internal package/template references still use a legacy joker module path" >&2
  exit 1
fi
