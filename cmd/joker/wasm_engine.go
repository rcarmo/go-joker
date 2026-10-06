package main

import (
	"fmt"
	"os"
	"strings"

	corewasm "github.com/rcarmo/go-joker/v42/core/wasm"
)

// Engine options belong to the runtime, not the embedded script's arguments.
func extractWasmEngine(args []string) ([]string, string, error) {
	result := make([]string, 0, len(args))
	mode := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			result = append(result, args[i:]...)
			break
		}
		if arg == "--wasm-engine" {
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("--wasm-engine requires auto, interpreter or compiler")
			}
			i++
			mode = args[i]
		} else if strings.HasPrefix(arg, "--wasm-engine=") {
			mode = strings.TrimPrefix(arg, "--wasm-engine=")
			if mode == "" {
				return nil, "", fmt.Errorf("--wasm-engine requires a value")
			}
		} else {
			result = append(result, arg)
		}
	}
	if mode != "" {
		var err error
		mode, err = corewasm.NormalizeEngine(mode)
		if err != nil {
			return nil, "", err
		}
	}
	return result, mode, nil
}
func applyWasmEngine(mode string) error {
	if mode == "" {
		mode = os.Getenv("JOKER_WASM_ENGINE")
	}
	normalized, err := corewasm.NormalizeEngine(mode)
	if err != nil {
		return err
	}
	return os.Setenv("JOKER_WASM_ENGINE", normalized)
}
