#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
JOKER_BIN="${JOKER_BIN:-${ROOT}/.cache/tmp/joker}"
OUT_DIR="${ROOT}/.cache/examples-smoke"
WIKI_BUILD_LOG="$OUT_DIR/wiki-build.log"
WIKI_ROOT_HTML="$OUT_DIR/wiki-root.html"
FLAME_LOG="$OUT_DIR/flame.log"
TETRIS_LINT_LOG="$OUT_DIR/tetris-lint.log"
AGENT_EVAL_LOG="$OUT_DIR/lisp-agent-eval.log"

cd "$ROOT"

mkdir -p "$(dirname "$JOKER_BIN")" "$OUT_DIR"
# Always rebuild so example smoke tests current source, not a stale cache binary.
go build -o "$JOKER_BIN" ./cmd/joker

require_file() {
  local path="$1"
  if [ ! -f "$path" ]; then
    echo "missing expected example file: $path" >&2
    exit 1
  fi
}

require_file examples/README.md
require_file examples/agents/lisp-agent.joke
require_file examples/agents/README.md
require_file examples/graphics/fractal-flame.joke
require_file examples/games/tetris.joke
require_file examples/wiki/static.joke
require_file examples/wiki/README.md
require_file examples/notebooks/rich-demo.edn
require_file examples/notebooks/complex-demo.edn

# Agent evaluator smoke does not require an API key or make a network request.
"$JOKER_BIN" examples/agents/lisp-agent.joke --eval \
  '(reduce + (range 1 101))' >"$AGENT_EVAL_LOG"
grep -qx '5050' "$AGENT_EVAL_LOG"

# Static wiki build smoke.
rm -rf "$OUT_DIR/wiki"
"$JOKER_BIN" examples/wiki/static.joke build examples/wiki/pages "$OUT_DIR/wiki" examples/wiki/theme >"$WIKI_BUILD_LOG"

grep -q 'Joker Wiki Static' "$OUT_DIR/wiki/index.html"
grep -q 'Hello from Joker' "$OUT_DIR/wiki/pages.html"
grep -q '<feed' "$OUT_DIR/wiki/feed.xml"
grep -q '<urlset' "$OUT_DIR/wiki/sitemap.xml"
test -f "$OUT_DIR/wiki/static/site.css"

# Dynamic wiki serve smoke.
PORT="${EXAMPLES_SMOKE_PORT:-19304}"
LOG="$OUT_DIR/wiki-serve.log"
"$JOKER_BIN" examples/wiki/static.joke serve examples/wiki/pages "127.0.0.1:${PORT}" examples/wiki/theme >"$LOG" 2>&1 &
server_pid=$!
cleanup() {
  kill "$server_pid" >/dev/null 2>&1 || true
}
trap cleanup EXIT

ready=0
for _ in $(seq 1 50); do
  if curl -fsS "http://127.0.0.1:${PORT}/" >"$WIKI_ROOT_HTML" 2>/dev/null; then
    ready=1
    break
  fi
  if ! kill -0 "$server_pid" >/dev/null 2>&1; then
    echo "wiki serve process exited early" >&2
    cat "$LOG" >&2 || true
    exit 1
  fi
  sleep 0.2
done
if [ "$ready" -ne 1 ]; then
  echo "wiki serve did not become ready on port ${PORT}" >&2
  cat "$LOG" >&2 || true
  exit 1
fi

grep -q 'Joker Wiki Static' "$WIKI_ROOT_HTML"
curl -fsS "http://127.0.0.1:${PORT}/pages.html" | grep -q 'Hello from Joker'
curl -fsS "http://127.0.0.1:${PORT}/feed.xml" | grep -q '<feed'
curl -fsS "http://127.0.0.1:${PORT}/static/site.css" | grep -q 'font'
cleanup
trap - EXIT

# WASM graphics smoke at tiny resolution to keep CI cheap.
rm -f "$OUT_DIR/flame.png" tricorn-flame.png cubic-flame.png
"$JOKER_BIN" examples/graphics/fractal-flame.joke 96 "$OUT_DIR/flame.png" >"$FLAME_LOG"
test -s "$OUT_DIR/flame.png"
rm -f tricorn-flame.png cubic-flame.png

# Interactive game: non-interactive lint/syntax smoke only.
"$JOKER_BIN" --lint examples/games/tetris.joke >"$TETRIS_LINT_LOG" 2>&1 || {
  if grep -q 'Parse error\|Eval error\|Unable to resolve' "$TETRIS_LINT_LOG"; then
    cat "$TETRIS_LINT_LOG" >&2
    exit 1
  fi
  # The current Tetris example has a harmless redundant-do lint warning.
  grep -q 'Parse warning: redundant do form' "$TETRIS_LINT_LOG"
}

# Notebook fixtures are validated by notebook-check; assert paths here.

echo "examples smoke passed"
