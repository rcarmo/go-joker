package wasm

import (
	"bytes"
	"context"
	"github.com/tetratelabs/wazero"
	"runtime"
	"testing"
)

func TestWASMInterpreterAndNativeCompiler(t *testing.T) {
	ctx := context.Background()
	// Identical standalone module: exported answer() -> i64 42.
	module := []byte{0, 97, 115, 109, 1, 0, 0, 0, 1, 5, 1, 96, 0, 1, 126, 3, 2, 1, 0, 7, 10, 1, 6, 'a', 'n', 's', 'w', 'e', 'r', 0, 0, 10, 6, 1, 4, 0, 66, 42, 11}
	for _, mode := range []string{"interpreter", "compiler"} {
		t.Run(mode, func(t *testing.T) {
			rt, selected, err := NewRuntime(ctx, mode)
			if err != nil {
				if mode == "compiler" && runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
					t.Logf("unsupported architecture rejects compiler explicitly: %v", err)
					return
				}
				t.Fatal(err)
			}
			defer rt.Close(ctx)
			if selected != mode {
				t.Fatalf("silent engine fallback: want %s, got %s", mode, selected)
			}
			saved := append([]byte(nil), module...)
			compiled, err := rt.CompileModule(ctx, module)
			if err != nil {
				t.Fatal(err)
			}
			defer compiled.Close(ctx)
			instance, err := rt.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithName(mode))
			if err != nil {
				t.Fatal(err)
			}
			defer instance.Close(ctx)
			value, err := instance.ExportedFunction("answer").Call(ctx)
			if err != nil || len(value) != 1 || value[0] != 42 {
				t.Fatalf("answer: %v, %v", value, err)
			}
			if !bytes.Equal(module, saved) {
				t.Fatal("compilation modified input module")
			}
		})
	}
}
func TestWASMEngineValidation(t *testing.T) {
	for _, mode := range []string{"wrong", "off"} {
		if rt, _, err := NewRuntime(context.Background(), mode); err == nil {
			rt.Close(context.Background())
			t.Fatalf("accepted %q", mode)
		}
	}
	if mode, err := NormalizeEngine("native"); err != nil || mode != "compiler" {
		t.Fatal(mode, err)
	}
	rt, actual, err := NewRuntime(context.Background(), "auto")
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(context.Background())
	if actual != "compiler" && actual != "interpreter" {
		t.Fatal(actual)
	}
}
