#!/usr/bin/env bash
# After reviewing a run, retain concise conclusions and immediately dispose raw data.
set -euo pipefail
source "$(dirname "${BASH_SOURCE[0]}")/project-env.sh"
run=${1:?profile run directory required}
note=${2:?concise analysis conclusion required}
[[ -n ${note//[[:space:]]/} ]] || { echo 'empty analysis conclusion' >&2; exit 1; }
case "$run" in "$PROJECT_TMP_ROOT"/runs/profiles/run-*|"$PROJECT_TMP_ROOT"/runs/profiles/*/run-*) ;; *) echo 'Refusing disposal outside canonical profile runs' >&2; exit 1;; esac
[[ -d $run && ! -L $run && -O $run ]] || { echo 'Unsafe/missing profile run' >&2; exit 1; }
# Reject traversal before walking ancestors (never follow project-owned links).
[[ $run != *'/../'* && $run != *'/./'* ]] || exit 1
parent=$run
while [[ $parent != "$PROJECT_TMP_ROOT" ]]; do
 [[ ! -L $parent ]] || { echo 'Symlink in run path' >&2; exit 1; }
 parent=${parent%/*}
done
mkdir -p "$PROFILE_CONCLUSIONS_ROOT"
out="$PROFILE_CONCLUSIONS_ROOT/$(basename "$run").md"
{
 printf '# Go profile conclusions\n\n%s\n\n' "$note"
 [[ ! -f $run/status.tsv ]] || { printf '## Result\n\n```\n'; cat "$run/status.tsv"; printf '```\n'; }
 # Keep a few diagnostic totals/top application lines, not profiles/log dumps.
 while IFS= read -r -d '' report; do
  printf '\n### %s\n' "${report#"$run"/}"
  grep -m1 -E 'Total samples =|Showing nodes accounting' "$report" || true
  grep '/go-joker/\|/go-joker/v42/' "$report" | head -3 || true
 done < <(find "$run" -type f \( -name cpu.txt -o -name alloc_space.txt -o -name alloc_objects.txt \) -print0)
} > "$out"
rm -rf -- "$run"
printf 'Disposed raw profiles/binaries/logs; conclusions: %s\n' "$out"
