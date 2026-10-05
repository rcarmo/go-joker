//go:build joker_ffi && (linux || darwin) && (amd64 || arm64)

package ffi

import "github.com/ebitengine/purego"

func openLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_LOCAL)
}
func symbol(handle uintptr, name string) (uintptr, error) { return purego.Dlsym(handle, name) }
