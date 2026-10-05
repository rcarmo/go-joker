#!/usr/bin/env bash
# Source before any cache/build/temp-producing command, including direct Go use.
# On this host the canonical root is /workspace/tmp/go-joker. CI maps it to an
# absolute runner-owned directory ending in /go-joker; retained evidence is separate.
project_repo=$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel 2>/dev/null) || project_repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
source "$(dirname "${BASH_SOURCE[0]}")/project-tmp.sh" || { return 1 2>/dev/null || exit 1; }
PROJECT_TMP_ROOT=$(project_tmp_resolve go-joker) || { return 1 2>/dev/null || exit 1; }
export PROJECT_TMP_ROOT
project_tmp_init "$PROJECT_TMP_ROOT" || { return 1 2>/dev/null || exit 1; }
# An inherited platform TMPDIR is a resolver candidate, not the child's scratch
# directory. Preserve only already isolated paths inside the resolved root.
case "${TMPDIR:-}" in "$PROJECT_TMP_ROOT"/*) ;; *) TMPDIR="$PROJECT_TMP_ROOT/runs/tmp" ;; esac
case "${GOTMPDIR:-}" in "$PROJECT_TMP_ROOT"/*) ;; *) GOTMPDIR="$PROJECT_TMP_ROOT/runs/go-build" ;; esac
export TMPDIR GOTMPDIR
export TMP="$TMPDIR" TEMP="$TMPDIR"
export GOCACHE=${GOCACHE:-$PROJECT_TMP_ROOT/cache/go-build} GOMODCACHE=${GOMODCACHE:-$PROJECT_TMP_ROOT/cache/go-mod}
for project_dir in "$TMPDIR" "$GOTMPDIR" "$GOCACHE" "$GOMODCACHE"; do
  case "$project_dir" in "$PROJECT_TMP_ROOT"/*) ;; *) echo "Path outside project temporary root: $project_dir" >&2; return 1 2>/dev/null || exit 1 ;; esac
done
export GOPATH="$PROJECT_TMP_ROOT/cache/gopath" GOBIN="$PROJECT_TMP_ROOT/cache/go-tools"
export BUN_INSTALL_CACHE_DIR="$PROJECT_TMP_ROOT/cache/bun"
export npm_config_cache="$PROJECT_TMP_ROOT/cache/npm" PIP_CACHE_DIR="$PROJECT_TMP_ROOT/cache/pip" UV_CACHE_DIR="$PROJECT_TMP_ROOT/cache/uv"
export PYTHONPYCACHEPREFIX="$PROJECT_TMP_ROOT/cache/python"
export PLAYWRIGHT_BROWSERS_PATH="$PROJECT_TMP_ROOT/cache/ms-playwright"
export XDG_CACHE_HOME="$PROJECT_TMP_ROOT/cache/xdg"
export CLI_BIN=${CLI_BIN:-$PROJECT_TMP_ROOT/build/joker}
export DOCS_JOKER_BIN=${DOCS_JOKER_BIN:-$PROJECT_TMP_ROOT/build/go-joker-docs}
# Raw captures/binaries/logs are disposable immediately after analysis/use.
export PROFILE_ROOT=${PROFILE_ROOT:-$PROJECT_TMP_ROOT/runs/profiles}
export PROFILE_CONCLUSIONS_ROOT=${PROFILE_CONCLUSIONS_ROOT:-$project_repo/.cache/profile-conclusions}
case "$PROFILE_ROOT" in "$PROJECT_TMP_ROOT"/runs/profiles|"$PROJECT_TMP_ROOT"/runs/profiles/*) ;; *) echo 'Profiles must use canonical disposable run paths' >&2; return 1 2>/dev/null || exit 1 ;; esac
for project_dir in "$TMPDIR" "$GOTMPDIR" "$GOCACHE" "$GOMODCACHE" "$GOPATH" "$GOBIN" "$BUN_INSTALL_CACHE_DIR" "$npm_config_cache" "$PIP_CACHE_DIR" "$UV_CACHE_DIR" "$PYTHONPYCACHEPREFIX" "$PLAYWRIGHT_BROWSERS_PATH" "$XDG_CACHE_HOME" "$PROJECT_TMP_ROOT/build"; do
  project_path=/
  IFS=/ read -r -a project_parts <<<"${project_dir#/}"
  for project_part in "${project_parts[@]}"; do
    project_path=${project_path%/}/$project_part
    if [[ -L $project_path && $project_path != /workspace ]]; then echo "Refusing symlink: $project_path" >&2; return 1 2>/dev/null || exit 1; fi
  done
  mkdir -p "$project_dir" || { return 1 2>/dev/null || exit 1; }
  if [[ ! -O $project_dir ]]; then echo "Not owned by current user: $project_dir" >&2; return 1 2>/dev/null || exit 1; fi
done
unset project_path project_parts project_part project_dir
