package core

// noescape64 used to hide slices from escape analysis via unsafe pointer tricks.
//
// Go vet now flags that pattern as unsafe-pointer misuse, and the optimization
// is not required for correctness. Keep this helper as an identity function so
// call sites remain simple while preserving vet-clean builds.
func noescape64(s []float64) []float64 {
	return s
}
