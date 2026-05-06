package core

import (
	"testing"
)

func TestPVEmpty(t *testing.T) {
	pv := EmptyPersistentVector()
	if pv.Count() != 0 {
		t.Fatalf("expected count 0, got %d", pv.Count())
	}
}

func TestPVConjSmall(t *testing.T) {
	pv := EmptyPersistentVector()
	for i := 0; i < 10; i++ {
		pv = pv.Conj(MakeInt(i))
	}
	if pv.Count() != 10 {
		t.Fatalf("expected 10, got %d", pv.Count())
	}
	for i := 0; i < 10; i++ {
		v := pv.Nth(i).(Int).I
		if v != i {
			t.Fatalf("Nth(%d) = %d, want %d", i, v, i)
		}
	}
}

func TestPVConjBeyondTail(t *testing.T) {
	pv := EmptyPersistentVector()
	for i := 0; i < 100; i++ {
		pv = pv.Conj(MakeInt(i))
	}
	if pv.Count() != 100 {
		t.Fatalf("expected 100, got %d", pv.Count())
	}
	for i := 0; i < 100; i++ {
		v := pv.Nth(i).(Int).I
		if v != i {
			t.Fatalf("Nth(%d) = %d, want %d", i, v, i)
		}
	}
}

func TestPVConjLarge(t *testing.T) {
	pv := EmptyPersistentVector()
	n := 2000
	for i := 0; i < n; i++ {
		pv = pv.Conj(MakeInt(i))
	}
	if pv.Count() != n {
		t.Fatalf("expected %d, got %d", n, pv.Count())
	}
	for i := 0; i < n; i++ {
		v := pv.Nth(i).(Int).I
		if v != i {
			t.Fatalf("Nth(%d) = %d, want %d", i, v, i)
		}
	}
}

func TestPVAssocTail(t *testing.T) {
	pv := PersistentVectorFrom([]Object{MakeInt(0), MakeInt(1), MakeInt(2)})
	pv2 := pv.Assoc(1, MakeInt(99))
	// Original unchanged
	if pv.Nth(1).(Int).I != 1 {
		t.Fatal("original modified")
	}
	// New version has update
	if pv2.Nth(1).(Int).I != 99 {
		t.Fatalf("assoc failed: got %d", pv2.Nth(1).(Int).I)
	}
	// Other elements unchanged
	if pv2.Nth(0).(Int).I != 0 || pv2.Nth(2).(Int).I != 2 {
		t.Fatal("assoc corrupted other elements")
	}
}

func TestPVAssocInTrie(t *testing.T) {
	// Build a vector larger than 32 elements
	pv := EmptyPersistentVector()
	for i := 0; i < 50; i++ {
		pv = pv.Conj(MakeInt(i))
	}
	// Assoc in the trie portion (index < 32)
	pv2 := pv.Assoc(10, MakeInt(999))
	if pv.Nth(10).(Int).I != 10 {
		t.Fatal("original modified")
	}
	if pv2.Nth(10).(Int).I != 999 {
		t.Fatalf("trie assoc failed: got %d", pv2.Nth(10).(Int).I)
	}
	// Check structural sharing: other elements unchanged
	for i := 0; i < 50; i++ {
		if i == 10 {
			continue
		}
		if pv2.Nth(i).(Int).I != i {
			t.Fatalf("structural sharing broken at %d", i)
		}
	}
}

func TestPVAssocAtEnd(t *testing.T) {
	pv := PersistentVectorFrom([]Object{MakeInt(1), MakeInt(2), MakeInt(3)})
	// Assoc at count = conj
	pv2 := pv.Assoc(3, MakeInt(4))
	if pv2.Count() != 4 {
		t.Fatalf("expected 4, got %d", pv2.Count())
	}
	if pv2.Nth(3).(Int).I != 4 {
		t.Fatal("assoc at end failed")
	}
}

func TestPVStructuralSharing(t *testing.T) {
	// Build vector, create two versions via assoc
	pv := EmptyPersistentVector()
	for i := 0; i < 64; i++ {
		pv = pv.Conj(MakeInt(i))
	}
	v1 := pv.Assoc(5, MakeInt(500))
	v2 := pv.Assoc(40, MakeInt(400))

	// All three versions are independent
	if pv.Nth(5).(Int).I != 5 {
		t.Fatal("original corrupted")
	}
	if v1.Nth(5).(Int).I != 500 {
		t.Fatal("v1 wrong")
	}
	if v2.Nth(5).(Int).I != 5 {
		t.Fatal("v2 corrupted v1's change")
	}
	if v2.Nth(40).(Int).I != 400 {
		t.Fatal("v2 wrong")
	}
	if v1.Nth(40).(Int).I != 40 {
		t.Fatal("v1 corrupted by v2")
	}
}

func TestPVPop(t *testing.T) {
	pv := PersistentVectorFrom([]Object{MakeInt(1), MakeInt(2), MakeInt(3)})
	pv2 := pv.Pop()
	if pv2.Count() != 2 {
		t.Fatalf("expected 2, got %d", pv2.Count())
	}
	if pv2.Nth(0).(Int).I != 1 || pv2.Nth(1).(Int).I != 2 {
		t.Fatal("pop corrupted remaining elements")
	}
	if pv.Count() != 3 {
		t.Fatal("original modified by pop")
	}
}

func TestPVPopLarge(t *testing.T) {
	pv := EmptyPersistentVector()
	for i := 0; i < 100; i++ {
		pv = pv.Conj(MakeInt(i))
	}
	for i := 99; i >= 0; i-- {
		if pv.Count() != i+1 {
			t.Fatalf("count mismatch at pop %d: got %d", 100-i, pv.Count())
		}
		if pv.Nth(i).(Int).I != i {
			t.Fatalf("last element wrong before pop %d", 100-i)
		}
		pv = pv.Pop()
	}
	if pv.Count() != 0 {
		t.Fatal("not empty after all pops")
	}
}

func TestPVToSlice(t *testing.T) {
	items := []Object{MakeInt(10), MakeInt(20), MakeInt(30)}
	pv := PersistentVectorFrom(items)
	s := pv.ToSlice()
	if len(s) != 3 {
		t.Fatalf("expected 3, got %d", len(s))
	}
	for i, item := range items {
		if !s[i].Equals(item) {
			t.Fatalf("slice[%d] = %v, want %v", i, s[i], item)
		}
	}
}

func TestPVNthOutOfBounds(t *testing.T) {
	pv := PersistentVectorFrom([]Object{MakeInt(1)})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for out-of-bounds nth")
		}
	}()
	pv.Nth(5)
}

func TestPVAssocOutOfBounds(t *testing.T) {
	pv := PersistentVectorFrom([]Object{MakeInt(1)})
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for out-of-bounds assoc")
		}
	}()
	pv.Assoc(5, MakeInt(99))
}

func TestPVMultipleAssocSameBase(t *testing.T) {
	// Simulate n-body pattern: multiple assocs on same base vector
	pv := EmptyPersistentVector()
	for i := 0; i < 35; i++ {
		pv = pv.Conj(Double{D: float64(i)})
	}
	// Assoc 3 consecutive indices (like setting vx, vy, vz)
	v1 := pv.Assoc(3, Double{D: 100.0})
	v2 := v1.Assoc(4, Double{D: 200.0})
	v3 := v2.Assoc(5, Double{D: 300.0})

	// Original unchanged
	if pv.Nth(3).(Double).D != 3.0 {
		t.Fatal("original corrupted")
	}
	// Final has all three updates
	if v3.Nth(3).(Double).D != 100.0 || v3.Nth(4).(Double).D != 200.0 || v3.Nth(5).(Double).D != 300.0 {
		t.Fatal("chained assoc failed")
	}
}

// Benchmark: PersistentVector assoc vs ArrayVector assoc
func BenchmarkPVAssoc35(b *testing.B) {
	pv := EmptyPersistentVector()
	for i := 0; i < 35; i++ {
		pv = pv.Conj(Double{D: float64(i)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v := pv
		for j := 0; j < 9; j++ {
			v = v.Assoc(j*3+3, Double{D: float64(i + j)})
		}
		_ = v
	}
}

func BenchmarkArrayVectorAssoc35(b *testing.B) {
	arr := make([]Object, 35)
	for i := range arr {
		arr[i] = Double{D: float64(i)}
	}
	av := &ArrayVector{arr: arr}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v Associative = av
		for j := 0; j < 9; j++ {
			v = v.Assoc(MakeInt(j*3+3), Double{D: float64(i + j)})
		}
		_ = v
	}
}
