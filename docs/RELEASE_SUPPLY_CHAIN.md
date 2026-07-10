# Release supply chain

Tag-triggered releases publish verification material with every supported binary. The workflow in `.github/workflows/build.yml` is the source of truth.

## Published assets

For each Linux, macOS, and Windows `amd64`/`arm64` binary, the release includes:

- the stripped, `CGO_ENABLED=0`, `-trimpath` binary;
- an SPDX JSON SBOM generated from that binary;
- a GitHub build-provenance attestation bound to the binary digest;
- a GitHub SBOM attestation binding the SPDX document to the binary digest.

`SHA256SUMS` covers all six binaries and all six SBOM files. It is generated only after the matrix artifacts have been downloaded into the release job.

## Publication gate

Before publication, `tests/verify_release_assets.sh`:

1. verifies every entry in `SHA256SUMS`;
2. requires the complete six-platform binary/SBOM set;
3. inspects Go build metadata in every binary and requires the tag commit plus `vcs.modified=false`;
4. executes the downloaded Linux `amd64` binary on the release runner and checks that `--version` reports the tag.

A failure prevents release creation or asset upload. `make release-supply-chain-check`, included in `make release-check`, guards the workflow contract and tests checksum tamper detection without producing a release.

## Consumer verification

After downloading a release into one directory:

```bash
sha256sum --strict --check SHA256SUMS
gh attestation verify joker-linux-amd64 --repo rcarmo/go-joker
gh attestation verify joker-linux-amd64 \
  --repo rcarmo/go-joker --predicate-type https://spdx.dev/Document/v2.3
./joker-linux-amd64 --version
```

Use the matching filename on other platforms. GitHub attestation verification checks the artifact digest and repository identity; checksum verification checks the complete downloaded set against the release manifest.
