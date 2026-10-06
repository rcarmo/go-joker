package jit

import (
	core "github.com/rcarmo/go-joker/v42/core"
	types "github.com/rcarmo/go-joker/v42/core/types"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

func TestJITWASMEngines(t *testing.T) {
	if mode := os.Getenv("JOKER_TEST_WASM_ENGINE"); mode != "" {
		if mode == "compiler" && runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
			t.Skip("explicit compiler unsupported on this architecture")
		}
		fn := mkFn("(fn [x] (loop [i 0 acc 0] (if (< i x) (recur (+ i 1) (+ acc i)) acc)))")
		result := compileWASM(fn).(types.Callable).Call([]types.Object{types.MakeInt(100)})
		if n, ok := result.(types.Int); !ok || n.I != 4950 {
			t.Fatalf("%s: %#v", mode, result)
		}
		if got := core.WasmEngineExported(); got != mode {
			t.Fatalf("%s selected %s", mode, got)
		}
		return
	}
	for _, mode := range []string{"interpreter", "compiler"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestJITWASMEngines$")
			cmd.Env = append(os.Environ(), "JOKER_WASM_ENGINE="+mode, "JOKER_TEST_WASM_ENGINE="+mode)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s: %v\n%s", mode, err, out)
			}
		})
	}
}
