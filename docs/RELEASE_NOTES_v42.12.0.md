# Release Notes -- v42.12.0

This minor release adds the experimental `joker.ffi` namespace, an SDL2 fluid-simulation example and mandatory profiling for Go test targets.

## Optional C ABI calls

`joker.ffi` binds fixed C signatures to libraries loaded from absolute `.so`, `.dylib` or `.dll` paths. Supported types include fixed-width integers, floats, call-scoped UTF-8 strings, opaque pointers and reusable byte buffers. Bound functions have distinct identity; returned pointers retain input owners conservatively. Libraries stay loaded for process lifetime.

Native calling uses pinned purego v0.11.1 and builds with cgo disabled:

```sh
CGO_ENABLED=0 make ffi-cli
.cache/tmp/joker-ffi doc joker.ffi
```

The default CLI and published binaries keep native calls disabled and expose documentation plus unavailable errors. This preserves the default no-cgo static deployment model; enabling FFI on Linux adds system loader/libc dependencies. FFI is arbitrary native execution for trusted scripts, with no sandbox. Callbacks, C variadics, by-value structs and unloading are not supported. See [FFI contracts](FFI.md).

## SDL fluid sample

The [SDL2 example](../examples/graphics/sdl-fluid/README.md) declares all SDL calls in Joker through `joker.ffi`. A companion Go host provides the stable-fluid solver and owned buffers. The script controls the window, texture upload, rendering, events, framebuffer capture and cleanup on the process main thread.

```sh
make sdl-fluid-screenshot
```

![SDL fluid simulation](images/sdl-fluid.png)

The checked-in screenshot is actual SDL readback after 240 fixed simulation steps under Xvfb. Native Linux amd64 is tested; the tagged CLI and example cross-build for all six release platforms, but macOS/Windows/ARM execution of this sample is not yet qualified. The solver reuses persistent arrays and records zero steady-state allocations in the focused step/RGBA benchmark. Its iterative relaxation remains the main CPU cost.

## Go profiling and allocation changes

Go test Makefile targets, CI benchmarks and nested module-consumer tests capture per-package CPU/heap profiles, binaries, commands and logs. Post-run reports include cumulative CPU, allocated bytes and allocated objects. Python checks are unchanged. See [first profiling pass](GO_TEST_PROFILING_2026-10-05.md).

The first full-suite profiles identified escaping goroutine-ID stack-header buffers. Reusing a private buffer pool reduced measured GoID and registered interpreter-state lookups from 64 B/op and one allocation to zero bytes/allocations in matched focused runs. `runtime.Stack` is still a CPU hotspot; no general application-speed claim accompanies this change.

## Validation

Release validation includes profiled default/tagged suites, pretag with browser smoke, race checks, namespace/generated/layout guards, external Go consumers and all-platform cross-builds. Independent FFI review led to pointer-alias lifetime, native-pointer equality and partial SDL initialization cleanup fixes. Image/solver acceptance uses numeric and quantisation tolerances, not screenshot hashes.

Published binaries, SPDX SBOMs, checksums and provenance remain the default build. Enable FFI from source; no FFI-enabled binary variant is attached in this release. Go consumers use `github.com/rcarmo/go-joker/v42@v42.12.0`.
