# Release Notes -- v42.11.1

This patch release updates notebook front-end dependencies and their locally served assets.

## Dependencies

* CodeMirror 5.65.18 to 5.65.21, using the maintained v5 channel. CodeMirror 6 requires a separate editor integration migration and is not included.
* Mermaid 11.15.0 to 11.17.2, including the vendored browser bundle and refreshed transitive dependencies.
* Playwright 1.60.0 (previous lockfile resolution) to 1.63.0, with Chromium 153 for local browser testing.
* ECharts remains at 6.1.0, the current stable release; its vendored bundle is verified against the installed package.

Direct package versions are pinned and `bun.lock` is refreshed. Notebook assets continue to be served locally without a CDN.

## Validation

The notebook Playwright smoke test passed with the upgraded browser/tooling. Release validation uses the full pre-tag gate, including Go tests, vet, documentation and generated-file checks, plus browser smoke. Release CI builds and verifies all supported platform binaries and supply-chain assets.
