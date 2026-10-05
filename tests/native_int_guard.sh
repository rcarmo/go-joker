#!/usr/bin/env bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)/scripts/project-env.sh" || exit 1
set -euo pipefail

root="${1:-.}"
cd "$root"

mkdir -p "$TMPDIR"
todos_file="$(mktemp "$TMPDIR"/go-joker-native-int-todos.XXXXXX)"
trap 'rm -f "$todos_file"' EXIT

if grep -R "TODO: 32-bit issue" -n core std >"$todos_file"; then
  cat "$todos_file" >&2
  echo "native int guard: unresolved 32-bit conversion TODOs remain" >&2
  exit 1
fi
