//go:build windows && (amd64 || arm64)

package ffi

import "golang.org/x/sys/windows"

func openLibrary(path string) (uintptr, error) {
	h, e := windows.LoadLibraryEx(path, 0, windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR|windows.LOAD_LIBRARY_SEARCH_DEFAULT_DIRS)
	return uintptr(h), e
}
func symbol(handle uintptr, name string) (uintptr, error) {
	return windows.GetProcAddress(windows.Handle(handle), name)
}
