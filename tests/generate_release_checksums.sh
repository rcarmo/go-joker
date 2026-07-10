#!/usr/bin/env bash
set -euo pipefail

release_dir=${1:-release-files}
cd "$release_dir"

platforms=(
  linux-amd64 linux-arm64
  darwin-amd64 darwin-arm64
  windows-amd64 windows-arm64
)
files=()
for platform in "${platforms[@]}"; do
  ext=""
  if [[ $platform == windows-* ]]; then ext=".exe"; fi
  binary="joker-${platform}${ext}"
  sbom="${binary}.spdx.json"
  [[ -f $binary ]] || { echo "missing release binary: $binary" >&2; exit 1; }
  [[ -f $sbom ]] || { echo "missing release SBOM: $sbom" >&2; exit 1; }
  files+=("$binary" "$sbom")
done

LC_ALL=C printf '%s\n' "${files[@]}" | sort | xargs sha256sum > SHA256SUMS
sha256sum --strict --check SHA256SUMS
