# Release Notes -- v42.11.3

This patch fixes Go module consumption by declaring `github.com/rcarmo/go-joker/v42`. Previous v42 tags declared an unversioned module path, so Go rejected them and resolved the old path's `@latest` to v1.8.0.

## Go consumers

```sh
go get github.com/rcarmo/go-joker/v42@v42.11.3
```

Add `/v42` to package imports, for example `github.com/rcarmo/go-joker/v42/core`, then run `go mod tidy`. The old and new paths are distinct Go modules; update all go-joker imports together. No replacement directive is needed. The minimum Go version remains 1.25.

Install the CLI with:

```sh
go install github.com/rcarmo/go-joker/v42/cmd/joker@v42.11.3
```

Existing tags remain unchanged. GitHub repository/source URLs and downloaded binary names are unchanged. This release changes module identity and validation; it does not change interpreter behaviour.

## Release checks

Internal imports, generator templates and generated import declarations use `/v42`. The one-time `tools/codegen/migrate-module-v42.ts` migration updates paths without regenerating bootstrap payloads.

`make module-consumer-check` checks the release major against the module declaration, serves the checkout through an offline file proxy, and builds a separate consumer without `replace` or workspace overrides. It tests interpreter evaluation and builds the CLI from the versioned module. Cached dependencies are required. Both pre-tag and CI release checks run it; public tag resolution is checked again before GitHub Release publication.
