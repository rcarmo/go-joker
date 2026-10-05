# FFI and SDL fluid validation -- 2026-10-05

The opt-in `joker.ffi` namespace and SDL fluid example passed native Linux amd64 validation with Go 1.26.5, purego v0.11.1 and the system SDL2 runtime. The image in `docs/images/sdl-fluid.png` is SDL framebuffer readback after 240 steps under Xvfb.

## Checks

* Profiled default full suite, pretag with browser smoke, and repository race target passed.
* Profiled tagged full suite (`joker_ffi`) and focused FFI/solver race tests passed.
* Disabled namespace exposes documented vars and throws unavailable errors. A `CGO_ENABLED=0` default CLI remained static and its dependency list excluded purego.
* Tagged CLI and fluid host cross-built with cgo disabled for Linux/macOS/Windows amd64/arm64. Untagged Linux/386 cross-build passed. Only Linux amd64 executed the new FFI/SDL sample; macOS/Windows/ARM execution is not established by these builds.
* Tests cover fixed integer/double calls, uint64 BigInt round-trip, NULL, synchronous buffers, bounds/overflow, invalid symbols/signatures, function identity/map keys and one native counter invocation before interpreter arithmetic overflow. This counter test does not prove every speculative execution tier.
* Independent targeted review found returned-pointer alias lifetime, pointer equality and partial SDL initialisation teardown bugs. Returned pointers now retain input owners transitively and compare/hash by native address; the script encloses setup in cleanup. Regression tests and screenshot rerun passed after changes.

Several initial FFI test compilations failed during API development, producing no profiles; the profiling runner reported these failures. One full run and render rebuild failed because imported artifacts disappeared from the shared Go build cache. Those failed logs remain in `.cache/ffi-validation`; final runs used a repository-local `GOCACHE` and passed. Failed attempts were not counted as validation.

## Performance/allocation review

The solver benchmark measures `Step` plus RGBA conversion after warmup. Six 10-iteration samples recorded zero bytes and zero allocations per step. Timings were instrumented and noisy, so no frame-time speedup is claimed. The solver profile attributes about 85% of CPU to iterative relaxation; persistent velocity/dye/scratch buffers avoid per-step heap growth. Removing relaxation passes would change the simulation and was not used as an optimization.

The full screenshot run sampled about 12 MB allocated over startup, 240 steps and PNG writing. The largest owned allocations were the persistent framebuffer (about 2.3 MB) and solver arrays (about 1.1 MB). Generated metadata, parser/interpreter objects, profiling buffers and PNG compression contribute the rest. FFI bindings cache symbols/signatures; reflection/argument boxing still has per-call overhead. Uploading one pixel buffer per frame avoids per-pixel FFI/Joker object construction. Native SDL allocations are outside Go heap measurement.

Focused ABI tests were too short for useful C-call CPU attribution; allocation profiles mostly show startup, fixture/test infrastructure and profiling compression. No speculative pooling of `reflect.Value` arrays or unsafe ownership bypass was added. A repeated fixed-call benchmark is needed before that optimization.

Solver acceptance checks finite/bounded fields, dye/output ranges and approximate reference statistics. Repeated renderings use a one-level maximum RGBA error and half-level RMS bound, reflecting byte quantisation; screenshot hashes or exact image bytes are not acceptance gates. The reference statistics are stored regression values from this implementation, not an independent fluid-physics oracle.

No push or release is part of this change. See [FFI contracts](FFI.md) and [sample instructions](../examples/graphics/sdl-fluid/README.md).
