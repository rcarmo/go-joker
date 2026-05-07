package bolt

import (
	"path/filepath"
	"testing"

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
