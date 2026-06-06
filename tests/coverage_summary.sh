#!/usr/bin/env bash
set -euo pipefail

profile=${1:-.cache/tmp/go-joker.cover}
out=${2:-.cache/tmp/go-joker.cover.func}
mkdir -p "$(dirname "$out")"

if [[ ! -f "$profile" ]]; then
  echo "coverage profile not found: $profile" >&2
  exit 2
fi

go tool cover -func="$profile" > "$out"

echo "== Aggregate coverage =="
tail -1 "$out"

echo
echo "== Non-generated function coverage =="
awk '
  function generated(path) {
    return path ~ /\/a_[^/]*\.go:/ || path ~ /types_.*_gen\.go:/ || path ~ /\/gen_code\//
  }
  $1 ~ /^github.com\// && $3 ~ /%$/ && !generated($1) {
    pct=$3; gsub("%", "", pct); pct += 0; total++; sum += pct;
    if (pct > 0) covered++;
    if (pct == 0) zero++;
  }
  END {
    if (total == 0) { print "no non-generated functions found"; exit 0 }
    printf "functions with coverage: %d/%d (%.1f%%)\n", covered, total, covered*100/total;
    printf "mean function coverage: %.1f%%\n", sum/total;
    printf "zero-coverage functions: %d\n", zero;
  }
' "$out"

echo

echo "== Gap-closure package coverage =="
awk '
  $1 ~ /\/std\/(pods|transit|edn)\// && $3 ~ /%$/ {
    pct=$3; gsub("%", "", pct); pkg=$1; sub(/:[0-9]+.*/, "", pkg);
    split(pkg, parts, "/std/"); split(parts[2], rest, "/"); name=rest[1]; count[name]++; sum[name]+=pct+0;
  }
  END {
    for (name in count) printf "%s mean function coverage: %.1f%% (%d funcs)\n", name, sum[name]/count[name], count[name];
  }
' "$out" | sort

echo
echo "== Lowest non-generated functions =="
awk '
  function generated(path) {
    return path ~ /\/a_[^/]*\.go:/ || path ~ /types_.*_gen\.go:/ || path ~ /\/gen_code\//
  }
  $1 ~ /^github.com\// && $3 ~ /%$/ && !generated($1) {
    pct=$3; gsub("%", "", pct); pct += 0; printf "%6.1f%% %s %s\n", pct, $1, $2;
  }
' "$out" | sort -n | sed -n '1,40p'
