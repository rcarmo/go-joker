# Release Notes -- v42.12.1

`joker.ffi` is now enabled in normal builds and published binaries on Linux, macOS and Windows amd64/arm64. It builds with `CGO_ENABLED=0`; no `joker_ffi` tag or C compiler is required. This corrects the feature gate introduced in v42.12.0.

## Usage

```sh
go get github.com/rcarmo/go-joker/v42@v42.12.1
CGO_ENABLED=0 go install github.com/rcarmo/go-joker/v42/cmd/joker@v42.12.1
```

Scripts can require `joker.ffi` and open an absolute `.so`, `.dylib` or `.dll` path. Fixed-signature bindings, pointers and reusable buffers are unchanged. Unsupported targets expose unavailable errors. Native libraries are only loaded when a script explicitly opens them.

The SDL fluid example builds without feature tags:

```sh
make sdl-fluid
make sdl-fluid-screenshot
```

It requires an SDL2 runtime matching the host architecture. The Go caller does not require cgo. README, namespace documentation and example instructions reflect the default feature.

## Validation and deployment

Release verification now requires every published binary to report `CGO_ENABLED=0` and contain the purego backend. A profiled smoke script invokes a real C ABI function through the normal CLI; CI tests both a downloaded release artifact and the installed Go-module CLI. Default no-cgo Go suites, FFI/solver tests, race checks, docs/browser gates and release-platform builds are included in validation.

Linux binaries use the system dynamic loader/libc. SDL and curl are not universal dependencies: only scripts using them need those runtimes. FFI permits arbitrary native code and remains intended for trusted scripts; it is not a sandbox. Callbacks, C variadics, by-value structs and library unloading remain unsupported. See [FFI contracts](FFI.md).

All prior tags remain unchanged. This release also includes the release-artifact filtering correction made after v42.12.0, keeping profiling artifacts separate from binaries/SBOMs.
