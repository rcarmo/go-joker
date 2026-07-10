#!/usr/bin/env bash
set -euo pipefail

root=${1:-.}
bin=${2:?usage: docs_generation_guard.sh ROOT JOKER_BIN}
root=$(cd "$root" && pwd)
bin=$(cd "$(dirname "$bin")" && pwd)/$(basename "$bin")
tmp_root=${TMPDIR:-$root/.cache/tmp}
mkdir -p "$tmp_root"
out=$(mktemp -d "$tmp_root/go-joker-docs.XXXXXX")
trap 'rm -rf "$out"' EXIT

cp "$root/docs/generate-docs.joke" "$out/"
cp -R "$root/docs/templates" "$out/"
(
  cd "$out"
  "$bin" generate-docs.joke > docs-generation.log
)
cat "$out/docs-generation.log"
if grep -q WARNING "$out/docs-generation.log"; then
  echo "documentation generation emitted warnings" >&2
  exit 1
fi

status=0
for generated in "$out"/*.html "$out"/main.js; do
  name=$(basename "$generated")
  tracked="$root/docs/$name"
  if [[ ! -f "$tracked" ]]; then
    echo "generated documentation is untracked: docs/$name" >&2
    status=1
    continue
  fi
  if ! diff -u "$tracked" "$generated"; then
    echo "generated documentation is stale: docs/$name" >&2
    status=1
  fi
done
for tracked in "$root"/docs/*.html "$root"/docs/main.js; do
  name=$(basename "$tracked")
  if [[ ! -f "$out/$name" ]]; then
    echo "tracked generated documentation is obsolete: docs/$name" >&2
    status=1
  fi
done
exit "$status"
