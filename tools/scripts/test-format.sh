#!/usr/bin/env bash
source "$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)/scripts/project-env.sh" || exit 1
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

filelist=$(find "$1" -type f -name "*.clj")

work=$(mktemp -d "$TMPDIR/format.XXXXXX")
trap 'rm -rf "$work"' EXIT
for f in $filelist
do
  "$CLI_BIN" --format "$f" > "$work/joker-format.clj"
  cat "$work/joker-format.clj" > "$f"
done
