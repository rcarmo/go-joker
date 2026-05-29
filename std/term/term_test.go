package term

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func TestResetStyleReturnsString(t *testing.T) {
	result := procResetStyle(nil)
	s, ok := result.(coretypes.String)
	if !ok {
		t.Fatalf("expected String, got %T", result)
	}
	if s.S != "\033[0m" {
		t.Errorf("expected \\033[0m, got %q", s.S)
	}
}

func TestFgReturnsANSI(t *testing.T) {
	args := []coretypes.Object{
		makeTestVector(255, 0, 128),
	}
	result := procFg(args)
	s := result.(coretypes.String)
	expected := "\033[38;2;255;0;128m"
	if s.S != expected {
		t.Errorf("expected %q, got %q", expected, s.S)
	}
}

func TestBgReturnsANSI(t *testing.T) {
	args := []coretypes.Object{
		makeTestVector(0x1f, 0x29, 0x37),
	}
	result := procBg(args)
	s := result.(coretypes.String)
	expected := "\033[48;2;31;41;55m"
	if s.S != expected {
		t.Errorf("expected %q, got %q", expected, s.S)
	}
}

func TestMillisReturnsPositive(t *testing.T) {
	result := procMillis(nil)
	i, ok := result.(coretypes.Int)
	if !ok {
		t.Fatalf("expected Int, got %T", result)
	}
	if i.I <= 0 {
		t.Errorf("expected positive millis, got %d", i.I)
	}
}
