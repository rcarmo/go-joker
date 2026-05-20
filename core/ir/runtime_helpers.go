package ir

// StableArgs clones args so returned closures don't retain mutable/shared backing arrays.
func StableArgs[T any](args []T) []T {
	if len(args) == 0 {
		return args
	}
	stable := make([]T, len(args))
	copy(stable, args)
	return stable
}

// Float64 used to hide slices from escape analysis via unsafe pointer tricks.
//
// Go vet flags that pattern as unsafe-pointer misuse, and the optimization is
// not required for correctness. Keep this helper as an identity function so
// call sites remain simple while preserving vet-clean builds.
func Float64(s []float64) []float64 {
	return s
}
