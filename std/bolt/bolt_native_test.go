package bolt

import (
	"math"
	"path/filepath"
	"testing"

	. "github.com/rcarmo/go-joker/core"
	boltlib "go.etcd.io/bbolt"
)

func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestCreateBucketClosedDBPanics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := boltlib.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	expectPanic(t, func() {
		createBucket(db, "k")
	})
}

func TestEnsureArgIsBoltDBChecksMissingArg(t *testing.T) {
	expectPanic(t, func() { EnsureArgIsBoltDB(nil, 0) })
}

func TestNextSequencePromotesBeyondNativeInt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := boltlib.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if err := db.Update(func(tx *boltlib.Tx) error {
		b, e := tx.CreateBucketIfNotExists([]byte("k"))
		if e != nil {
			return e
		}
		return b.SetSequence(uint64(math.MaxInt64))
	}); err != nil {
		t.Fatalf("set sequence: %v", err)
	}
	got := nextSequence(db, "k")
	if math.MaxInt64 > int64(int(^uint(0)>>1)) && got.GetType() != TYPE.BigInt {
		t.Fatalf("nextSequence type = %s, want BigInt", got.GetType().ToString(false))
	}
}

func TestPutClosedDBPanics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := boltlib.Open(path, 0o600, nil)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Update(func(tx *boltlib.Tx) error {
		_, e := tx.CreateBucketIfNotExists([]byte("k"))
		return e
	}); err != nil {
		t.Fatalf("prep: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	expectPanic(t, func() {
		put(db, "k", "a", "b")
	})
}
