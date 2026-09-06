package ir

import (
	"math"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

const (
	nbQuiet  uint64 = 0x7FF8_0000_0000_0000
	nbTagInt uint64 = 0x7FF8_0001_0000_0000
	nbTagBol uint64 = 0x7FF8_0002_0000_0000
	nbTagNil uint64 = 0x7FF8_0003_0000_0000
	nbTagObj uint64 = 0x7FF8_0004_0000_0000
	nbMask32 uint64 = 0x0000_0000_FFFF_FFFF
)

func BoxDouble(f float64) uint64 {
	// Reserve one canonical NaN payload; other quiet-NaN payloads encode tags.
	if math.IsNaN(f) {
		return nbQuiet
	}
	return math.Float64bits(f)
}
func BoxInt(i int) uint64 { return nbTagInt | uint64(uint32(i)) }
func BoxBool(b bool) uint64 {
	if b {
		return nbTagBol | 1
	}
	return nbTagBol
}
func BoxNil() uint64        { return nbTagNil }
func BoxObj(idx int) uint64 { return nbTagObj | uint64(uint32(idx)) }

func IsDouble(v uint64) bool { return v == nbQuiet || (v&nbQuiet) != nbQuiet }
func IsInt(v uint64) bool    { return (v & 0xFFFF_FFFF_0000_0000) == nbTagInt }
func IsBool(v uint64) bool   { return (v & 0xFFFF_FFFF_0000_0000) == nbTagBol }
func IsNil(v uint64) bool    { return v == nbTagNil }
func IsObj(v uint64) bool    { return (v & 0xFFFF_FFFF_0000_0000) == nbTagObj }

func ToDouble(v uint64) float64 { return math.Float64frombits(v) }
func ToInt(v uint64) int        { return int(int32(v & nbMask32)) }
func ToBool(v uint64) bool      { return (v & 1) != 0 }
func ToObjIdx(v uint64) int     { return int(v & nbMask32) }

func Truthy(v uint64) bool {
	if IsNil(v) {
		return false
	}
	if IsBool(v) {
		return ToBool(v)
	}
	return true
}

func ToFloat(v uint64) float64 {
	if IsDouble(v) {
		return ToDouble(v)
	}
	if IsInt(v) {
		return float64(ToInt(v))
	}
	return 0
}

func NBFromObject(obj coretypes.Object, table *[]coretypes.Object, isNil func(coretypes.Object) bool) uint64 {
	switch v := obj.(type) {
	case coretypes.Int:
		if v.I >= math.MinInt32 && v.I <= math.MaxInt32 {
			return BoxInt(v.I)
		}
		idx := len(*table)
		*table = append(*table, obj)
		return BoxObj(idx)
	case coretypes.Double:
		return BoxDouble(v.D)
	case coretypes.Boolean:
		return BoxBool(v.B)
	default:
		if isNil != nil && isNil(obj) {
			return BoxNil()
		}
		idx := len(*table)
		*table = append(*table, obj)
		return BoxObj(idx)
	}
}

func NBToObject(v uint64, table []coretypes.Object, nilObj coretypes.Object) coretypes.Object {
	if IsDouble(v) {
		return coretypes.Double{D: ToDouble(v)}
	}
	if IsInt(v) {
		return coretypes.Int{I: ToInt(v)}
	}
	if IsBool(v) {
		return coretypes.Boolean{B: ToBool(v)}
	}
	if IsNil(v) {
		return nilObj
	}
	if IsObj(v) {
		idx := ToObjIdx(v)
		if idx < len(table) {
			return table[idx]
		}
	}
	return nilObj
}
