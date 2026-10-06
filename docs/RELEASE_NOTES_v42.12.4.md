# Release Notes - v42.12.4

This patch publishes the checked numeric WASM buffers and mixed SDL/FFI/compiled-WASM fluid example described in [v42.12.3](RELEASE_NOTES_v42.12.3.md).

The v42.12.3 tag is retained unchanged. Its release workflow stopped before compilation because the arithmetic documentation check ran before Bun was installed; no release assets were published. This patch installs Bun before the canonical release gate in both CI and the release workflow.

Builds and verification use the latest stable Go 1.27.1 through Makefile targets. CI parity and imported-subset checks now use the same targets as local execution. The full profiled pre-tag gate, browser smoke, race tests, Linux/386 interpreter checks, both WASM engines, no-cgo FFI smoke and six cross-builds passed locally for the underlying implementation. The corrected workflow ordering is covered by actionlint and the release contract checks.

The fluid example includes a compiler-engine SDL framebuffer screenshot after 300 steps. Its Joker kernels perform pressure projection, vorticity confinement, advection, obstacle handling and colour conversion; SDL calls use FFI outside WASM. See [the example](../examples/graphics/sdl-wasm-fluid/README.md) and [WASM execution](WASM_EXECUTION.md) for commands and numerical limits.
