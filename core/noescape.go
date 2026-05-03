package core

import "unsafe"

// noescape64 hides a []float64 from escape analysis.
// The caller must ensure the slice is not retained by the callee.
//
//go:nosplit
func noescape64(s []float64) []float64 {
	p := unsafe.SliceData(s)
	np := (*float64)(noescape(unsafe.Pointer(p)))
	return unsafe.Slice(np, len(s))
}

// noescape hides a pointer from escape analysis.
//
//go:nosplit
//go:nocheckptr
func noescape(p unsafe.Pointer) unsafe.Pointer {
	x := uintptr(p)
	return unsafe.Pointer(x ^ 0 ^ 0)
}
