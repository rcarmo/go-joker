# WASM interpretation and native compilation

Joker can turn eligible numeric functions and loops into WebAssembly, then execute that WASM using wazero's interpreter or native compiler. Both engines run the same module format without cgo or an external C compiler.

| Engine | Execution |
| --- | --- |
| `interpreter` | Interprets WASM instructions |
| `compiler` | Compiles the module into host machine code and executes it |
| `auto` | Uses the compiler when available, otherwise the interpreter |

The engine flag selects how WASM executes. It does not force every Joker expression through WASM. Select the engine before the first WASM use:

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

## Compile a numeric function

Use `joker.jit` to request WASM compilation explicitly:

```clojure
(require '[joker.jit :as jit])

(def sum
  (jit/compile-wasm
    (fn [n]
      (loop [i 0 total 0]
        (if (< i n)
          (recur (+ i 1) (+ total i))
          total)))))

(println (jit/wasm-engine)) ;; "compiler" or "interpreter", actual selection
(println (sum 100))        ;; 4950
```

The runnable version is [`examples/wasm/native-sum.joke`](../examples/wasm/native-sum.joke).

## What can compile

The current emitter supports a bounded numeric IR subset: supported arithmetic, comparisons, branches and loop/recur shapes. Integer modules use `i64`; floating-point modules use `f64`.

Arbitrary object, string, collection and callback operations are not generally WASM-compilable. `jit/compile-wasm` reports an eligibility error for unsupported shapes. Existing overflow, exactness and type-shape safeguards remain in force; restricted cases recover through IR rather than accepting incorrect WASM arithmetic. Selecting the compiler does not relax these contracts.

Export an eligible function as a numeric module with an `exec` export:

```clojure
(jit/export-wasm (fn [x] (+ x 1)) "increment.wasm")
```

Both engine modes execute that WASM format. The `joker.wasm` namespace contains encoding helpers; Joker does not currently provide a general WASI application runner.

## Checked dense numeric buffers

`jit/compile-wasm` accepts `{:buffers [argument-indices]}` for checked f64 kernels with nested `loop`/`recur`, `let`, comparisons, branches, arithmetic and `joker.math/sqrt`, `abs` and `floor`. Modules have no host imports.

```clojure
(require '[joker.jit :as jit])
(def values (jit/numeric-buffer 96))
(def fill! (jit/compile-wasm
  (fn [a] (loop [i 0]
    (if (< i 96)
      (do (jit/buffer-set! a i (* i 0.25)) (recur (+ i 1)))
      (jit/buffer-get a 95))))
  {:buffers [0]}))
(println (fill! values)) ; 23.75
```

Dense kernels use IEEE-754 f64 arithmetic and return `Double`. Scalar arguments accept `Int` or `Double`; integers outside +/-2^53 are rejected before execution. Indices must be finite, integral and in bounds. Conditions preserve Joker truthiness, including numeric zero. Ambiguous mixed-type conditions, type-changing recurrences, captured locals, arbitrary calls and using buffers as scalars are rejected. Without options, the existing scalar compiler is unchanged.

Each invocation copies unique buffers into and out of linear memory. Aliases share their region; kernel and buffer locks serialise mutation in stable identity order. Writes completed before a trap are copied back without replay. Rebinding a primitive invalidates the compiled call before it writes memory.

The [mixed SDL/FFI/WASM fluid example](../examples/graphics/sdl-wasm-fluid/README.md) defines pressure projection, vorticity confinement, advection and colour conversion in Joker. Its screenshot is actual SDL readback with the compiler engine selected. SDL calls remain outside WASM.

## Verification

Tests instantiate an identical standalone module in both engine configurations and assert actual selection plus its independent expected result. Separate JIT processes verify an emitted numeric-loop module returns `4950` in each mode. CLI integration builds/runs a saved compiler-mode executable, deletes the source, runs it in both modes and checks environment overrides.

Native compilation is supported by wazero on amd64/arm64 where the host allows executable memory. Other architectures use interpreter mode in `auto`; explicit compiler mode errors. Cross-builds establish compilation only, not native execution. CPU/heap diagnostics are captured for release checks, analysed and disposed immediately after use.

The implementation was verified on Linux amd64 with Go 1.27.1 and wazero v1.12.0: core/JIT/CLI tests passed separately under both engine selections; the full release gate, browser smoke and race suite passed. Linux/386 interpreter execution and explicit compiler rejection passed; all six no-cgo release targets cross-built. Native arm64/macOS/Windows execution was not performed in this pass.

Profile review found runtime.Stack/goroutine-state lookup still dominating the core concurrency suite; vector construction and IR execution still lead allocations. Module/compiler setup is initialized once, not switched on each call. Short captures and subprocess tests are not throughput evidence, and no performance improvement is claimed. Raw captures, matching binaries and disposable logs were deleted after review; concise conclusions remain.
