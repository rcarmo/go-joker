#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
cd "$ROOT"

mkdir -p .cache/tmp
MATCHES_FILE="$(mktemp .cache/tmp/go-joker-doc-paths.XXXXXX)"
trap 'rm -f "$MATCHES_FILE"' EXIT

fail=0

check_absent() {
  local pattern="$1"
  local label="$2"
  : >"$MATCHES_FILE"
  if grep -R --line-number --fixed-strings "$pattern" README.md docs examples \
      --include='*.md' --include='*.joke' --include='*.edn' --include='*.html' >"$MATCHES_FILE" 2>/dev/null; then
    echo "stale path/reference found for $label: $pattern" >&2
    cat "$MATCHES_FILE" >&2
    fail=1
  fi
}

check_present() {
  local path="$1"
  if [ ! -e "$path" ]; then
    echo "documented path missing: $path" >&2
    fail=1
  fi
}

# Paths moved during examples reorganization.
check_absent 'examples/fractal-flame.joke' 'old fractal example path'
check_absent 'examples/tetris.joke' 'old tetris example path'
check_absent 'examples/wiki-static.joke' 'old wiki example script path'
check_absent 'examples/wiki-static' 'old wiki example directory path'
check_absent 'examples/sushy-static' 'old sushy example path'
check_absent 'sushy-site' 'old sushy output path'

# Current high-value examples and docs paths.
check_present examples/README.md
check_present examples/agents/lisp-agent.joke
check_present examples/agents/introspective-agent.joke
check_present examples/agents/README.md
check_present examples/agents/HARDENING.md
check_present tests/introspective_agent_test.sh
check_present examples/graphics/fractal-flame.joke
check_present examples/games/tetris.joke
check_present examples/wiki/static.joke
check_present examples/wiki/README.md
check_present examples/notebooks/rich-demo.edn
check_present examples/notebooks/complex-demo.edn
check_present docs/API_STABILITY.md

if [ "$fail" -ne 0 ]; then
  exit 1
fi

echo "docs path guard passed"
