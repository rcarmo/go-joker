package core

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strconv"
	"time"
	"unicode/utf8"

	corert "github.com/rcarmo/go-joker/core/runtime"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	"github.com/rcarmo/go-joker/core/deps"
	coregenerated "github.com/rcarmo/go-joker/core/generated"
	"github.com/rcarmo/go-joker/core/osutil"
	corereader "github.com/rcarmo/go-joker/core/reader"
	"github.com/rcarmo/go-joker/core/types/numerical"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

const VERSION = "v42.8.2"

func ExtractCallable(args []coretypes.Object, index int) coretypes.Callable {
	return coretypes.EnsureArgIsCallable(args, index)
}

func ExtractObject(args []coretypes.Object, index int) coretypes.Object {
	return args[index]
}

func ExtractString(args []coretypes.Object, index int) string {
	return coretypes.EnsureArgIsString(args, index).S
}

func ExtractKeyword(args []coretypes.Object, index int) string {
	return coretypes.EnsureArgIsKeyword(args, index).ToString(false)
}

func ExtractStringable(args []coretypes.Object, index int) string {
	return coretypes.EnsureArgIsStringable(args, index).S
}

func ExtractStrings(args []coretypes.Object, index int) []string {
	strs := make([]string, 0)
	for i := index; i < len(args); i++ {
		strs = append(strs, coretypes.EnsureArgIsString(args, i).S)
	}
	return strs
}

func ExtractInt(args []coretypes.Object, index int) int {
	return coretypes.EnsureArgIsInt(args, index).I
}

func ExtractInteger(args []coretypes.Object, index int) int {
	switch c := args[index].(type) {
	case coretypes.Number:
		return c.Int().I
	default:
		panic(RT.NewArgTypeError(index, c, "coretypes.Number"))
	}
}

func ExtractBoolean(args []coretypes.Object, index int) bool {
	return coretypes.EnsureArgIsBoolean(args, index).B
}

func FailArg(obj coretypes.Object, typeName string, index int) *EvalError {
	return RT.NewArgTypeError(index, obj, typeName)
}

func FailObject(obj coretypes.Object, typeName, pattern string) *EvalError {
	if pattern == "" {
		pattern = "%s"
	}
	msg := fmt.Sprintf("Expected %s, got %s", typeName, obj.GetType().ToString(false))
	return RT.NewError(fmt.Sprintf(pattern, msg))
}

func installAssertionErrors() {
	coretypes.AssertionFailArg = func(obj coretypes.Object, typeName string, index int) any {
		return FailArg(obj, typeName, index)
	}
	coretypes.AssertionFailObject = func(obj coretypes.Object, typeName, pattern string) any {
		return FailObject(obj, typeName, pattern)
	}
}

func ExtractChar(args []coretypes.Object, index int) rune {
	return coretypes.EnsureArgIsChar(args, index).Ch
}

func ExtractTime(args []coretypes.Object, index int) time.Time {
	return coretypes.EnsureArgIsTime(args, index).T
}

func ExtractDouble(args []coretypes.Object, index int) float64 {
	return coretypes.EnsureArgIsDouble(args, index).D
}

func ExtractNumber(args []coretypes.Object, index int) coretypes.Number {
	return coretypes.EnsureArgIsNumber(args, index)
}

func ExtractBigInt(args []coretypes.Object, index int) *big.Int {
	return coretypes.EnsureArgIsBigInt(args, index).B
}

func ExtractBigFloat(args []coretypes.Object, index int) *big.Float {
	return coretypes.EnsureArgIsBigFloat(args, index).B
}

func ExtractRegex(args []coretypes.Object, index int) *regexp.Regexp {
	return coretypes.EnsureArgIsRegex(args, index).R
}

func ExtractSeqable(args []coretypes.Object, index int) coretypes.Seqable {
	return coretypes.EnsureArgIsSeqable(args, index)
}

func ExtractMap(args []coretypes.Object, index int) coretypes.Map {
	return coretypes.EnsureArgIsMap(args, index)
}

func ExtractIOReader(args []coretypes.Object, index int) io.Reader {
	return coretypes.EnsureArgIsio_Reader(args, index)
}

func ExtractIOWriter(args []coretypes.Object, index int) io.Writer {
	return coretypes.EnsureArgIsio_Writer(args, index)
}

var procMeta = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	switch obj := args[0].(type) {
	case coretypes.Meta:
		meta := obj.GetMeta()
		if meta != nil {
			return meta
		}
	}
	return NIL
}

var procWithMeta = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 2)
	m := coretypes.EnsureArgIsMeta(args, 0)
	if args[1].Equals(NIL) {
		return args[0]
	}
	return m.WithMeta(coretypes.EnsureArgIsMap(args, 1))
}

var procIsZero = func(args []coretypes.Object) coretypes.Object {
	switch n := args[0].(type) {
	case coretypes.Int:
		return coretypes.Boolean{B: n.I == 0}
	case coretypes.Double:
		return coretypes.Boolean{B: n.D == 0}
	}
	n := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(n)
	return coretypes.Boolean{B: ops.IsZero(n)}
}

var procIsPos = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(n)
	return coretypes.Boolean{B: ops.Gt(n, coretypes.Int{I: 0})}
}

var procIsNeg = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(n)
	return coretypes.Boolean{B: ops.Lt(n, coretypes.Int{I: 0})}
}

var procAdd = func(args []coretypes.Object) coretypes.Object {
	switch x := args[0].(type) {
	case coretypes.Int:
		switch y := args[1].(type) {
		case coretypes.Int:
			return coretypes.INT_OPS.Add(x, y)
		case coretypes.Double:
			return coretypes.Double{D: float64(x.I) + y.D}
		}
	case coretypes.Double:
		switch y := args[1].(type) {
		case coretypes.Int:
			return coretypes.Double{D: x.D + float64(y.I)}
		case coretypes.Double:
			return coretypes.Double{D: x.D + y.D}
		}
	}
	x := coretypes.EnsureObjectIsNumber(args[0], "")
	y := coretypes.EnsureObjectIsNumber(args[1], "")
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y))
	return ops.Add(x, y)
}

var procAddEx = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureObjectIsNumber(args[0], "")
	y := coretypes.EnsureObjectIsNumber(args[1], "")
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y)).Combine(coretypes.BIGINT_OPS)
	return ops.Add(x, y)
}

var procMultiply = func(args []coretypes.Object) coretypes.Object {
	switch x := args[0].(type) {
	case coretypes.Int:
		switch y := args[1].(type) {
		case coretypes.Int:
			return coretypes.INT_OPS.Multiply(x, y)
		case coretypes.Double:
			return coretypes.Double{D: float64(x.I) * y.D}
		}
	case coretypes.Double:
		switch y := args[1].(type) {
		case coretypes.Int:
			return coretypes.Double{D: x.D * float64(y.I)}
		case coretypes.Double:
			return coretypes.Double{D: x.D * y.D}
		}
	}
	x := coretypes.EnsureObjectIsNumber(args[0], "")
	y := coretypes.EnsureObjectIsNumber(args[1], "")
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y))
	return ops.Multiply(x, y)
}

var procMultiplyEx = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureObjectIsNumber(args[0], "")
	y := coretypes.EnsureObjectIsNumber(args[1], "")
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y)).Combine(coretypes.BIGINT_OPS)
	return ops.Multiply(x, y)
}

var procSubtract = func(args []coretypes.Object) coretypes.Object {
	if len(args) == 1 {
		switch x := args[0].(type) {
		case coretypes.Int:
			return coretypes.INT_OPS.Subtract(coretypes.Int{I: 0}, x)
		case coretypes.Double:
			return coretypes.Double{D: -x.D}
		}
		a := coretypes.Int{I: 0}
		b := coretypes.EnsureObjectIsNumber(args[0], "")
		ops := coretypes.GetOps(a).Combine(coretypes.GetOps(b))
		return ops.Subtract(a, b)
	}
	switch a := args[0].(type) {
	case coretypes.Int:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.INT_OPS.Subtract(a, b)
		case coretypes.Double:
			return coretypes.Double{D: float64(a.I) - b.D}
		}
	case coretypes.Double:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.Double{D: a.D - float64(b.I)}
		case coretypes.Double:
			return coretypes.Double{D: a.D - b.D}
		}
	}
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	ops := coretypes.GetOps(a).Combine(coretypes.GetOps(b))
	return ops.Subtract(a, b)
}

var procSubtractEx = func(args []coretypes.Object) coretypes.Object {
	var a, b coretypes.Object
	if len(args) == 1 {
		a = coretypes.Int{I: 0}
		b = args[0]
	} else {
		a = args[0]
		b = args[1]
	}
	an := coretypes.EnsureObjectIsNumber(a, "")
	bn := coretypes.EnsureObjectIsNumber(b, "")
	ops := coretypes.GetOps(an).Combine(coretypes.GetOps(bn)).Combine(coretypes.BIGINT_OPS)
	return ops.Subtract(an, bn)
}

var procDivide = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureArgIsNumber(args, 0)
	y := coretypes.EnsureArgIsNumber(args, 1)
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y))
	return ops.Divide(x, y)
}

var procQuot = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureArgIsNumber(args, 0)
	y := coretypes.EnsureArgIsNumber(args, 1)
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y))
	return ops.Quotient(x, y)
}

var procRem = func(args []coretypes.Object) coretypes.Object {
	switch x := args[0].(type) {
	case coretypes.Int:
		if y, ok := args[1].(coretypes.Int); ok {
			if y.I == 0 {
				coretypes.PanicOnZero(coretypes.INT_OPS, y)
			}
			return coretypes.Int{I: x.I % y.I}
		}
	}
	x := coretypes.EnsureArgIsNumber(args, 0)
	y := coretypes.EnsureArgIsNumber(args, 1)
	ops := coretypes.GetOps(x).Combine(coretypes.GetOps(y))
	return ops.Rem(x, y)
}

var procBitNot = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureObjectIsInt(args[0], "Bit operation not supported for "+args[0].GetType().ToString(false))
	return coretypes.Int{I: ^x.I}
}

func EnsureObjectIsInts(args []coretypes.Object) (coretypes.Int, coretypes.Int) {
	x := coretypes.EnsureObjectIsInt(args[0], "Bit operation not supported: %s")
	y := coretypes.EnsureObjectIsInt(args[1], "Bit operation not supported: %s")
	return x, y
}

var procBitAnd = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I & y.I}
}

var procBitOr = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I | y.I}
}

var procBitXor = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I ^ y.I}
}

var procBitAndNot = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I &^ y.I}
}

func checkedBitIndex(index int, op string) uint {
	if index < 0 {
		panic(RT.NewError(op + " bit index must be non-negative"))
	}
	if index >= strconv.IntSize {
		panic(RT.NewError(op + " bit index is too large"))
	}
	return uint(index)
}

func checkedShiftCount(count int, op string) uint {
	if count < 0 {
		panic(RT.NewError(op + " shift count must be non-negative"))
	}
	return uint(count)
}

var procBitClear = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I &^ (1 << checkedBitIndex(y.I, "bit-clear"))}
}

var procBitSet = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I | (1 << checkedBitIndex(y.I, "bit-set"))}
}

var procBitFlip = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I ^ (1 << checkedBitIndex(y.I, "bit-flip"))}
}

var procBitTest = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Boolean{B: x.I&(1<<checkedBitIndex(y.I, "bit-test")) != 0}
}

var procBitShiftLeft = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I << checkedShiftCount(y.I, "bit-shift-left")}
}

var procBitShiftRight = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: x.I >> checkedShiftCount(y.I, "bit-shift-right")}
}

var procUnsignedBitShiftRight = func(args []coretypes.Object) coretypes.Object {
	x, y := EnsureObjectIsInts(args)
	return coretypes.Int{I: int(uint(x.I) >> checkedShiftCount(y.I, "unsigned-bit-shift-right"))}
}

var procExInfo = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 3)
	res := &ExInfo{
		rt: cloneGRT(),
	}
	res.Add(KEYWORDS.message, coretypes.EnsureArgIsString(args, 0))
	res.Add(KEYWORDS.data, coretypes.EnsureArgIsMap(args, 1))
	if len(args) == 3 {
		res.Add(KEYWORDS.cause, coretypes.EnsureArgIsError(args, 2))
	}
	return res
}

var procExData = func(args []coretypes.Object) coretypes.Object {
	if ok, res := args[0].(*ExInfo).Get(KEYWORDS.data); ok {
		return res
	}
	return NIL
}

var procExCause = func(args []coretypes.Object) coretypes.Object {
	if ok, res := args[0].(*ExInfo).Get(KEYWORDS.cause); ok {
		return res
	}
	return NIL
}

var procExMessage = func(args []coretypes.Object) coretypes.Object {
	return args[0].(coretypes.Error).Message()
}

var procRegex = func(args []coretypes.Object) coretypes.Object {
	r, err := regexp.Compile(coretypes.EnsureArgIsString(args, 0).S)
	if err != nil {
		panic(RT.NewError("Invalid regex: " + err.Error()))
	}
	return coretypes.MakeRegex(r)
}

func reGroups(s string, indexes []int) coretypes.Object {
	if indexes == nil {
		return NIL
	} else if len(indexes) == 2 {
		if indexes[0] == -1 {
			return NIL
		} else {
			return coretypes.String{S: s[indexes[0]:indexes[1]]}
		}
	} else {
		v := corecollections.EmptyVector()
		for i := 0; i < len(indexes); i += 2 {
			if indexes[i] == -1 {
				v = v.Conjoin(NIL)
			} else {
				v = v.Conjoin(coretypes.String{S: s[indexes[i]:indexes[i+1]]})
			}
		}
		return v
	}
}

var procReSeq = func(args []coretypes.Object) coretypes.Object {
	re := coretypes.EnsureArgIsRegex(args, 0)
	s := coretypes.EnsureArgIsString(args, 1)
	matches := re.R.FindAllStringSubmatchIndex(s.S, -1)
	if matches == nil {
		return NIL
	}
	res := make([]coretypes.Object, len(matches))
	for i, match := range matches {
		res[i] = reGroups(s.S, match)
	}
	return &corecollections.ArraySeq{Arr: res}
}

var procReFind = func(args []coretypes.Object) coretypes.Object {
	re := coretypes.EnsureArgIsRegex(args, 0)
	s := coretypes.EnsureArgIsString(args, 1)
	match := re.R.FindStringSubmatchIndex(s.S)
	return reGroups(s.S, match)
}

var procRand = func(args []coretypes.Object) coretypes.Object {
	r := rand.Float64()
	return coretypes.Double{D: r}
}

var procIsSpecialSymbol = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Boolean{B: coretypes.IsSpecialSymbol(args[0])}
}

var procSubs = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0).S
	start := coretypes.EnsureArgIsInt(args, 1).I
	slen := utf8.RuneCountInString(s)
	end := slen
	if len(args) > 2 {
		end = coretypes.EnsureArgIsInt(args, 2).I
	}
	if start < 0 || start > slen {
		panic(RT.NewError(fmt.Sprintf("String index out of range: %d", start)))
	}
	if end < 0 || end > slen {
		panic(RT.NewError(fmt.Sprintf("String index out of range: %d", end)))
	}
	return coretypes.String{S: string([]rune(s)[start:end])}
}

var procIntern = func(args []coretypes.Object) coretypes.Object {
	ns := EnsureArgIsNamespace(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	vr := ns.Intern(sym)
	if len(args) == 3 {
		vr.Value = args[2]
	}
	return vr
}

var procSetMeta = func(args []coretypes.Object) coretypes.Object {
	vr := EnsureArgIsVar(args, 0)
	meta := coretypes.EnsureArgIsMap(args, 1)
	vr.Meta = meta
	return NIL
}

var procAtom = func(args []coretypes.Object) coretypes.Object {
	res := corert.NewAtom(args[0], nil)
	if len(args) > 1 {
		m := corecollections.NewHashMap(args[1:]...)
		if ok, v := m.Get(KEYWORDS.meta); ok {
			res = corert.NewAtom(args[0], coretypes.EnsureObjectIsMap(v, ""))
		}
	}
	return res
}

var procDeref = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsDeref(args, 0).Deref()
}

var procSwap = func(args []coretypes.Object) coretypes.Object {
	a := EnsureArgIsAtom(args, 0)
	f := coretypes.EnsureArgIsCallable(args, 1)
	oldValue, newValue := a.Swap(f, args[2:], func(v coretypes.Object) { validateAtom(a, v) })
	notifyWatches(a, oldValue, newValue)
	return newValue
}

var procSwapVals = func(args []coretypes.Object) coretypes.Object {
	a := EnsureArgIsAtom(args, 0)
	f := coretypes.EnsureArgIsCallable(args, 1)
	oldValue, newValue := a.Swap(f, args[2:], func(v coretypes.Object) { validateAtom(a, v) })
	notifyWatches(a, oldValue, newValue)
	return corecollections.NewVectorFrom(oldValue, newValue)
}

var procReset = func(args []coretypes.Object) coretypes.Object {
	a := EnsureArgIsAtom(args, 0)
	newValue := args[1]
	oldValue := a.Reset(newValue, func(v coretypes.Object) { validateAtom(a, v) })
	notifyWatches(a, oldValue, newValue)
	return newValue
}

var procResetVals = func(args []coretypes.Object) coretypes.Object {
	a := EnsureArgIsAtom(args, 0)
	newValue := args[1]
	oldValue := a.Reset(newValue, func(v coretypes.Object) { validateAtom(a, v) })
	notifyWatches(a, oldValue, newValue)
	return corecollections.NewVectorFrom(oldValue, newValue)
}

var procAlterMeta = func(args []coretypes.Object) coretypes.Object {
	r := coretypes.EnsureArgIsRef(args, 0)
	f := EnsureArgIsFn(args, 1)
	return r.AlterMeta(f, args[2:])
}

var procResetMeta = func(args []coretypes.Object) coretypes.Object {
	r := coretypes.EnsureArgIsRef(args, 0)
	m := coretypes.EnsureArgIsMap(args, 1)
	return r.ResetMeta(m)
}

var procEmpty = func(args []coretypes.Object) coretypes.Object {
	switch c := args[0].(type) {
	case coretypes.Collection:
		return c.Empty()
	default:
		return NIL
	}
}

var procIsBound = func(args []coretypes.Object) coretypes.Object {
	vr := EnsureArgIsVar(args, 0)
	return coretypes.Boolean{B: vr.Value != nil}
}

// Convert Joker object to native Go object. For those satisfying the
// coretypes.Native type, that's straightforward. For other Joker objects, try
// converting them to suitable native Go objects. E.g. a coretypes.BigInt might
// hold a value > MaxInt64 but < MaxUint64, in which case conversion
// to a uint64 makes more sense than returning the stringized version,
// for use cases such as `(format "%x" value)`. Even for coretypes.BigFloat and
// BigRat, try to (accurately) convert them to native types so they
// can be formatted via the usual ways.
func ToNative(obj coretypes.Object) interface{} {
	switch obj := obj.(type) {
	case coretypes.Native:
		return obj.Native()
	case *coretypes.BigInt:
		b := obj.BigInt()
		if b.IsInt64() {
			return b.Int64()
		}
		if b.IsUint64() {
			return b.Uint64()
		}
	case *coretypes.BigFloat:
		b := obj.BigFloat()
		if f, acc := b.Float64(); acc == big.Exact {
			return f
		}
	case *coretypes.Ratio:
		b := obj.Ratio()
		if f, exact := b.Float64(); exact {
			return f
		}
	}
	return obj.ToString(false)
}

var procFormat = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0)
	objs := args[1:]
	fargs := make([]interface{}, len(objs))
	for i, v := range objs {
		fargs[i] = ToNative(v)
	}
	res := fmt.Sprintf(s.S, fargs...)
	return coretypes.String{S: res}
}

var procList = func(args []coretypes.Object) coretypes.Object {
	return corecollections.NewListFrom(args...)
}

var procCons = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 2)
	s := coretypes.EnsureArgIsSeqable(args, 1).Seq()
	return s.Cons(args[0])
}

var procFirst = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	s := coretypes.EnsureArgIsSeqable(args, 0).Seq()
	return s.First()
}

var procNext = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	s := coretypes.EnsureArgIsSeqable(args, 0).Seq()
	res := s.Rest()
	if res.IsEmpty() {
		return NIL
	}
	return res
}

var procRest = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	s := coretypes.EnsureArgIsSeqable(args, 0).Seq()
	return s.Rest()
}

var procConj = func(args []coretypes.Object) coretypes.Object {
	switch c := args[0].(type) {
	case coretypes.Conjable:
		return c.Conj(args[1])
	case coretypes.Seq:
		return c.Cons(args[1])
	default:
		panic(RT.NewError("conj's first argument must be a collection, got " + c.GetType().ToString(false)))
	}
}

var procSeq = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	s := coretypes.EnsureArgIsSeqable(args, 0).Seq()
	if s.IsEmpty() {
		return NIL
	}
	return s
}

var procIsInstance = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 2, 2)
	t := coretypes.EnsureArgIsType(args, 0)
	return coretypes.Boolean{B: coretypes.IsInstance(t, args[1])}
}

var procAssoc = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsAssociative(args, 0).Assoc(args[1], args[2])
}

var procEquals = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Boolean{B: args[0].Equals(args[1])}
}

var procCount = func(args []coretypes.Object) coretypes.Object {
	switch obj := args[0].(type) {
	case coretypes.Counted:
		return coretypes.Int{I: obj.Count()}
	default:
		s := coretypes.EnsureObjectIsSeqable(obj, "count not supported on this type: %s")
		return coretypes.Int{I: corecollections.SeqCount(s.Seq())}
	}
}

var procSubvec = func(args []coretypes.Object) coretypes.Object {
	// TODO: implement proper Subvector structure
	v := coretypes.EnsureArgIsVec(args, 0)
	start := coretypes.EnsureArgIsInt(args, 1).I
	end := coretypes.EnsureArgIsInt(args, 2).I
	if start > end {
		panic(RT.NewError(fmt.Sprintf("subvec's start index (%d) is greater than end index (%d)", start, end)))
	}
	if end > v.Count() {
		panic(RT.NewError(fmt.Sprintf("subvec's end index (%d) is greater than vector's count (%d)", end, v.Count())))
	}
	subv := make([]coretypes.Object, 0, end-start)
	for i := start; i < end; i++ {
		subv = append(subv, v.At(i))
	}
	return corecollections.NewVectorFrom(subv...)
}

var procCast = func(args []coretypes.Object) coretypes.Object {
	t := coretypes.EnsureArgIsType(args, 0)
	if coretypes.IsEqualOrImplements(t, args[1].GetType()) {
		return args[1]
	}
	panic(RT.NewError("Cannot cast " + args[1].GetType().ToString(false) + " to " + t.ToString(false)))
}

var procVec = func(args []coretypes.Object) coretypes.Object {
	return corecollections.NewVectorFromSeq(coretypes.EnsureArgIsSeqable(args, 0).Seq())
}

var procHashMap = func(args []coretypes.Object) coretypes.Object {
	if len(args)%2 != 0 {
		panic(RT.NewError("No value supplied for key " + args[len(args)-1].ToString(false)))
	}
	return corecollections.NewHashMap(args...)
}

var procHashSet = func(args []coretypes.Object) coretypes.Object {
	res := corecollections.EmptySet()
	for i := 0; i < len(args); i++ {
		res.Add(args[i])
	}
	return res
}

func str(args ...coretypes.Object) string {
	var buffer bytes.Buffer
	for _, obj := range args {
		if !obj.Equals(NIL) {
			t := obj.GetType()
			// TODO: this is a hack. Rethink escape parameter in ToString
			escaped := (t == TYPE.String) || (t == TYPE.Char) || (t == TYPE.Regex)
			buffer.WriteString(obj.ToString(!escaped))
		}
	}
	return buffer.String()
}

var procStr = func(args []coretypes.Object) coretypes.Object {
	// Fast path: 2-arg str (common in parsers: (str buf c))
	if len(args) == 2 {
		a, b := args[0], args[1]
		// Fastest: string + char (the parser hot path)
		if as, ok := a.(coretypes.String); ok {
			if bc, ok := b.(coretypes.Char); ok {
				return coretypes.String{S: as.S + charToStringFast(bc.Ch)}
			}
			if bs, ok := b.(coretypes.String); ok {
				return coretypes.String{S: as.S + bs.S}
			}
		}
		// General 2-arg
		if a.Equals(NIL) {
			if b.Equals(NIL) {
				return coretypes.String{S: ""}
			}
			return coretypes.String{S: b.ToString(false)}
		}
		if b.Equals(NIL) {
			return coretypes.String{S: a.ToString(false)}
		}
		return coretypes.String{S: a.ToString(false) + b.ToString(false)}
	}
	// 1-arg str
	if len(args) == 1 {
		a := args[0]
		if a.Equals(NIL) {
			return coretypes.String{S: ""}
		}
		if s, ok := a.(coretypes.String); ok {
			return s
		}
		return coretypes.String{S: a.ToString(false)}
	}
	return coretypes.String{S: str(args...)}
}

var procSymbol = func(args []coretypes.Object) coretypes.Object {
	if len(args) == 1 {
		return coretypes.MakeSymbol(STRINGS.Intern, coretypes.EnsureArgIsString(args, 0).S)
	}
	var ns *string = nil
	if !args[0].Equals(NIL) {
		ns = STRINGS.Intern(coretypes.EnsureArgIsString(args, 0).S)
	}
	return coretypes.MakeSymbolFromKeys(ns, STRINGS.Intern(coretypes.EnsureArgIsString(args, 1).S))
}

var procKeyword = func(args []coretypes.Object) coretypes.Object {
	if len(args) == 1 {
		switch obj := args[0].(type) {
		case coretypes.String:
			return coretypes.MakeKeyword(STRINGS.Intern, obj.S)
		case coretypes.Symbol:
			return coretypes.MakeKeywordFromKeys(obj.NamespaceKey(), obj.NameKey())
		default:
			return NIL
		}
	}
	var ns *string = nil
	if !args[0].Equals(NIL) {
		ns = STRINGS.Intern(coretypes.EnsureArgIsString(args, 0).S)
	}
	name := STRINGS.Intern(coretypes.EnsureArgIsString(args, 1).S)
	return coretypes.MakeKeywordFromKeys(ns, name)
}

var procGensym = func(args []coretypes.Object) coretypes.Object {
	return genSym(coretypes.EnsureArgIsString(args, 0).S, "")
}

var procApply = func(args []coretypes.Object) coretypes.Object {
	// TODO:
	// coretypes.Stacktrace is broken. Need to somehow know
	// the name of the function passed ...
	f := coretypes.EnsureArgIsCallable(args, 0)
	return f.Call(corecollections.ToSlice(coretypes.EnsureArgIsSeqable(args, 1).Seq()))
}

var procLazySeq = func(args []coretypes.Object) coretypes.Object {
	return &corecollections.LazySeq{
		Fn: args[0].(*Fn),
	}
}

var procDelay = func(args []coretypes.Object) coretypes.Object {
	return coretypes.NewDelay(args[0].(*Fn))
}

var procForce = func(args []coretypes.Object) coretypes.Object {
	switch d := args[0].(type) {
	case *coretypes.Delay:
		return d.Force()
	default:
		return d
	}
}

var procIdentical = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Boolean{B: args[0] == args[1]}
}

var procCompare = func(args []coretypes.Object) coretypes.Object {
	k1, k2 := args[0], args[1]
	if k1.Equals(k2) {
		return coretypes.Int{I: 0}
	}
	switch k2.(type) {
	case Nil:
		return coretypes.Int{I: 1}
	}
	switch k1 := k1.(type) {
	case Nil:
		return coretypes.Int{I: -1}
	case coretypes.Comparable:
		return coretypes.Int{I: k1.Compare(k2)}
	}
	panic(RT.NewError(fmt.Sprintf("%s (type: %s) is not a Comparable", k1.ToString(true), k1.GetType().ToString(false))))
}

var procInt = func(args []coretypes.Object) coretypes.Object {
	switch obj := args[0].(type) {
	case coretypes.Char:
		return coretypes.Int{I: int(obj.Ch)}
	case coretypes.Number:
		return obj.Int()
	default:
		panic(RT.NewError(fmt.Sprintf("Cannot cast %s (type: %s) to Int", obj.ToString(true), obj.GetType().ToString(false))))
	}
}

var procNumber = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureObjectIsNumber(args[0], "Cannot cast "+args[0].ToString(true)+": %s")
}

var procDouble = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureObjectIsNumber(args[0], "Cannot cast "+args[0].ToString(true)+": %s")
	return n.Double()
}

var procChar = func(args []coretypes.Object) coretypes.Object {
	switch c := args[0].(type) {
	case coretypes.Char:
		return c
	case coretypes.Number:
		i := c.Int().I
		if i < coretypes.MIN_RUNE || i > coretypes.MAX_RUNE {
			panic(RT.NewError(fmt.Sprintf("Value out of range for char: %d", i)))
		}
		return coretypes.Char{Ch: rune(i)}
	default:
		panic(RT.NewError(fmt.Sprintf("Cannot cast %s (type: %s) to Char", c.ToString(true), c.GetType().ToString(false))))
	}
}

var procBoolean = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Boolean{B: ToBool(args[0])}
}

var procNumerator = func(args []coretypes.Object) coretypes.Object {
	bi := coretypes.EnsureArgIsRatio(args, 0).R.Num()
	return &coretypes.BigInt{B: bi}
}

var procDenominator = func(args []coretypes.Object) coretypes.Object {
	bi := coretypes.EnsureArgIsRatio(args, 0).R.Denom()
	return &coretypes.BigInt{B: bi}
}

var procBigInt = func(args []coretypes.Object) coretypes.Object {
	switch n := args[0].(type) {
	case coretypes.Number:
		return &coretypes.BigInt{B: n.BigInt()}
	case coretypes.String:
		bi := &big.Int{}
		if _, ok := bi.SetString(n.S, 10); ok {
			return &coretypes.BigInt{B: bi}
		}
		panic(RT.NewError("Invalid number format " + n.S))
	default:
		panic(RT.NewError(fmt.Sprintf("Cannot cast %s (type: %s) to coretypes.BigInt", n.ToString(true), n.GetType().ToString(false))))
	}
}

var procBigFloat = func(args []coretypes.Object) coretypes.Object {
	switch n := args[0].(type) {
	case coretypes.Number:
		return &coretypes.BigFloat{B: n.BigFloat()}
	case coretypes.String:
		b := &big.Float{}
		if _, ok := b.SetString(n.S); ok {
			return &coretypes.BigFloat{B: b}
		}
		panic(RT.NewError("Invalid number format " + n.S))
	default:
		panic(RT.NewError(fmt.Sprintf("Cannot cast %s (type: %s) to coretypes.BigFloat", n.ToString(true), n.GetType().ToString(false))))
	}
}

var procNth = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 1).Int().I
	switch coll := args[0].(type) {
	case coretypes.Indexed:
		if len(args) == 3 {
			return coll.TryNth(n, args[2])
		}
		return coll.Nth(n)
	case Nil:
		return NIL
	case coretypes.Sequential:
		switch coll := args[0].(type) {
		case coretypes.Seqable:
			if len(args) == 3 {
				return corecollections.SeqTryNth(coll.Seq(), n, args[2])
			}
			return corecollections.SeqNth(coll.Seq(), n)
		}
	}
	panic(RT.NewError("nth not supported on this type: " + args[0].GetType().ToString(false)))
}

var procLt = func(args []coretypes.Object) coretypes.Object {
	switch a := args[0].(type) {
	case coretypes.Int:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.Boolean{B: a.I < b.I}
		case coretypes.Double:
			return coretypes.Boolean{B: float64(a.I) < b.D}
		}
	case coretypes.Double:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.Boolean{B: a.D < float64(b.I)}
		case coretypes.Double:
			return coretypes.Boolean{B: a.D < b.D}
		}
	}
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Boolean{B: coretypes.GetOps(a).Combine(coretypes.GetOps(b)).Lt(a, b)}
}

var procLte = func(args []coretypes.Object) coretypes.Object {
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Boolean{B: coretypes.GetOps(a).Combine(coretypes.GetOps(b)).Lte(a, b)}
}

var procGt = func(args []coretypes.Object) coretypes.Object {
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Boolean{B: coretypes.GetOps(a).Combine(coretypes.GetOps(b)).Gt(a, b)}
}

var procGte = func(args []coretypes.Object) coretypes.Object {
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Boolean{B: coretypes.GetOps(a).Combine(coretypes.GetOps(b)).Gte(a, b)}
}

var procEq = func(args []coretypes.Object) coretypes.Object {
	switch a := args[0].(type) {
	case coretypes.Int:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.Boolean{B: a.I == b.I}
		case coretypes.Double:
			return coretypes.Boolean{B: float64(a.I) == b.D}
		}
	case coretypes.Double:
		switch b := args[1].(type) {
		case coretypes.Int:
			return coretypes.Boolean{B: a.D == float64(b.I)}
		case coretypes.Double:
			return coretypes.Boolean{B: a.D == b.D}
		}
	}
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Boolean{B: coretypes.NumbersEq(a, b)}
}

var procMax = func(args []coretypes.Object) coretypes.Object {
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Max(a, b)
}

var procMin = func(args []coretypes.Object) coretypes.Object {
	a := coretypes.EnsureObjectIsNumber(args[0], "")
	b := coretypes.EnsureObjectIsNumber(args[1], "")
	return coretypes.Min(a, b)
}

var procIncEx = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(x).Combine(coretypes.BIGINT_OPS)
	return ops.Add(x, coretypes.Int{I: 1})
}

var procDecEx = func(args []coretypes.Object) coretypes.Object {
	x := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(x).Combine(coretypes.BIGINT_OPS)
	return ops.Subtract(x, coretypes.Int{I: 1})
}

var procInc = func(args []coretypes.Object) coretypes.Object {
	switch x := args[0].(type) {
	case coretypes.Int:
		return coretypes.INT_OPS.Add(x, coretypes.Int{I: 1})
	case coretypes.Double:
		return coretypes.Double{D: x.D + 1}
	}
	x := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(x).Combine(coretypes.INT_OPS)
	return ops.Add(x, coretypes.Int{I: 1})
}

var procDec = func(args []coretypes.Object) coretypes.Object {
	switch x := args[0].(type) {
	case coretypes.Int:
		return coretypes.INT_OPS.Subtract(x, coretypes.Int{I: 1})
	case coretypes.Double:
		return coretypes.Double{D: x.D - 1}
	}
	x := coretypes.EnsureArgIsNumber(args, 0)
	ops := coretypes.GetOps(x).Combine(coretypes.INT_OPS)
	return ops.Subtract(x, coretypes.Int{I: 1})
}

var procPeek = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureObjectIsStack(args[0], "")
	return s.Peek()
}

var procPop = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureObjectIsStack(args[0], "")
	return s.Pop().(coretypes.Object)
}

var procContains = func(args []coretypes.Object) coretypes.Object {
	switch c := args[0].(type) {
	case coretypes.Gettable:
		ok, _ := c.Get(args[1])
		if ok {
			return coretypes.Boolean{B: true}
		}
		return coretypes.Boolean{B: false}
	}
	panic(RT.NewError("contains? not supported on type " + args[0].GetType().ToString(false)))
}

var procGet = func(args []coretypes.Object) coretypes.Object {
	switch c := args[0].(type) {
	case coretypes.Gettable:
		ok, v := c.Get(args[1])
		if ok {
			return v
		}
	}
	if len(args) == 3 {
		return args[2]
	}
	return NIL
}

var procDissoc = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsMap(args, 0).Without(args[1])
}

var procDisj = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsSet(args, 0).Disjoin(args[1])
}

var procFind = func(args []coretypes.Object) coretypes.Object {
	res := coretypes.EnsureArgIsAssociative(args, 0).EntryAt(args[1])
	if res == nil {
		return NIL
	}
	return res
}

var procKeys = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsMap(args, 0).Keys()
}

var procVals = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsMap(args, 0).Vals()
}

var procRseq = func(args []coretypes.Object) coretypes.Object {
	return coretypes.EnsureArgIsReversible(args, 0).Rseq()
}

var procName = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: coretypes.EnsureArgIsNamed(args, 0).Name()}
}

var procNamespace = func(args []coretypes.Object) coretypes.Object {
	ns := coretypes.EnsureArgIsNamed(args, 0).Namespace()
	if ns == "" {
		return NIL
	}
	return coretypes.String{S: ns}
}

var procFindVar = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	if sym.NamespaceKey() == nil {
		panic(RT.NewError("find-var argument must be namespace-qualified symbol"))
	}
	if v, ok := GLOBAL_ENV.Resolve(sym); ok {
		return v
	}
	return NIL
}

var procSort = func(args []coretypes.Object) coretypes.Object {
	cmp := coretypes.EnsureArgIsComparator(args, 0)
	coll := coretypes.EnsureArgIsSeqable(args, 1)
	s := coretypes.ComparatorSlice[coretypes.Object]{
		Items: corecollections.ToSlice(coll.Seq()),
		Cmp:   cmp,
	}
	sort.Sort(s)
	return &corecollections.ArraySeq{Arr: s.Items}
}

var procEval = func(args []coretypes.Object) coretypes.Object {
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	expr := Parse(args[0], parseContext)
	return Eval(expr, nil)
}

var procType = func(args []coretypes.Object) coretypes.Object {
	return args[0].GetType()
}

var procPprint = func(args []coretypes.Object) coretypes.Object {
	obj := args[0]
	w := coretypes.EnsureObjectIsio_Writer(GLOBAL_ENV.stdout.Value, "")
	pprintObject(obj, 0, w)
	fmt.Fprint(w, "\n")
	return NIL
}

func PrintObject(obj coretypes.Object, w io.Writer) {
	printReadably := ToBool(GLOBAL_ENV.printReadably.Value)
	switch obj := obj.(type) {
	case coretypes.Printer:
		obj.Print(w, printReadably)
	default:
		fmt.Fprint(w, obj.ToString(printReadably))
	}
}

var procPr = func(args []coretypes.Object) coretypes.Object {
	n := len(args)
	if n > 0 {
		f := coretypes.EnsureObjectIsio_Writer(GLOBAL_ENV.stdout.Value, "")
		for _, arg := range args[:n-1] {
			PrintObject(arg, f)
			fmt.Fprint(f, " ")
		}
		PrintObject(args[n-1], f)
	}
	return NIL
}

var procNewline = func(args []coretypes.Object) coretypes.Object {
	f := coretypes.EnsureObjectIsio_Writer(GLOBAL_ENV.stdout.Value, "")
	fmt.Fprintln(f)
	return NIL
}

var procFlush = func(args []coretypes.Object) coretypes.Object {
	switch f := args[0].(type) {
	case *File:
		f.Sync()
	}
	return NIL
}

func readFromReader(reader io.RuneReader) coretypes.Object {
	r := readerConstruction.NewReader(reader, "<>")
	obj, err := readerConstruction.TryRead(r)
	PanicOnErr(err)
	return obj
}

var procRead = func(args []coretypes.Object) coretypes.Object {
	switch f := args[0].(type) {
	case io.RuneReader:
		return readFromReader(f)
	case io.Reader:
		return readFromReader(osutil.AsRuneReader(f))
	default:
		panic(RT.NewArgTypeError(0, args[0], "io.RuneReader or io.Reader"))
	}
}

var procReadString = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	return readFromReader(osutil.StringRuneReader(coretypes.EnsureArgIsString(args, 0).S))
}

var procReadLine = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 0, 0)
	f := coretypes.EnsureObjectIsStringReader(GLOBAL_ENV.stdin.Value, "")
	line, err := osutil.ReadLine(f)
	if err != nil {
		return NIL
	}
	return coretypes.String{S: line}
}

var procReaderReadLine = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	rdr := coretypes.EnsureArgIsStringReader(args, 0)
	line, err := osutil.ReadLine(rdr)
	if err != nil {
		return NIL
	}
	return coretypes.String{S: line}
}

var procNanoTime = func(args []coretypes.Object) coretypes.Object {
	return &coretypes.BigInt{B: big.NewInt(time.Now().UnixNano())}
}

var procMacroexpand1 = func(args []coretypes.Object) coretypes.Object {
	switch s := args[0].(type) {
	case coretypes.Seq:
		parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
		return macroexpand1(s, parseContext)
	default:
		return s
	}
}

func loadReader(reader *Reader) (coretypes.Object, error) {
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	var lastObj coretypes.Object = NIL
	for {
		obj, err := readerConstruction.TryRead(reader)
		if err == io.EOF {
			return lastObj, nil
		}
		if err != nil {
			return nil, err
		}
		expr, err := TryParse(obj, parseContext)
		if err != nil {
			return nil, err
		}
		lastObj, err = TryEval(expr)
		if err != nil {
			return nil, err
		}
	}
}

var procLoadString = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0)
	obj, err := loadReader(readerConstruction.NewReader(osutil.StringRuneReader(s.S), "<string>"))
	if err != nil {
		panic(RT.NewError(err.Error()))
	}
	return obj
}

var procFindNamespace = func(args []coretypes.Object) coretypes.Object {
	ns := GLOBAL_ENV.FindNamespace(coretypes.EnsureArgIsSymbol(args, 0))
	if ns == nil {
		return NIL
	}
	return ns
}

var procCreateNamespace = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	res := GLOBAL_ENV.EnsureSymbolIsNamespace(sym)
	// In linter mode the latest create-ns call overrides position info.
	// This is for the cases when (ns ...) is called in .jokerd/linter.clj file and alike.
	// Also, isUsed needs to be reset in this case.
	if LINTER_MODE {
		res.Name = res.Name.WithInfo(sym.GetInfo()).(coretypes.Symbol)
		res.isUsed = false
	}
	return res
}

var procInjectNamespace = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	ns := GLOBAL_ENV.EnsureSymbolIsNamespace(sym)
	ns.isUsed = true
	ns.isGloballyUsed = true
	return ns
}

var procInjectLinterType = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	LINTER_TYPES[sym.NameKey()] = true
	return NIL
}

var procRemoveNamespace = func(args []coretypes.Object) coretypes.Object {
	ns := GLOBAL_ENV.RemoveNamespace(coretypes.EnsureArgIsSymbol(args, 0))
	if ns == nil {
		return NIL
	}
	return ns
}

var procAllNamespaces = func(args []coretypes.Object) coretypes.Object {
	s := make([]coretypes.Object, 0, len(GLOBAL_ENV.Namespaces))
	for _, ns := range GLOBAL_ENV.Namespaces {
		s = append(s, ns)
	}
	return &corecollections.ArraySeq{Arr: s}
}

var procNamespaceName = func(args []coretypes.Object) coretypes.Object {
	return EnsureArgIsNamespace(args, 0).Name
}

var procNamespaceMap = func(args []coretypes.Object) coretypes.Object {
	r := &corecollections.ArrayMap{}
	for k, v := range EnsureArgIsNamespace(args, 0).mappings {
		r.Add(coretypes.MakeSymbol(STRINGS.Intern, *k), v)
	}
	return r
}

var procNamespaceUnmap = func(args []coretypes.Object) coretypes.Object {
	ns := EnsureArgIsNamespace(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	if sym.NamespaceKey() != nil {
		panic(RT.NewError("Can't unintern namespace-qualified symbol"))
	}
	delete(ns.mappings, sym.NameKey())
	return NIL
}

var procVarNamespace = func(args []coretypes.Object) coretypes.Object {
	v := EnsureArgIsVar(args, 0)
	return v.ns
}

var procRefer = func(args []coretypes.Object) coretypes.Object {
	ns := EnsureArgIsNamespace(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	v := EnsureArgIsVar(args, 2)
	return ns.Refer(sym, v)
}

var procAlias = func(args []coretypes.Object) coretypes.Object {
	EnsureArgIsNamespace(args, 0).AddAlias(coretypes.EnsureArgIsSymbol(args, 1), EnsureArgIsNamespace(args, 2))
	return NIL
}

var procNamespaceAliases = func(args []coretypes.Object) coretypes.Object {
	r := &corecollections.ArrayMap{}
	for k, v := range EnsureArgIsNamespace(args, 0).aliases {
		r.Add(coretypes.MakeSymbol(STRINGS.Intern, *k), v)
	}
	return r
}

var procNamespaceUnalias = func(args []coretypes.Object) coretypes.Object {
	ns := EnsureArgIsNamespace(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	if sym.NamespaceKey() != nil {
		panic(RT.NewError("Alias can't be namespace-qualified"))
	}
	delete(ns.aliases, sym.NameKey())
	return NIL
}

var procVarGet = func(args []coretypes.Object) coretypes.Object {
	return EnsureArgIsVar(args, 0).Resolve()
}

var procVarSet = func(args []coretypes.Object) coretypes.Object {
	EnsureArgIsVar(args, 0).Value = args[1]
	return args[1]
}

var procNsResolve = func(args []coretypes.Object) coretypes.Object {
	ns := EnsureArgIsNamespace(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	if sym.NamespaceKey() == nil && TYPES.Contains(sym.NameKey()) {
		return TYPES.Lookup(sym.NameKey())
	}
	if vr, ok := GLOBAL_ENV.ResolveIn(ns, sym); ok {
		return vr
	}
	return NIL
}

var procArrayMap = func(args []coretypes.Object) coretypes.Object {
	if len(args)%2 == 1 {
		panic(RT.NewError("No value supplied for key " + args[len(args)-1].ToString(false)))
	}
	res := corecollections.EmptyArrayMap()
	for i := 0; i < len(args); i += 2 {
		res.Set(args[i], args[i+1])
	}
	return res
}

const bufferHashMask uint32 = 0x5ed19e84

var procBuffer = func(args []coretypes.Object) coretypes.Object {
	if len(args) > 0 {
		s := coretypes.EnsureArgIsString(args, 0)
		return MakeBuffer(bytes.NewBufferString(s.S))
	}
	return MakeBuffer(&bytes.Buffer{})
}

var procBufferedReader = func(args []coretypes.Object) coretypes.Object {
	switch rdr := args[0].(type) {
	case io.Reader:
		return MakeBufferedReader(rdr)
	default:
		panic(RT.NewArgTypeError(0, args[0], "IOReader"))
	}
}

var procSlurp = func(args []coretypes.Object) coretypes.Object {
	switch f := args[0].(type) {
	case coretypes.String:
		s, err := osutil.ReadFileString(f.S)
		PanicOnErr(err)
		return coretypes.String{S: s}
	case io.Reader:
		s, err := osutil.ReadAllString(f)
		PanicOnErr(err)
		return coretypes.String{S: s}
	default:
		panic(RT.NewArgTypeError(0, args[0], "String or IOReader"))
	}
}

var procSpit = func(args []coretypes.Object) coretypes.Object {
	f := args[0]
	content := args[1]
	opts := coretypes.EnsureArgIsMap(args, 2)
	appendFile := false
	if ok, append := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "append")); ok {
		appendFile = ToBool(append)
	}
	switch f := f.(type) {
	case coretypes.String:
		err := osutil.WriteFileString(f.S, str(content), appendFile)
		PanicOnErr(err)
	case io.Writer:
		err := osutil.WriteString(f, str(content))
		PanicOnErr(err)
	default:
		panic(RT.NewArgTypeError(0, args[0], "String or IOWriter"))
	}
	return NIL
}

var procShuffle = func(args []coretypes.Object) coretypes.Object {
	s := corecollections.ToSlice(coretypes.EnsureArgIsSeqable(args, 0).Seq())
	for i := range s {
		j := rand.Intn(i + 1)
		s[i], s[j] = s[j], s[i]
	}
	return corecollections.NewVectorFrom(s...)
}

var procIsRealized = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Boolean{B: coretypes.EnsureArgIsPending(args, 0).IsRealized()}
}

var procDeriveInfo = func(args []coretypes.Object) coretypes.Object {
	dest := args[0]
	src := args[1]
	return coretypes.WithInfo(dest, src.GetInfo())
}

var procJokerVersion = func(args []coretypes.Object) coretypes.Object {
	return coretypes.String{S: VERSION[1:]}
}

var procHash = func(args []coretypes.Object) coretypes.Object {
	return coretypes.Int{I: int(args[0].Hash())}
}

func loadFile(filename string) coretypes.Object {
	var reader *Reader
	f, rr, err := osutil.OpenRuneFile(filename)
	PanicOnErr(err)
	defer func() { PanicOnErr(f.Close()) }()
	reader = readerConstruction.NewReader(rr, filename)
	ProcessReaderFromEval(reader, filename)
	return NIL
}

var procLoadFile = func(args []coretypes.Object) coretypes.Object {
	filename := coretypes.EnsureArgIsString(args, 0)
	return loadFile(filename.S)
}

var procLoadLibFromPath = func(args []coretypes.Object) coretypes.Object {
	libname := coretypes.EnsureArgIsSymbol(args, 0).Name()
	pathname := coretypes.EnsureArgIsString(args, 1).S
	cp := GLOBAL_ENV.classPath.Value
	cpvec := coretypes.EnsureObjectIsVec(cp, "*classpath*: %s")
	count := cpvec.Count()
	var f *os.File
	var err error
	var canonicalErr error
	var filename string
	for i := 0; i < count; i++ {
		elem := cpvec.At(i)
		cpelem := coretypes.EnsureObjectIsString(elem, "*classpath*["+strconv.Itoa(i)+"]: %s")
		s := cpelem.S
		if s == "" {
			filename = pathname
		} else {
			filename = deps.ResolveLibPath(s, libname)
		}
		f, _, err = osutil.OpenRuneFile(filename)
		if err == nil {
			canonicalErr = nil
			break
		}
		if s == "" {
			canonicalErr = err
		}
	}
	PanicOnErr(canonicalErr)
	PanicOnErr(err)
	defer func() { PanicOnErr(f.Close()) }()
	reader := readerConstruction.NewReader(osutil.AsRuneReader(f), filename)
	ProcessReaderFromEval(reader, filename)
	return NIL
}

var procReduceKv = func(args []coretypes.Object) coretypes.Object {
	f := coretypes.EnsureArgIsCallable(args, 0)
	init := args[1]
	coll := coretypes.EnsureArgIsKVReduce(args, 2)
	return coll.KVReduce(f, init)
}

var procReduce = func(args []coretypes.Object) coretypes.Object {
	f := coretypes.EnsureArgIsCallable(args, 0)
	if len(args) == 2 {
		coll := coretypes.EnsureArgIsReduce(args, 1)
		return coll.Reduce(f)
	}
	init := args[1]
	coll := coretypes.EnsureArgIsReduce(args, 2)
	return coll.ReduceInit(f, init)
}

var procIndexOf = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0)
	ch := coretypes.EnsureArgIsChar(args, 1)
	for i, r := range s.S {
		if r == ch.Ch {
			return coretypes.Int{I: i}
		}
	}
	return coretypes.Int{I: -1}
}

func libExternalPath(sym coretypes.Symbol) (path string, ok bool) {
	nsSourcesVar, _ := GLOBAL_ENV.Resolve(coretypes.MakeSymbol(STRINGS.Intern, "joker.core/*ns-sources*"))
	nsSources := corecollections.ToSlice(nsSourcesVar.Value.(coretypes.Vec).Seq())

	var sourceKey string
	var sourceMap coretypes.Map
	for _, source := range nsSources {
		sourceKey = source.(coretypes.Vec).Nth(0).ToString(false)
		match, _ := regexp.MatchString(sourceKey, sym.Name())
		if match {
			sourceMap = source.(coretypes.Vec).Nth(1).(coretypes.Map)
			break
		}
	}
	if sourceMap != nil {
		ok, url := sourceMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "url"))
		if !ok {
			panic(RT.NewError("Key :url not found in ns-sources for: " + sourceKey))
		} else {
			path, err := deps.ExternalSourceToPath(osutil.HomeDir(), sym.Name(), url.ToString(false))
			PanicOnErr(err)
			return path, true
		}
	}
	return
}

var procLibPath = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	var path string

	path, ok := libExternalPath(sym)

	if !ok {
		var file string
		if GLOBAL_ENV.file.Value == nil {
			var err error
			file, err = osutil.Abs("user")
			PanicOnErr(err)
		} else {
			file = coretypes.EnsureObjectIsString(GLOBAL_ENV.file.Value, "").S
			file = osutil.ResolveSymlink(file)
		}
		ns := GLOBAL_ENV.CurrentNamespace().Name
		path = deps.ResolveRelativeLibPath(file, ns.Name(), sym.Name())
	}
	return coretypes.String{S: path}
}

var procInternFakeVar = func(args []coretypes.Object) coretypes.Object {
	nsSym := coretypes.EnsureArgIsSymbol(args, 0)
	sym := coretypes.EnsureArgIsSymbol(args, 1)
	isMacro := ToBool(args[2])
	res := InternFakeSymbol(GLOBAL_ENV.FindNamespace(nsSym), sym)
	res.isMacro = isMacro
	return res
}

var procParse = func(args []coretypes.Object) coretypes.Object {
	lm, _ := GLOBAL_ENV.Resolve(coretypes.MakeSymbol(STRINGS.Intern, "joker.core/*linter-mode*"))
	lm.Value = coretypes.Boolean{B: true}
	LINTER_MODE = true
	defer func() {
		LINTER_MODE = false
		lm.Value = coretypes.Boolean{B: false}
	}()
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	res := Parse(args[0], parseContext)
	return res.Dump(false)
}

var procTypes = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 0, 0)
	res := corecollections.EmptyArrayMap()
	for k, v := range TYPES {
		res.Add(coretypes.String{S: *k}, v)
	}
	return res
}

var procCreateChan = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	n := coretypes.EnsureArgIsInt(args, 0)
	ch := make(chan corert.FutureResult, n.I)
	return corert.NewObjectChannel(ch)
}

var procCloseChan = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	EnsureArgIsChannel(args, 0).Close()
	return NIL
}

var procSend = func(args []coretypes.Object) (obj coretypes.Object) {
	CheckArity(args, 2, 2)
	ch := EnsureArgIsChannel(args, 0)
	v := args[1]
	if v.Equals(NIL) {
		panic(RT.NewError("Can't put nil on channel"))
	}
	if ch.IsClosed() {
		return coretypes.MakeBoolean(false)
	}
	return coretypes.MakeBoolean(ch.Send(v))
}

var procReceive = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	ch := EnsureArgIsChannel(args, 0)
	value, status, err := ch.Receive(nil)
	if status == corert.ChannelReceiveClosed {
		return NIL
	}
	if err != nil {
		panic(coretypes.Object(err))
	}
	return value
}

var procGo = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	f := coretypes.EnsureArgIsCallable(args, 0)
	ch := corert.NewObjectChannel(make(chan corert.FutureResult, 1))
	go func() {
		registerGoroutineRT()
		defer unregisterGoroutineRT()

		defer func() {
			if r := recover(); r != nil {
				switch r := r.(type) {
				case coretypes.Error:
					ch.SendResult(corert.NewFutureResult(NIL, r))
					ch.Close()
				default:
					panic(r)
				}
			}
		}()

		res := call0(f)
		ch.SendResult(corert.NewFutureResult(res, nil))
		ch.Close()
	}()
	return ch
}

var procVerbosityLevel = func(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 0, 0)
	return coretypes.MakeInt(VerbosityLevel)
}

var procExit = func(args []coretypes.Object) coretypes.Object {
	ExitJoker(coretypes.EnsureArgIsInt(args, 0).I)
	return NIL
}

var procIsNaN = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0)
	return coretypes.Boolean{B: math.IsNaN(n.Double().D)}
}

var procAbs = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0)
	switch n := n.(type) {
	case coretypes.Double:
		return coretypes.Double{D: math.Abs(n.D)}
	case *coretypes.BigInt:
		b := &big.Int{}
		return &coretypes.BigInt{B: b.Abs(n.B)}
	case *coretypes.BigFloat:
		b := &big.Float{}
		return &coretypes.BigFloat{B: b.Abs(n.B)}
	case *coretypes.Ratio:
		r := &big.Rat{}
		return &coretypes.Ratio{R: r.Abs(n.R)}
	case coretypes.Int:
		x := n.I
		if x < 0 {
			x = -x
		}
		return coretypes.Int{I: x}
	}
	panic(FailArg(n, "coretypes.Number", 0))
}

var procIsInfinite = func(args []coretypes.Object) coretypes.Object {
	n := coretypes.EnsureArgIsNumber(args, 0)
	return coretypes.Boolean{B: math.IsInf(n.Double().D, 0)}
}

var procParseDouble = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0)
	d, err := numerical.ParseFloat64(s.S)
	if err != nil {
		return NIL
	}
	return coretypes.Double{D: d}
}

var procParseLong = func(args []coretypes.Object) coretypes.Object {
	s := coretypes.EnsureArgIsString(args, 0)
	i, err := numerical.ParseInt(s.S, 10, 64)
	if err != nil {
		return NIL
	}
	return coretypes.Int{I: int(i)}
}

func PackReader(reader *Reader, filename string) ([]byte, error) {
	var p []byte
	packEnv := NewPackEnv()
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	if filename != "" {
		currentFilename := parseContext.GlobalEnv.file.Value
		defer func() {
			parseContext.GlobalEnv.SetFilename(currentFilename)
		}()
		s, err := osutil.Abs(filename)
		PanicOnErr(err)
		parseContext.GlobalEnv.SetFilename(coretypes.MakeString(s))
	}
	for {
		obj, err := readerConstruction.TryRead(reader)
		if err == io.EOF {
			var hp []byte
			hp = packEnv.Pack(hp)
			return append(hp, p...), nil
		}
		if err != nil {
			fmt.Fprintln(Stderr, err)
			return nil, err
		}
		expr, err := TryParse(obj, parseContext)
		if err != nil {
			fmt.Fprintln(Stderr, err)
			return nil, err
		}
		p = expr.Pack(p, packEnv)
		_, err = TryEval(expr)
		if err != nil {
			fmt.Fprintln(Stderr, err)
			return nil, err
		}
	}
}

var procIncProblemCount = func(args []coretypes.Object) coretypes.Object {
	PROBLEM_COUNT++
	return NIL
}

func ProcessReader(reader *Reader, filename string, phase corereader.Phase) error {
	if phase == corereader.FormatPhase {
		FORMAT_MODE = true
		coretypes.FormatMode = true
		corecollections.HASHMAP_THRESHOLD = 100000
	}
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	if filename != "" {
		currentFilename := parseContext.GlobalEnv.file.Value
		defer func() {
			parseContext.GlobalEnv.SetFilename(currentFilename)
		}()
		s, err := osutil.Abs(filename)
		PanicOnErr(err)
		parseContext.GlobalEnv.SetFilename(coretypes.MakeString(s))
	}
	var prevObj coretypes.Object
	for {
		obj, err := readerConstruction.TryRead(reader)
		if err == io.EOF {
			if FORMAT_MODE && prevObj != nil {
				fmt.Fprint(Stdout, "\n")
			}
			return nil
		}
		if err != nil {
			fmt.Fprintln(Stderr, err)
			return err
		}
		if phase == corereader.ReadPhase {
			continue
		}
		if phase == corereader.FormatPhase {
			if prevObj != nil {
				cnt := newLineCount(prevObj, obj)
				for i := 0; i < cnt; i++ {
					fmt.Fprint(Stdout, "\n")
				}
				if cnt == 0 {
					fmt.Fprint(Stdout, " ")
				}
			}
			formatObject(obj, 0, Stdout)
			prevObj = obj
			continue
		}
		expr, err := TryParse(obj, parseContext)
		if err != nil {
			fmt.Fprintln(Stderr, err)
		}
		if phase == corereader.ParsePhase {
			continue
		}
		if err != nil {
			return err
		}
		obj, err = TryEval(expr)
		if err != nil {
			fmt.Fprintln(Stderr, err)
			return err
		}
		if phase == corereader.EvalPhase {
			continue
		}
		if _, ok := obj.(Nil); !ok {
			fmt.Fprintln(Stdout, obj.ToString(true))
		}
	}
}

func ProcessReaderFromEval(reader *Reader, filename string) {
	maybeOverrideRange()
	parseContext := &ParseContext{GlobalEnv: GLOBAL_ENV}
	if filename != "" {
		currentFilename := parseContext.GlobalEnv.file.Value
		defer func() {
			parseContext.GlobalEnv.SetFilename(currentFilename)
		}()
		s, err := osutil.Abs(filename)
		PanicOnErr(err)
		parseContext.GlobalEnv.SetFilename(coretypes.MakeString(s))
	}
	for {
		obj, err := readerConstruction.TryRead(reader)
		if err == io.EOF {
			return
		}
		PanicOnErr(err)
		expr, err := TryParse(obj, parseContext)
		PanicOnErr(err)
		_, err = TryEval(expr)
		PanicOnErr(err)
	}
}

var haveSetCoreNamespaces bool

func ProcessCoreData() {
	// Let MaybeLazy() handle initialization.
	if !haveSetCoreNamespaces {
		setCoreNamespaces()
		haveSetCoreNamespaces = true
	}
}

func ProcessReplData() {
	// Let MaybeLazy() handle initialization.
}

func ProcessLinterData(dialect corereader.Dialect) {
	if dialect == corereader.EDNDialect {
		markJokerNamespacesAsUsed()
		return
	}
	processGeneratedLinterPayload("linter_all.joke")
	if dialect == corereader.JokerDialect {
		markJokerNamespacesAsUsed()
		processGeneratedLinterPayload("linter_joker.joke")
		return
	}
	processGeneratedLinterPayload("linter_cljx.joke")
	switch dialect {
	case corereader.CLJDialect:
		processGeneratedLinterPayload("linter_clj.joke")
	case corereader.CLJSDialect:
		processGeneratedLinterPayload("linter_cljs.joke")
	}
}

func processGeneratedLinterPayload(path string) {
	data, ok := coregenerated.LinterDataByPath(path)
	if !ok {
		panic(RT.NewError("missing generated linter payload: " + path))
	}
	processData(data)
}

func processData(data []byte) {
	ns := GLOBAL_ENV.CurrentNamespace()
	GLOBAL_ENV.SetCurrentNamespace(GLOBAL_ENV.CoreNamespace)
	defer func() { GLOBAL_ENV.SetCurrentNamespace(ns) }()
	header, p := UnpackHeader(data, GLOBAL_ENV)
	for len(p) > 0 {
		var expr Expr
		expr, p = UnpackExpr(p, header)
		_, err := TryEval(expr)
		PanicOnErr(err)
	}
	if VerbosityLevel > 0 {
		fmt.Fprintf(Stderr, "processData: Evaluated code for %s\n", GLOBAL_ENV.CurrentNamespace().ToString(false))
	}
}

func setCoreNamespaces() {
	ns := GLOBAL_ENV.CoreNamespace
	ns.MaybeLazy("joker.core")

	vr := ns.Resolve("*core-namespaces*")
	set := vr.Value.(*corecollections.MapSet)
	for _, ns := range coregenerated.CoreNamespaces() {
		set = set.Conj(coretypes.MakeSymbol(STRINGS.Intern, ns)).(*corecollections.MapSet)
	}
	set = set.Conj(coretypes.MakeSymbol(STRINGS.Intern, "user")).(*corecollections.MapSet)
	vr.Value = set

	// Add 'joker.core to *loaded-libs*, now that it's loaded.
	vr = ns.Resolve("*loaded-libs*")
	set = vr.Value.(*corecollections.MapSet).Conj(ns.Name).(*corecollections.MapSet)
	vr.Value = set

	// Install runtime overrides that depend on core.joke vars existing.
	maybeOverrideRange()
	maybeOverrideSeqOps()
}

var procIsNamespaceInitialized = func(args []coretypes.Object) coretypes.Object {
	sym := coretypes.EnsureArgIsSymbol(args, 0)
	if sym.NamespaceKey() != nil {
		panic(RT.NewError("Can't ask for namespace info on namespace-qualified symbol"))
	}
	// First look for registered (e.g. std) libs
	ns, found := GLOBAL_ENV.Namespaces[sym.NameKey()]
	return coretypes.MakeBoolean(found && ns.Lazy == nil)
}

func findConfigFile(filename string, workingDir string, findDir bool) string {
	configName := ".joker"
	if findDir {
		configName = ".jokerd"
	}
	path, err := osutil.FindConfigPath(filename, workingDir, configName, osutil.HomeDir(), findDir)
	if err != nil {
		fmt.Fprintln(Stderr, "coretypes.Error reading config file "+filename+": ", err)
		return ""
	}
	return path
}

func printConfigError(filename, msg string) {
	fmt.Fprintln(Stderr, "coretypes.Error reading config file "+filename+": ", msg)
}

func knownMacrosToMap(km coretypes.Object) (coretypes.Map, error) {
	s := km.(coretypes.Seqable).Seq()
	res := corecollections.EmptyArrayMap()
	for !s.IsEmpty() {
		obj := s.First()
		switch obj := obj.(type) {
		case coretypes.Symbol:
			res.Add(obj, NIL)
		case coretypes.Vec:
			if obj.Count() != 2 {
				return nil, errors.New(":known-macros item must be a symbol or a vector with two elements")
			}
			res.Add(obj.At(0), obj.At(1))
		default:
			return nil, errors.New(":known-macros item must be a symbol or a vector, got " + obj.GetType().ToString(false))
		}
		s = s.Rest()
	}
	return res, nil
}

func ReadConfig(filename string, workingDir string) {
	LINTER_CONFIG = GLOBAL_ENV.CoreNamespace.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*linter-config*"))
	LINTER_CONFIG.Value = corecollections.EmptyArrayMap()
	configFileName := findConfigFile(filename, workingDir, false)
	if configFileName == "" {
		return
	}
	f, rr, err := osutil.OpenRuneFile(configFileName)
	if err != nil {
		printConfigError(configFileName, err.Error())
		return
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			printConfigError(configFileName, closeErr.Error())
		}
	}()
	r := readerConstruction.NewReader(rr, configFileName)
	config, err := readerConstruction.TryRead(r)
	if err != nil {
		printConfigError(configFileName, err.Error())
		return
	}
	configMap, ok := config.(coretypes.Map)
	if !ok {
		printConfigError(configFileName, "config root object must be a map, got "+config.GetType().ToString(false))
		return
	}
	ok, ignoredUnusedNamespaces := configMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "ignored-unused-namespaces"))
	if ok {
		seq, ok1 := ignoredUnusedNamespaces.(coretypes.Seqable)
		if ok1 {
			WARNINGS.ignoredUnusedNamespaces = corecollections.NewSetFromSeq(seq.Seq())
		} else {
			printConfigError(configFileName, ":ignored-unused-namespaces value must be a vector, got "+ignoredUnusedNamespaces.GetType().ToString(false))
			return
		}
	}
	ok, ignoredFileRegexes := configMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "ignored-file-regexes"))
	if ok {
		seq, ok1 := ignoredFileRegexes.(coretypes.Seqable)
		if ok1 {
			s := seq.Seq()
			for !s.IsEmpty() {
				regex, ok2 := s.First().(*coretypes.Regex)
				if !ok2 {
					printConfigError(configFileName, ":ignored-file-regexes elements must be regexes, got "+s.First().GetType().ToString(false))
					return
				}
				WARNINGS.IgnoredFileRegexes = append(WARNINGS.IgnoredFileRegexes, regex.R)
				s = s.Rest()
			}
		} else {
			printConfigError(configFileName, ":ignored-file-regexes value must be a vector, got "+ignoredFileRegexes.GetType().ToString(false))
			return
		}
	}
	ok, entryPoints := configMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "entry-points"))
	if ok {
		seq, ok1 := entryPoints.(coretypes.Seqable)
		if ok1 {
			WARNINGS.entryPoints = corecollections.NewSetFromSeq(seq.Seq())
		} else {
			printConfigError(configFileName, ":entry-points value must be a vector, got "+entryPoints.GetType().ToString(false))
			return
		}
	}
	ok, knownNamespaces := configMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "known-namespaces"))
	if ok {
		if _, ok1 := knownNamespaces.(coretypes.Seqable); !ok1 {
			printConfigError(configFileName, ":known-namespaces value must be a vector, got "+knownNamespaces.GetType().ToString(false))
			return
		}
	}
	ok, knownTags := configMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "known-tags"))
	if ok {
		if _, ok1 := knownTags.(coretypes.Seqable); !ok1 {
			printConfigError(configFileName, ":known-tags value must be a vector, got "+knownTags.GetType().ToString(false))
			return
		}
	}
	ok, knownMacros := configMap.Get(KEYWORDS.knownMacros)
	if ok {
		_, ok1 := knownMacros.(coretypes.Seqable)
		if !ok1 {
			printConfigError(configFileName, ":known-macros value must be a vector, got "+knownMacros.GetType().ToString(false))
			return
		}
		m, err := knownMacrosToMap(knownMacros)
		if err != nil {
			printConfigError(configFileName, err.Error())
			return
		}
		configMap = configMap.Assoc(KEYWORDS.knownMacros, m).(coretypes.Map)
	}
	ok, rules := configMap.Get(KEYWORDS.rules)
	if ok {
		m, ok := rules.(coretypes.Map)
		if !ok {
			printConfigError(configFileName, ":rules value must be a map, got "+rules.GetType().ToString(false))
			return
		}
		if ok, v := m.Get(KEYWORDS.ifWithoutElse); ok {
			WARNINGS.ifWithoutElse = ToBool(v)
		}
		if ok, v := m.Get(KEYWORDS.unusedFnParameters); ok {
			WARNINGS.unusedFnParameters = ToBool(v)
		}
		if ok, v := m.Get(KEYWORDS.fnWithEmptyBody); ok {
			WARNINGS.fnWithEmptyBody = ToBool(v)
		}
	}
	if ok, valid := configMap.Get(KEYWORDS.validIdent); ok {
		m, ok := valid.(coretypes.Map)
		if !ok {
			printConfigError(configFileName, ":valid-ident value must be a map, got "+valid.GetType().ToString(false))
			return
		}
		if ok, v := m.Get(KEYWORDS.characterSet); ok {
			switch {
			case v.Equals(KEYWORDS.core):
				SetIdentSetCore()
			case v.Equals(KEYWORDS.symbol):
				SetIdentSetSymbol()
			case v.Equals(KEYWORDS.visible):
				SetIdentSetVisible()
			case v.Equals(KEYWORDS.any):
				SetIdentSetAny()
			default:
				printConfigError(configFileName, ":character-set value (in :valid-ident) value must be :core, :symbol, :visible, or :any; got "+v.GetType().ToString(false)+" "+v.ToString(false))
				return
			}
		}
		if ok, v := m.Get(KEYWORDS.encodingRange); ok {
			switch {
			case v.Equals(KEYWORDS.unicode):
				SetIdentRangeUnicode()
			case v.Equals(KEYWORDS.ascii):
				SetIdentRangeASCII()
			case v.Equals(KEYWORDS.any):
				SetIdentRangeAny()
			default:
				printConfigError(configFileName, ":encoding-range value (in :valid-ident) value must be :unicode, :ascii, or :any; got "+v.GetType().ToString(false)+" "+v.ToString(false))
				return
			}
		}
	}
	LINTER_CONFIG.Value = configMap
}

func RemoveJokerNamespaces() {
	for k, ns := range GLOBAL_ENV.Namespaces {
		if ns != GLOBAL_ENV.CoreNamespace && corestr.HasJokerNamespacePrefix(*k) {
			delete(GLOBAL_ENV.Namespaces, k)
		}
	}
}

func markJokerNamespacesAsUsed() {
	for k, ns := range GLOBAL_ENV.Namespaces {
		if ns != GLOBAL_ENV.CoreNamespace && corestr.HasJokerNamespacePrefix(*k) {
			ns.isUsed = true
			ns.isGloballyUsed = true
		}
	}
}

func NewReaderFromFile(filename string) (*Reader, error) {
	data, err := osutil.ReadFileBytes(filename)
	if err != nil {
		fmt.Fprintln(Stderr, "coretypes.Error: ", err)
		return nil, err
	}
	return readerConstruction.NewReader(osutil.ByteRuneReader(data), filename), nil
}

func ProcessLinterFile(configDir string, filename string) {
	if linterFileName := osutil.ExistingChild(configDir, filename); linterFileName != "" {
		if reader, err := NewReaderFromFile(linterFileName); err == nil {
			ProcessReader(reader, linterFileName, corereader.EvalPhase)
		}
	}
}

func ProcessLinterFiles(dialect corereader.Dialect, filename string, workingDir string) {
	if dialect == corereader.EDNDialect {
		return
	}
	configDir := findConfigFile(filename, workingDir, true)
	if configDir == "" {
		return
	}
	if dialect == corereader.JokerDialect {
		ProcessLinterFile(configDir, "linter.joke")
		return
	}
	ProcessLinterFile(configDir, "linter.cljc")
	switch dialect {
	case corereader.CLJSDialect:
		ProcessLinterFile(configDir, "linter.cljs")
	case corereader.CLJDialect:
		ProcessLinterFile(configDir, "linter.clj")
	}
}
