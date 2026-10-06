# Release Notes -- v42.12.2

This patch adds explicit WASM interpreter/native-compiler selection and an easier standalone script build/run workflow. It also reduces standalone packaging allocations and updates the developer, tracing, profiling and release guides.

## WASM engines and executable scripts

```sh
joker --wasm-engine=interpreter examples/wasm/native-sum.joke
joker --wasm-engine=compiler examples/wasm/native-sum.joke
joker compile --native --run examples/wasm/native-sum.joke -o native-sum
```

`auto` prefers the native compiler where platform/executable-memory support permits it, then uses the interpreter. Explicit `compiler` never silently falls back; `native` is an alias. `JOKER_WASM_ENGINE` provides the same selection for embedded hosts. `jit/wasm-engine` reports the actual engine. Select it before the shared WASM runtime is initialized.

`compile --native` saves compiler mode in the executable; `--run` executes it immediately and forwards script arguments after `--`. Runtime CLI flags and the environment can override the saved mode. Bundled programs receive arguments named `doc`, `compile` and `notebook` as script arguments. Existing source-only payloads remain readable.

The executable bundles the native Joker runtime and source. Eligible WASM functions compile to machine code at runtime; arbitrary Joker forms are not whole-program ahead-of-time compiled. See [WASM execution](WASM_EXECUTION.md).

## Packaging allocation reduction

Packaging now reads the existing footer and streams the runtime to a temporary output. Reusing that footer avoids extra per-build allocations. An output is closed and made executable before replacing the destination; build failures preserve an existing target, and source/running-executable overwrite attempts are rejected.

Six matched two-build samples on Go 1.27.1/Linux amd64 reduced packaging from about 46 MB to 7–9 KiB of Go allocation per operation (about 99.98% fewer bytes). Allocation counts were unchanged or lower. Timings were instrumented and noisy, so no general execution-speed claim accompanies this result. See [profiling findings](GO_TEST_PROFILING_2026-10-05.md).

## Profiling lifecycle

Pre-release Go checks capture CPU/heap profiles for hotspot analysis and tuning. Ordinary development tests need not profile. Raw captures, matching binaries and completed logs/scratch are removed immediately after use; only compact conclusions persist. CI no longer uploads raw profiling artifacts.

## Validation and compatibility

Identical WASM executes in both explicit engine modes; independent numeric-loop tests and standalone CLI tests verify results and actual selection. Default FFI remains enabled without cgo. Linux amd64 execution and Linux/386 interpretation/compiler-rejection checks pass; all six release targets cross-build. Other platforms need native execution qualification. Full pre-tag/browser/race, module-consumer and generated/documentation guards are required before publication.

Go consumers use `github.com/rcarmo/go-joker/v42@v42.12.2`. Existing tags are unchanged.
