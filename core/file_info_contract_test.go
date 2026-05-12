package core

import (
	"io/fs"
	"math"
	"testing"
	"time"
)

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
