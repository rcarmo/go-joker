#!/usr/bin/env bash
set -euo pipefail

release_dir=${1:-release-files}
expected_tag=${2:?expected tag is required}
expected_revision=${3:?expected git revision is required}
cd "$release_dir"

[[ -f SHA256SUMS ]] || { echo "missing SHA256SUMS" >&2; exit 1; }
sha256sum --strict --check SHA256SUMS

platforms=(
  linux-amd64 linux-arm64
  darwin-amd64 darwin-arm64
  windows-amd64 windows-arm64
)
for platform in "${platforms[@]}"; do
  ext=""
  if [[ $platform == windows-* ]]; then ext=".exe"; fi
  binary="joker-${platform}${ext}"
  sbom="${binary}.spdx.json"
  [[ -s $binary ]] || { echo "missing or empty release binary: $binary" >&2; exit 1; }
  [[ -s $sbom ]] || { echo "missing or empty release SBOM: $sbom" >&2; exit 1; }

  metadata=$(go version -m "$binary")
  grep -Fq $'\tbuild\tvcs.revision='"$expected_revision" <<<"$metadata" || {
    echo "$binary was not built from $expected_revision" >&2
    exit 1
  }
  grep -Fq $'\tbuild\tvcs.modified=false' <<<"$metadata" || {
    echo "$binary was built from a modified worktree" >&2
    exit 1
  }
done

if [[ $(go env GOOS)/$(go env GOARCH) == linux/amd64 ]]; then
  chmod +x joker-linux-amd64
  version_output=$(./joker-linux-amd64 --version 2>&1)
  grep -Fq "$expected_tag" <<<"$version_output" || {
    echo "downloaded linux/amd64 binary did not report $expected_tag: $version_output" >&2
    exit 1
  }
fi

echo "verified release assets for $expected_tag at $expected_revision"
