package reader

import "math"

// SymbolicValue resolves symbolic reader values such as ##Inf without owning
// root Object construction or Symbol handling.
func SymbolicValue(name string) (float64, bool) {
	switch name {
	case "Inf":
		return math.Inf(1), true
	case "-Inf":
		return math.Inf(-1), true
	case "NaN":
		return math.NaN(), true
	default:
		return 0, false
	}
}
