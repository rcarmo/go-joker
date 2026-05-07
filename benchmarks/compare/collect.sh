#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT_DIR="${1:-${ROOT_DIR}/benchmarks/compare/out/latest}"

mkdir -p "${OUT_DIR}"

echo "[compare] output dir: ${OUT_DIR}"

run_capture() {
  local name="$1"
  shift
  local out_file="${OUT_DIR}/${name}.txt"
  local err_file="${OUT_DIR}/${name}.err"

  echo "[compare] running ${name}: $*"
  if "$@" >"${out_file}" 2>"${err_file}"; then
    rm -f "${err_file}"
    echo "[compare] ${name}: ok"
  else
    echo "[compare] ${name}: skipped/failed (see ${err_file})"
    printf "# SKIPPED: command failed: %s\n" "$*" >"${out_file}"
  fi
}

if command -v python3 >/dev/null 2>&1; then
  run_capture python python3 "${ROOT_DIR}/benchmarks/cross_lang_bench.py"
else
  printf "# SKIPPED: python3 not found\n" >"${OUT_DIR}/python.txt"
fi

if command -v bun >/dev/null 2>&1; then
  run_capture bun bun "${ROOT_DIR}/benchmarks/cross_lang_bench.js"
elif command -v node >/dev/null 2>&1; then
  run_capture bun node "${ROOT_DIR}/benchmarks/cross_lang_bench.js"
else
  printf "# SKIPPED: bun/node not found\n" >"${OUT_DIR}/bun.txt"
fi

if command -v go >/dev/null 2>&1; then
  run_capture goja go run "${ROOT_DIR}/benchmarks/cross_lang_bench_goja.go"
else
  printf "# SKIPPED: go not found\n" >"${OUT_DIR}/goja.txt"
fi

if command -v lg >/dev/null 2>&1; then
  run_capture letgo lg "${ROOT_DIR}/benchmarks/cross_lang_bench.clj"
elif command -v let-go >/dev/null 2>&1; then
  run_capture letgo let-go "${ROOT_DIR}/benchmarks/cross_lang_bench.clj"
else
  printf "# SKIPPED: lg/let-go not found\n" >"${OUT_DIR}/letgo.txt"
fi

if command -v go >/dev/null 2>&1; then
  run_capture report go run "${ROOT_DIR}/benchmarks/compare/render_table.go" \
    -history "${ROOT_DIR}/benchmarks/benchmark-history.json" \
    -python "${OUT_DIR}/python.txt" \
    -bun "${OUT_DIR}/bun.txt" \
    -goja "${OUT_DIR}/goja.txt" \
    -letgo "${OUT_DIR}/letgo.txt" \
    -out "${OUT_DIR}/direct-comparison.md"
else
  printf "# SKIPPED: go not found\n" >"${OUT_DIR}/direct-comparison.md"
fi

echo "[compare] done"
