package core

import "math"

// NaN-boxing: encode all values in a single uint64.
// Doubles: raw IEEE 754 bits (any non-quiet-NaN)
// Int:     0x7FF8_0001_XXXX_XXXX (quiet NaN + tag + signed 32-bit)
// Bool:    0x7FF8_0002_0000_000X (0 or 1)
// Nil:     0x7FF8_0003_0000_0000
// Object:  0x7FF8_0004_XXXX_XXXX (index into side-table)

const (
	nbQuiet  uint64 = 0x7FF8_0000_0000_0000
	nbTagInt uint64 = 0x7FF8_0001_0000_0000
	nbTagBol uint64 = 0x7FF8_0002_0000_0000
	nbTagNil uint64 = 0x7FF8_0003_0000_0000
	nbTagObj uint64 = 0x7FF8_0004_0000_0000
	nbMask32 uint64 = 0x0000_0000_FFFF_FFFF
)

func nbDouble(f float64) uint64 { return math.Float64bits(f) }
func nbInt(i int) uint64        { return nbTagInt | uint64(uint32(i)) }
func nbBool(b bool) uint64 {
	if b {
		return nbTagBol | 1
	}
	return nbTagBol
}
func nbNil() uint64        { return nbTagNil }
func nbObj(idx int) uint64 { return nbTagObj | uint64(uint32(idx)) }

func nbIsDouble(v uint64) bool { return (v & nbQuiet) != nbQuiet }
func nbIsInt(v uint64) bool    { return (v & 0xFFFF_FFFF_0000_0000) == nbTagInt }
func nbIsBool(v uint64) bool   { return (v & 0xFFFF_FFFF_0000_0000) == nbTagBol }
func nbIsNil(v uint64) bool    { return v == nbTagNil }
func nbIsObj(v uint64) bool    { return (v & 0xFFFF_FFFF_0000_0000) == nbTagObj }

func nbToDouble(v uint64) float64 { return math.Float64frombits(v) }
func nbToInt(v uint64) int        { return int(int32(v & nbMask32)) }
func nbToBool(v uint64) bool      { return (v & 1) != 0 }
func nbToObjIdx(v uint64) int     { return int(v & nbMask32) }

func nbTruthy(v uint64) bool {
	if nbIsNil(v) {
		return false
	}
	if nbIsBool(v) {
		return nbToBool(v)
	}
	return true
}

func nbToFloat(v uint64) float64 {
	if nbIsDouble(v) {
		return nbToDouble(v)
	}
	if nbIsInt(v) {
		return float64(nbToInt(v))
	}
	return 0
}

func nbFromObject(obj Object, table *[]Object) uint64 {
	switch v := obj.(type) {
	case Int:
		return nbInt(v.I)
	case Double:
		return nbDouble(v.D)
	case Boolean:
		return nbBool(v.B)
	case Nil:
		return nbNil()
	default:
		idx := len(*table)
		*table = append(*table, obj)
		return nbObj(idx)
	}
}

func nbToObject(v uint64, table []Object) Object {
	if nbIsDouble(v) {
		return Double{D: nbToDouble(v)}
	}
	if nbIsInt(v) {
		return Int{I: nbToInt(v)}
	}
	if nbIsBool(v) {
		return Boolean{B: nbToBool(v)}
	}
	if nbIsNil(v) {
		return NIL
	}
	if nbIsObj(v) {
		idx := nbToObjIdx(v)
		if idx < len(table) {
			return table[idx]
		}
	}
	return NIL
}
