package types

import (
	"io"
	"math/big"
	"regexp"
	"time"
)

func ExtractCallable(args []Object, index int) Callable { return EnsureArgIsCallable(args, index) }
func ExtractObject(args []Object, index int) Object     { return args[index] }
func ExtractString(args []Object, index int) string     { return EnsureArgIsString(args, index).S }
func ExtractKeyword(args []Object, index int) string {
	return EnsureArgIsKeyword(args, index).ToString(false)
}
func ExtractStringable(args []Object, index int) string { return EnsureArgIsStringable(args, index).S }

func ExtractStrings(args []Object, index int) []string {
	strs := make([]string, 0, len(args)-index)
	for i := index; i < len(args); i++ {
		strs = append(strs, EnsureArgIsString(args, i).S)
	}
	return strs
}

func ExtractInt(args []Object, index int) int { return EnsureArgIsInt(args, index).I }
func ExtractInteger(args []Object, index int) int {
	return EnsureArgIsNumber(args, index).Int().I
}
func ExtractBoolean(args []Object, index int) bool    { return EnsureArgIsBoolean(args, index).B }
func ExtractChar(args []Object, index int) rune       { return EnsureArgIsChar(args, index).Ch }
func ExtractTime(args []Object, index int) time.Time  { return EnsureArgIsTime(args, index).T }
func ExtractDouble(args []Object, index int) float64  { return EnsureArgIsDouble(args, index).D }
func ExtractNumber(args []Object, index int) Number   { return EnsureArgIsNumber(args, index) }
func ExtractBigInt(args []Object, index int) *big.Int { return EnsureArgIsBigInt(args, index).B }
func ExtractBigFloat(args []Object, index int) *big.Float {
	return EnsureArgIsBigFloat(args, index).B
}
func ExtractRegex(args []Object, index int) *regexp.Regexp { return EnsureArgIsRegex(args, index).R }
func ExtractSeqable(args []Object, index int) Seqable      { return EnsureArgIsSeqable(args, index) }
func ExtractMap(args []Object, index int) Map              { return EnsureArgIsMap(args, index) }
func ExtractIOReader(args []Object, index int) io.Reader   { return EnsureArgIsio_Reader(args, index) }
func ExtractIOWriter(args []Object, index int) io.Writer   { return EnsureArgIsio_Writer(args, index) }
