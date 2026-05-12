#!/usr/bin/env bash
set -euo pipefail

root="${1:-.}"
cd "$root"

if grep -R "TODO: 32-bit issue" -n core std >/tmp/go-joker-native-int-todos.txt; then
  cat /tmp/go-joker-native-int-todos.txt >&2
  echo "native int guard: unresolved 32-bit conversion TODOs remain" >&2
  exit 1
fi
