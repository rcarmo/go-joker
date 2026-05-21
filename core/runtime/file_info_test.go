package runtime

import (
	"math"
	"os"
	"testing"
	"time"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type testFileInfo struct{ size int64 }

func (t testFileInfo) Name() string       { return "test" }
func (t testFileInfo) Size() int64        { return t.size }
func (t testFileInfo) Mode() os.FileMode  { return 0644 }
func (t testFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (t testFileInfo) IsDir() bool        { return false }
func (t testFileInfo) Sys() any           { return nil }

func TestFileInfoMapPromotesLargeSize(t *testing.T) {
	keys := map[string]*string{}
	intern := func(s string) *string {
		if keys[s] == nil {
			v := s
			keys[s] = &v
		}
		return keys[s]
	}
	m := FileInfoMap("contract", testFileInfo{size: math.MaxInt64}, intern)
	ok, size := m.Get(coretypes.MakeKeyword(intern, "size"))
	if !ok {
		t.Fatal("FileInfoMap missing :size")
	}
	if math.MaxInt64 > int64(coretypes.MaxInt) {
		if _, ok := size.(*coretypes.BigInt); !ok {
			t.Fatalf("FileInfoMap size type = %T, want *coretypes.BigInt", size)
		}
		return
	}
	if _, ok := size.(coretypes.Int); !ok {
		t.Fatalf("FileInfoMap size type = %T, want coretypes.Int", size)
	}
}
