package ir

import "math"

const (
	nbQuiet  uint64 = 0x7FF8_0000_0000_0000
	nbTagInt uint64 = 0x7FF8_0001_0000_0000
	nbTagBol uint64 = 0x7FF8_0002_0000_0000
	nbTagNil uint64 = 0x7FF8_0003_0000_0000
	nbTagObj uint64 = 0x7FF8_0004_0000_0000
	nbMask32 uint64 = 0x0000_0000_FFFF_FFFF
)

func BoxDouble(f float64) uint64 { return math.Float64bits(f) }
func BoxInt(i int) uint64        { return nbTagInt | uint64(uint32(i)) }
func BoxBool(b bool) uint64 {
	if b {
		return nbTagBol | 1
	}
	return nbTagBol
}
func BoxNil() uint64        { return nbTagNil }
func BoxObj(idx int) uint64 { return nbTagObj | uint64(uint32(idx)) }

func IsDouble(v uint64) bool { return (v & nbQuiet) != nbQuiet }
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
