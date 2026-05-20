package types

import (
	"math/big"

	"github.com/rcarmo/go-joker/core/types/numerical"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

type Double struct{ D float64 }
type Int struct{ I int }

func MakeDouble(d float64) Double { return Double{D: d} }
func MakeInt(i int) Int           { return Int{I: i} }

func (d Double) GetInfo() *ObjectInfo        { return nil }
func (d Double) ToString(escape bool) string { return numerical.DoubleString(d.D) }
func (d Double) Equals(other interface{}) bool {
	o, ok := numerical.NumericFloat(other)
	return ok && d.D == o
}
func (d Double) GetType() *Type      { return RuntimeTypes.Double }
func (d Double) Native() interface{} { return d.D }
func (d Double) Hash() uint32        { return numerical.FloatHash64(d.D) }
func (d Double) Compare(other Object) int {
	o, _ := numerical.NumericFloat(other)
	return numerical.CmpFloat(d.D, o)
}
func (d Double) Int() Int             { return Int{I: int(d.D)} }
func (d Double) BigInt() *big.Int     { return big.NewInt(int64(d.D)) }
func (d Double) Double() Double       { return d }
func (d Double) BigFloat() *big.Float { return big.NewFloat(float64(d.D)) }
func (d Double) Ratio() *big.Rat      { res := big.Rat{}; return res.SetFloat64(float64(d.D)) }

func (i Int) GetInfo() *ObjectInfo        { return nil }
func (i Int) ToString(escape bool) string { return corestr.Int(i.I) }
func (i Int) Equals(other interface{}) bool {
	o, ok := numerical.NumericFloat(other)
	return ok && float64(i.I) == o
}
func (i Int) GetType() *Type      { return RuntimeTypes.Int }
func (i Int) Native() interface{} { return i.I }
func (i Int) Hash() uint32        { return numerical.IntHash64(i.I) }
func (i Int) Compare(other Object) int {
	o, _ := numerical.NumericFloat(other)
	return numerical.CmpFloat(float64(i.I), o)
}
func (i Int) Int() Int             { return i }
func (i Int) Double() Double       { return Double{D: float64(i.I)} }
func (i Int) BigInt() *big.Int     { return big.NewInt(int64(i.I)) }
func (i Int) BigFloat() *big.Float { return big.NewFloat(float64(i.I)) }
func (i Int) Ratio() *big.Rat      { return big.NewRat(int64(i.I), 1) }

func (i Int) IntValue() int           { return i.I }
func (d Double) DoubleValue() float64 { return d.D }
