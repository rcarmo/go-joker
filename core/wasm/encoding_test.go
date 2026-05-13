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
