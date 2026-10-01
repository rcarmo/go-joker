# Release Notes -- v42.11.4

This release completes the Go module-path fix introduced in v42.11.3. Import packages from `github.com/rcarmo/go-joker/v42`:

```sh
go get github.com/rcarmo/go-joker/v42@v42.11.4
go install github.com/rcarmo/go-joker/v42/cmd/joker@v42.11.4
```

Existing Go consumers must add `/v42` to every go-joker import and run `go mod tidy`. No replacement directive is needed. Repository URLs, downloaded binary names and interpreter behaviour are unchanged. The minimum Go version remains 1.25.

The v42.11.3 tag is importable, but its binary release job stopped because the fresh CI runner lacked a test dependency archive required by the new offline consumer check. CI now runs `go mod download` before the offline checks. Previous tags remain untouched.

Release validation checks the module major, builds a separate interpreter consumer and CLI through an offline proxy, then verifies public proxy resolution, the exact release commit and CLI installation before publication. See [v42.11.3 notes](RELEASE_NOTES_v42.11.3.md) for migration details.
