package core

import (
	"io/fs"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"
	"time"
)

func requirePanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatalf("%s did not panic", name)
		}
	}()
	fn()
}

func TestBitOpsRejectInvalidIndexesAndShifts(t *testing.T) {
	requirePanic(t, "negative bit index", func() { procBitSet([]Object{MakeInt(0), MakeInt(-1)}) })
	requirePanic(t, "too-large bit index", func() { procBitTest([]Object{MakeInt(0), MakeInt(strconv.IntSize)}) })
	requirePanic(t, "negative shift count", func() { procBitShiftLeft([]Object{MakeInt(1), MakeInt(-1)}) })
}

func TestIntArithmeticPromotesToBigIntOnOverflow(t *testing.T) {
	if got := procAdd([]Object{Int{I: maxInt}, Int{I: 1}}); got.GetType() != TYPE.BigInt || got.ToString(false) != "9223372036854775808N" {
		t.Fatalf("add promotion mismatch: %T %s", got, got.ToString(false))
	}
	if got := procSubtract([]Object{Int{I: minInt}, Int{I: 1}}); got.GetType() != TYPE.BigInt || got.ToString(false) != "-9223372036854775809N" {
		t.Fatalf("subtract promotion mismatch: %T %s", got, got.ToString(false))
	}
	if got := procMultiply([]Object{Int{I: maxInt}, Int{I: 2}}); got.GetType() != TYPE.BigInt || got.ToString(false) != "18446744073709551614N" {
		t.Fatalf("multiply promotion mismatch: %T %s", got, got.ToString(false))
	}
}

func TestIncDecPromoteToBigIntOnOverflow(t *testing.T) {
	if got := procInc([]Object{Int{I: maxInt}}); got.GetType() != TYPE.BigInt {
		t.Fatalf("inc did not promote: %T %s", got, got.ToString(false))
	}
	if got := procDec([]Object{Int{I: minInt}}); got.GetType() != TYPE.BigInt {
		t.Fatalf("dec did not promote: %T %s", got, got.ToString(false))
	}
}

func TestBigDecimalArithmeticKeepsBigFloat(t *testing.T) {
	a, _ := MakeBigFloatWithOrig("0.1", "0.1M")
	b, _ := MakeBigFloatWithOrig("0.2", "0.2M")
	got := procAdd([]Object{a, b})
	if got.GetType() != TYPE.BigFloat || !strings.HasPrefix(got.ToString(false), "0.3") || !strings.HasSuffix(got.ToString(false), "M") {
		t.Fatalf("big decimal add mismatch: %T %s", got, got.ToString(false))
	}
}

func TestBigIntIntPanicsOutsideNativeRange(t *testing.T) {
	tooLarge := MakeBigInt(new(big.Int).Add(maxIntBig, big.NewInt(1)))
	defer func() {
		if recover() == nil {
			t.Fatal("BigInt.Int should panic outside native int range")
		}
	}()
	_ = tooLarge.Int()
}

func TestBigIntIntConvertsWithinNativeRange(t *testing.T) {
	got := MakeBigInt(big.NewInt(42)).Int()
	if got.I != 42 {
		t.Fatalf("BigInt.Int = %d, want 42", got.I)
	}
}

func TestBigIntDoubleUsesFullMagnitude(t *testing.T) {
	large := MakeBigInt(new(big.Int).Lsh(big.NewInt(1), 70))
	got := large.Double().D
	want := math.Pow(2, 70)
	if got != want {
		t.Fatalf("BigInt.Double = %.0f, want %.0f", got, want)
	}
}

type contractFileInfo struct {
	size int64
}

func (fi contractFileInfo) Name() string       { return "contract" }
func (fi contractFileInfo) Size() int64        { return fi.size }
func (fi contractFileInfo) Mode() fs.FileMode  { return 0o644 }
func (fi contractFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (fi contractFileInfo) IsDir() bool        { return false }
func (fi contractFileInfo) Sys() any           { return nil }

func TestFileInfoMapPromotesLargeSize(t *testing.T) {
	m := FileInfoMap("contract", contractFileInfo{size: math.MaxInt64})
	found, got := m.Get(MakeKeyword("size"))
	if !found {
		t.Fatal("FileInfoMap missing :size")
	}
	if math.MaxInt64 > int64(maxInt) {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("file size type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("file size type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestRatioOrIntUsesNativeIntRange(t *testing.T) {
	tooLargeFor32Bit := new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 40))
	got := ratioOrInt(tooLargeFor32Bit)
	if strconv.IntSize == 32 {
		if got.GetType() != TYPE.BigInt {
			t.Fatalf("32-bit ratio integer promotion type = %s, want BigInt", got.GetType().ToString(false))
		}
		return
	}
	if got.GetType() != TYPE.Int {
		t.Fatalf("64-bit ratio integer promotion type = %s, want Int", got.GetType().ToString(false))
	}
}

func TestRatioOrIntWithOriginalPreservesBigIntOriginal(t *testing.T) {
	tooLarge := new(big.Rat).SetInt(new(big.Int).Lsh(big.NewInt(1), 70))
	got := ratioOrIntWithOriginal("1180591620717411303424/1", tooLarge)
	bi, ok := got.(*BigInt)
	if !ok {
		t.Fatalf("large ratio integer type = %T, want *BigInt", got)
	}
	if bi.Original != "1180591620717411303424/1" {
		t.Fatalf("BigInt original = %q", bi.Original)
	}
}
