# Release Notes - v42.12.3

This patch adds an opt-in dense numeric ABI to `joker.jit/compile-wasm` and a mixed SDL/FFI/compiled-WASM fluid example. The existing scalar compiler remains unchanged when called without options.

## Dense numeric kernels

* `jit/numeric-buffer`, `buffer-get` and `buffer-set!` provide checked f64 storage. `compile-wasm` accepts `{:buffers [argument-indices]}` for nested loops, branches, arithmetic, comparisons, square root, absolute value and floor.
* Dense kernels use IEEE-754 f64 arithmetic and return `Double`; scalar integers outside +/-2^53 are rejected. Conditions preserve Joker truthiness, including numeric zero. Unsupported shapes fail before execution.
* Buffer aliases share linear-memory regions. Kernels serialise buffer mutation with stable lock ordering. Index traps preserve completed writes without replay, and changed primitive bindings invalidate compiled calls.
* A reusable wazero call stack eliminates the result-slice allocation. A 9216-element read/add/store benchmark changed from 16 B/op and one allocation/op to zero for both, across six equivalent samples. Timing was noisy and slower in the second set; there is no execution-speed improvement claim.

## Mixed SDL/FFI/WASM example

[`examples/graphics/sdl-wasm-fluid`](../examples/graphics/sdl-wasm-fluid/README.md) defines pressure projection, vorticity confinement, advection, obstacle handling and colour conversion in Joker, compiled with `jit/compile-wasm`. Wazero's compiler engine executes the kernels; Joker calls SDL2 through FFI for display and events. The example includes actual SDL framebuffer readback after 300 simulation steps.

Build with `make sdl-wasm-fluid`; capture and profile with `make sdl-wasm-fluid-screenshot`. The host needs a matching SDL2 runtime library and builds with `CGO_ENABLED=0`. Native SDL execution is qualified on Linux amd64; native macOS/Windows execution is unavailable here. The separate original SDL example is preserved.

## Generation and documentation

The std generator accepts `JOKER_STD_NAMESPACE` for narrow regeneration and uses extracted object/symbol APIs for the JIT bindings. API HTML, the WASM guide and example documentation describe the new opt-in ABI. See [WASM execution](WASM_EXECUTION.md) for numerical and execution limits.

## Verification

Validation uses the latest stable Go 1.27.1, pinned consistently in the module, Makefile and CI. The full profiled pre-tag gate, notebook browser smoke, repository race suite and no-cgo FFI checks passed on Linux amd64. Both WASM engines passed the dense-buffer checks and a tree-walker fluid comparison: maximum absolute field error 4.44e-16, zero colour differences for the tested two-step workload. A conservative f64 tolerance of 1e-10 absolute plus 1e-9 relative covers arithmetic rounding; colour comparison allows one quantisation level per channel. The tests also check longer-run finite/bounded fields, aliasing, trap side effects, rebinding and concurrent access.

Linux/386 interpreter checks passed; native WASM compilation is unavailable on that architecture. All six release platforms cross-built with `CGO_ENABLED=0`; cross-builds are not native execution coverage. CPU, alloc_space and alloc_objects were analysed for tests and the 300-frame render, and raw captures were removed after review. The independent delegated review timed out and supplied no sign-off.
