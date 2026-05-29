package term

import (
	"testing"
	"time"

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

func TestReadKeyTimeoutReturnsNone(t *testing.T) {
	result := readKey(50*time.Millisecond, func(timeout time.Duration) (byte, bool, error) {
		return 0, false, nil
	}, nil)
	kw, ok := result.(coretypes.Keyword)
	if !ok {
		t.Fatalf("expected Keyword, got %T", result)
	}
	if kw.Name() != "none" {
		t.Fatalf("expected :none, got %s", kw.ToString(false))
	}
}

func TestReadKeyArrowSequence(t *testing.T) {
	bytes := []byte{27, '[', 'A'}
	result := readKey(50*time.Millisecond, func(timeout time.Duration) (byte, bool, error) {
		if len(bytes) == 0 {
			return 0, false, nil
		}
		b := bytes[0]
		bytes = bytes[1:]
		return b, true, nil
	}, nil)
	kw, ok := result.(coretypes.Keyword)
	if !ok {
		t.Fatalf("expected Keyword, got %T", result)
	}
	if kw.Name() != "up" {
		t.Fatalf("expected :up, got %s", kw.ToString(false))
	}
}

func TestReadKeyEscUnreadNonSequenceByte(t *testing.T) {
	bytes := []byte{27, 'x'}
	var unread []byte
	result := readKey(50*time.Millisecond, func(timeout time.Duration) (byte, bool, error) {
		if len(bytes) == 0 {
			return 0, false, nil
		}
		b := bytes[0]
		bytes = bytes[1:]
		return b, true, nil
	}, func(b byte) {
		unread = append(unread, b)
	})
	kw, ok := result.(coretypes.Keyword)
	if !ok {
		t.Fatalf("expected Keyword, got %T", result)
	}
	if kw.Name() != "esc" {
		t.Fatalf("expected :esc, got %s", kw.ToString(false))
	}
	if len(unread) != 1 || unread[0] != 'x' {
		t.Fatalf("expected unread byte 'x', got %v", unread)
	}
}
