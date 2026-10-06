package wasm

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/tetratelabs/wazero"
)

// NormalizeEngine accepts explicit execution choices; compiler never silently
// falls back to the interpreter. auto prefers compilation where it is usable.
func NormalizeEngine(mode string) (string, error) {
	switch strings.ToLower(mode) {
	case "", "auto":
		return "auto", nil
	case "interpreter":
		return "interpreter", nil
	case "compiler", "native":
		return "compiler", nil
	default:
		return "", fmt.Errorf("invalid WASM engine %q (use auto, interpreter or compiler)", mode)
	}
}

// NewRuntime returns a configured runtime and the engine actually selected.
// Compile a minimal module to detect executable-memory/platform restrictions
// before auto selects the compiler. Explicit compiler requests report failure.
func NewRuntime(ctx context.Context, mode string) (rt wazero.Runtime, selected string, err error) {
	mode, err = NormalizeEngine(mode)
	if err != nil {
		return nil, "", err
	}
	if mode == "interpreter" {
		return wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter()), mode, nil
	}
	rt, err = newCompiler(ctx)
	if err == nil {
		return rt, "compiler", nil
	}
	if mode == "compiler" {
		return nil, "", err
	}
	return wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigInterpreter()), "interpreter", nil
}
func newCompiler(ctx context.Context) (rt wazero.Runtime, err error) {
	defer func() {
		if p := recover(); p != nil {
			if rt != nil {
				closeErr := rt.Close(ctx)
				rt = nil
				if closeErr != nil {
					p = fmt.Sprintf("%v (close failed: %v)", p, closeErr)
				}
			}
			err = fmt.Errorf("WASM native compiler unavailable on %s/%s: %v", runtime.GOOS, runtime.GOARCH, p)
		}
	}()
	// wazero's compiler supports these architectures. Avoid treating an explicit
	// compiler config as portable when its implementation only supports two ABIs.
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		return nil, fmt.Errorf("WASM native compiler unavailable on %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	rt = wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
	// Include actual executable code: an empty module may never allocate native
	// executable memory and would miss a restricted-mmap environment.
	probe, err := rt.CompileModule(ctx, []byte{0, 97, 115, 109, 1, 0, 0, 0, 1, 5, 1, 96, 0, 1, 126, 3, 2, 1, 0, 10, 6, 1, 4, 0, 66, 42, 11})
	if err != nil {
		if closeErr := rt.Close(ctx); closeErr != nil {
			return nil, fmt.Errorf("WASM native compiler unavailable: %w (close failed: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("WASM native compiler unavailable: %w", err)
	}
	if err := probe.Close(ctx); err != nil {
		if closeErr := rt.Close(ctx); closeErr != nil {
			return nil, fmt.Errorf("probe close: %v; runtime close: %v", err, closeErr)
		}
		return nil, fmt.Errorf("probe close: %w", err)
	}
	return rt, nil
}
