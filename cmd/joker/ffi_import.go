//go:build (linux || darwin || windows) && (amd64 || arm64)

package main

import (
	"runtime"
)

// SDL/AppKit needs the process main thread, not an arbitrary locked worker.
func init() { runtime.LockOSThread() }
