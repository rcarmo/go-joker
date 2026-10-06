# WASM interpretation and native compilation

Joker's WASM backend can execute the same module using wazero's interpreter or native compiler. Select the engine before the first WASM use:

```sh
joker --wasm-engine=interpreter examples/wasm/native-sum.joke
joker --wasm-engine=compiler examples/wasm/native-sum.joke
```

`JOKER_WASM_ENGINE=auto|interpreter|compiler` provides the same selection for embedded Go users and scripts launched without CLI flags. `native` is an alias for `compiler`. The default `auto` tries the native compiler and falls back to the interpreter when platform/executable-memory support is unavailable. Explicit `compiler` returns an error instead of falling back. Invalid modes are rejected.

The shared runtime is initialized once; set the mode before compiling/executing a WASM function. A small executable module is compiled to probe native compilation availability. Interpreter mode uses `wazero.NewRuntimeConfigInterpreter`; compiler mode uses `wazero.NewRuntimeConfigCompiler`. The compiler emits host machine code during module compilation; it does not invoke an external C compiler or require cgo.

## One-command executable workflow

```sh
joker compile --native --run examples/wasm/native-sum.joke -o ./native-sum
./native-sum
./native-sum --wasm-engine=interpreter
```

The example explicitly uses `jit/compile-wasm` to compile its integer loop. Both modes return `4950` for `sum(100)` and print their actual engine.

`compile --run` builds and executes immediately; arguments after `--` are passed to the resulting program. `--native` embeds compiler selection. `--wasm-engine=interpreter` or `auto` can be embedded instead. Runtime CLI engine flags override the embedded choice; an explicit environment variable also overrides it. Without any saved selection, `auto` is the default. Existing source-only standalone payloads still load.

Packaging streams the existing runtime into a temporary output and appends the source/engine metadata, avoiding a runtime-sized Go buffer. The temporary file is closed and made executable before replacing the destination; build errors preserve an existing destination. Source and running-executable overwrite attempts are rejected.

The generated executable contains the native Joker runtime and bundled source. Eligible functions run as native helpers/IR or WASM according to their supported shape; arbitrary unsupported forms still use the interpreter. This is **not whole-program ahead-of-time compilation of Joker**. The standalone format embeds source/engine metadata, not portable persisted machine code. It targets the platform of the Joker binary used to build it. macOS signed executables may need re-signing after source is appended.

## Script diagnostics

```clojure
(require '[joker.jit :as jit])
(println (jit/wasm-engine)) ;; "compiler" or "interpreter", actual selection
(def sum (jit/compile-wasm
           (fn [n]
             (loop [i 0 total 0]
               (if (< i n)
                 (recur (+ i 1) (+ total i))
                 total)))))
(sum 100)
```

The current WASM emitter supports a bounded numeric IR surface. `jit/compile-wasm` reports eligibility errors. Existing overflow/exactness/type-shape safeguards and restricted IR recovery remain in force; selecting the compiler does not relax numeric contracts. `jit/export-wasm` exports the module bytes; both engine modes execute that WASM format. The `joker.wasm` namespace still contains encoding helpers, not a general WASI/program loader.

## Verification

Tests instantiate an identical standalone module in both engine configurations and assert actual selection plus its independent expected result. Separate JIT processes verify an emitted numeric-loop module returns `4950` in each mode. CLI integration builds/runs a saved compiler-mode executable, deletes the source, runs it in both modes and checks environment overrides.

Native compilation is supported by wazero on amd64/arm64 where the host allows executable memory. Other architectures use interpreter mode in `auto`; explicit compiler mode errors. Cross-builds establish compilation only, not native execution. CPU/heap diagnostics are captured for release checks, analysed and disposed immediately after use.

The implementation was verified on Linux amd64 with Go 1.27.1 and wazero v1.12.0: core/JIT/CLI tests passed separately under both engine selections; the full release gate, browser smoke and race suite passed. Linux/386 interpreter execution and explicit compiler rejection passed; all six no-cgo release targets cross-built. Native arm64/macOS/Windows execution was not performed in this pass.

Profile review found runtime.Stack/goroutine-state lookup still dominating the core concurrency suite; vector construction and IR execution still lead allocations. Module/compiler setup is initialized once, not switched on each call. Short captures and subprocess tests are not throughput evidence, and no performance improvement is claimed. Raw captures, matching binaries and disposable logs were deleted after review; concise conclusions remain.
