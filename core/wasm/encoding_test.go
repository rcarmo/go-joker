package wasm

import (
	"encoding/hex"
	"testing"
)

func TestAppendULEB(t *testing.T) {
	if got := hex.EncodeToString(AppendULEB(nil, 624485)); got != "e58e26" {
		t.Fatalf("AppendULEB(624485) = %s", got)
	}
}

func TestAppendSLEB(t *testing.T) {
	if got := hex.EncodeToString(AppendSLEB(nil, -123456)); got != "c0bb78" {
		t.Fatalf("AppendSLEB(-123456) = %s", got)
	}
}

func TestAppendF64(t *testing.T) {
	if got := hex.EncodeToString(AppendF64(nil, 1.5)); got != "000000000000f83f" {
		t.Fatalf("AppendF64(1.5) = %s", got)
	}
}

func TestValueTypeConstants(t *testing.T) {
	cases := map[string]byte{"i32": ValTypeI32, "i64": ValTypeI64, "f32": ValTypeF32, "f64": ValTypeF64}
	want := map[string]byte{"i32": 0x7f, "i64": 0x7e, "f32": 0x7d, "f64": 0x7c}
	for k, v := range cases {
		if v != want[k] {
			t.Fatalf("%s value type = %#x, want %#x", k, v, want[k])
		}
	}
}
