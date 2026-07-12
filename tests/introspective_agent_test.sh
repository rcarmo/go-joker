#!/usr/bin/env bash
set -euo pipefail

ROOT="${1:-.}"
JOKER_BIN="${JOKER_BIN:-${ROOT}/.cache/tmp/joker}"
AGENT="examples/agents/introspective-agent.joke"
OUT_DIR="${ROOT}/.cache/introspective-agent-test"

cd "$ROOT"
mkdir -p "$OUT_DIR"

"$JOKER_BIN" "$AGENT" --self-test >"$OUT_DIR/self-test.log"
grep -q 'introspective agent self-test passed' "$OUT_DIR/self-test.log"

"$JOKER_BIN" "$AGENT" --probe >"$OUT_DIR/probe.json"
grep -q '"status": "ok"' "$OUT_DIR/probe.json"
grep -q '"joker.repl/apropos"' "$OUT_DIR/probe.json"
grep -q '"ns-map"' "$OUT_DIR/probe.json"

"$JOKER_BIN" "$AGENT" --tools >"$OUT_DIR/tools.json"
for tool in search-symbols describe-symbol list-namespace-publics macroexpand-form invoke-capability; do
  grep -q "\"name\": \"$tool\"" "$OUT_DIR/tools.json"
done

"$JOKER_BIN" "$AGENT" --search join joker.string 5 >"$OUT_DIR/search-a.json"
"$JOKER_BIN" "$AGENT" --search join joker.string 5 >"$OUT_DIR/search-b.json"
cmp "$OUT_DIR/search-a.json" "$OUT_DIR/search-b.json"

grep -q '"parameters-schema"' "$OUT_DIR/search-a.json"

"$JOKER_BIN" "$AGENT" --tool pure-data invoke-capability \
  '{"qualified-name":"joker.string/join","args":["-",["a","b"]]}' \
  >"$OUT_DIR/invoke.json"
grep -q '"ok": true' "$OUT_DIR/invoke.json"
grep -q '\\"a-b\\"' "$OUT_DIR/invoke.json"

"$JOKER_BIN" "$AGENT" --tool discover-only invoke-capability \
  '{"qualified-name":"joker.string/join","args":[]}' >"$OUT_DIR/denied.json"
grep -q '"category": "policy-denied"' "$OUT_DIR/denied.json"

"$JOKER_BIN" "$AGENT" --tool pure-data invoke-capability \
  '{"qualified-name":"joker.core/*ns*","args":[]}' >"$OUT_DIR/dynamic-denied.json"
grep -q '"category": "policy-denied"' "$OUT_DIR/dynamic-denied.json"

"$JOKER_BIN" "$AGENT" --tool pure-data macroexpand-form \
  '{"form":"(when true (joker.os/env))"}' >"$OUT_DIR/expansion-denied.json"
grep -q '"category": "policy-denied"' "$OUT_DIR/expansion-denied.json"

"$JOKER_BIN" "$AGENT" --macroexpand pure-data '(' >"$OUT_DIR/invalid-form.json"
grep -q '"category": "invalid-form"' "$OUT_DIR/invalid-form.json"
grep -q '"ok": false' "$OUT_DIR/invalid-form.json"

"$JOKER_BIN" "$AGENT" --tool pure-data missing '{}' >"$OUT_DIR/unknown-tool.json"
grep -q '"category": "unknown-tool"' "$OUT_DIR/unknown-tool.json"

"$JOKER_BIN" "$AGENT" --tool pure-data invoke-capability '{broken' \
  >"$OUT_DIR/malformed.json"
grep -q '"category": "malformed-tool-call"' "$OUT_DIR/malformed.json"

JOKER_AGENT_TIMEOUT=1 "$JOKER_BIN" "$AGENT" --invoke pure-data joker.core/range \
  '[0,1000000000]' >"$OUT_DIR/timeout.json"
grep -q '"category": "timeout"' "$OUT_DIR/timeout.json"

if grep -R -E 'OPENROUTER_API_KEY=|Bearer (sk-|[A-Za-z0-9_-]{20,})' "$OUT_DIR"; then
  echo 'possible secret in introspective agent test output' >&2
  exit 1
fi

echo "introspective agent tests passed"
