#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

filelist=$(find "$1" -type f -name "*.clj")

for f in $filelist
do
  ./joker --format "$f" > /tmp/joker-format.clj
  cat /tmp/joker-format.clj > "$f"
done
