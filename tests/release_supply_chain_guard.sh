#!/usr/bin/env bash
set -euo pipefail

ROOT=${1:-.}
cd "$ROOT"

workflow=.github/workflows/build.yml
for required in \
  'actions/attest-build-provenance@v2' \
  'actions/attest-sbom@v2' \
  'anchore/sbom-action@v0' \
  'tests/generate_release_checksums.sh release-files' \
  'tests/verify_release_assets.sh release-files' \
  'gh release upload "$TAG" release-files/* --clobber'; do
  grep -Fq "$required" "$workflow" || {
    echo "release workflow is missing supply-chain step: $required" >&2
    exit 1
  }
done

grep -A5 '^  build:' "$workflow" | grep -Fq 'id-token: write' || {
  echo "release build job cannot issue attestations" >&2
  exit 1
}
grep -A5 '^  build:' "$workflow" | grep -Fq 'attestations: write' || {
  echo "release build job cannot persist attestations" >&2
  exit 1
}

bash -n tests/generate_release_checksums.sh tests/verify_release_assets.sh

fixture=$(mktemp -d "${TMPDIR:-.cache/tmp}/release-assets.XXXXXX")
trap 'rm -rf "$fixture"' EXIT
for platform in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 windows-arm64; do
  ext=""
  if [[ $platform == windows-* ]]; then ext=.exe; fi
  printf 'binary-%s\n' "$platform" > "$fixture/joker-${platform}${ext}"
  printf '{"spdxVersion":"SPDX-2.3","name":"%s"}\n' "$platform" > "$fixture/joker-${platform}${ext}.spdx.json"
done

tests/generate_release_checksums.sh "$fixture" >/dev/null
[[ $(wc -l < "$fixture/SHA256SUMS") -eq 12 ]] || {
  echo "SHA256SUMS does not cover all six binaries and SBOMs" >&2
  exit 1
}
printf 'tampered\n' >> "$fixture/joker-linux-amd64"
if (cd "$fixture" && sha256sum --strict --check SHA256SUMS >/dev/null 2>&1); then
  echo "release checksum verification accepted a modified binary" >&2
  exit 1
fi

echo "release supply-chain guard passed"
