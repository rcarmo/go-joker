package collections

import coretypes "github.com/rcarmo/go-joker/core/types"

// BitCount returns the number of set bits in n.
func BitCount(n int) int {
	var count int
	for n != 0 {
		count++
		n &= n - 1
	}
	return count
}

// HashMask returns the 5-bit trie index for hash at shift.
func HashMask(hash uint32, shift uint) uint32 {
	return (hash >> shift) & 0x01f
}

// Bitpos returns the bitmap position for hash at shift.
func Bitpos(hash uint32, shift uint) int {
	return 1 << HashMask(hash, shift)
}

// RemovePair returns a copy of pairs with the pair at pairIndex removed. The
// input slice is interpreted as [k0, v0, k1, v1, ...].
func RemovePair[T any](pairs []T, pairIndex int) []T {
	newPairs := make([]T, len(pairs)-2)
	copy(newPairs, pairs[:2*pairIndex])
	copy(newPairs[2*pairIndex:], pairs[2*(pairIndex+1):])
	return newPairs
}

// AppendPair returns a copy of pairs with key/value appended.
func AppendPair[T any](pairs []T, key T, val T) []T {
	newPairs := make([]T, len(pairs)+2)
	copy(newPairs, pairs)
	newPairs[len(pairs)] = key
	newPairs[len(pairs)+1] = val
	return newPairs
}

// InsertPair returns a copy of pairs with key/value inserted at pairIndex.
func InsertPair[T any](pairs []T, pairIndex int, key T, val T) []T {
	newPairs := make([]T, len(pairs)+2)
	insert := 2 * pairIndex
	copy(newPairs, pairs[:insert])
	newPairs[insert] = key
	newPairs[insert+1] = val
	copy(newPairs[insert+2:], pairs[insert:])
	return newPairs
}

// Assoc2Copy returns a cloned slice with indexes i and j set to valI and valJ.
func Assoc2Copy[T any](src []T, i int, valI T, j int, valJ T) []T {
	dst := CloneSlice(src)
	dst[i] = valI
	dst[j] = valJ
	return dst
}

// PackIndexedNodes compacts a sparse 32-way trie node array into bitmap-indexed
// [nil, node] pairs, omitting skipIndex. present reports whether a node slot is
// populated so callers can keep their node type private to the owning package.
func PackIndexedNodes[T any](nodes []T, skipIndex uint, present func(T) bool) (int, []interface{}) {
	packed := make([]interface{}, 0, 2*(len(nodes)-1))
	bitmap := 0
	for i, node := range nodes {
		if uint(i) == skipIndex || !present(node) {
			continue
		}
		packed = append(packed, nil, node)
		bitmap |= 1 << uint(i)
	}
	return bitmap, packed
}

func NodeIteratorNext(nextEntry *coretypes.Pair, nextIter Iterator[*coretypes.Pair], advance func() (*coretypes.Pair, Iterator[*coretypes.Pair], bool)) (*coretypes.Pair, Iterator[*coretypes.Pair], bool) {
	if nextEntry != nil {
		return nextEntry, nextIter, true
	}
	if nextIter != nil {
		ret := nextIter.Next()
		if !nextIter.HasNext() {
			nextIter = nil
		}
		return ret, nextIter, true
	}
	if ret, nested, ok := advance(); ok {
		return ret, nested, true
	}
	return nil, nextIter, false
}

type Iterator[T any] interface {
	HasNext() bool
	Next() T
}

func NodeArrayAdvance[T any](array []interface{}, i int, iterFor func(any) Iterator[T], makeEntry func(key any, value any) T) (nextI int, entry T, hasEntry bool, nested Iterator[T], hasNested bool) {
	for i < len(array) {
		key := array[i]
		nodeOrVal := array[i+1]
		i += 2
		if key != nil {
			return i, makeEntry(key, nodeOrVal), true, nil, false
		}
		if nodeOrVal != nil {
			iter := iterFor(nodeOrVal)
			if iter != nil && iter.HasNext() {
				return i, entry, false, iter, true
			}
		}
	}
	return i, entry, false, nil, false
}

func ArrayNodeIterHasNext[N any, T any](nodes []N, i int, nested Iterator[T], isNil func(N) bool, iterFor func(N) Iterator[T]) (nextI int, nextNested Iterator[T], has bool) {
	for {
		if nested != nil {
			if nested.HasNext() {
				return i, nested, true
			}
			nested = nil
		}
		if i >= len(nodes) {
			return i, nil, false
		}
		node := nodes[i]
		i++
		if !isNil(node) {
			nested = iterFor(node)
		}
	}
}

func NextArrayNodeSeq[N any, S any](nodes []N, i int, s S, hasSeq func(S) bool, nodeSeq func(N) S) (int, S, bool) {
	if hasSeq(s) {
		return i, s, true
	}
	for j := i; j < len(nodes); j++ {
		ns := nodeSeq(nodes[j])
		if hasSeq(ns) {
			return j + 1, ns, true
		}
	}
	var zero S
	return i, zero, false
}

func NextNodeSeq[S any](array []interface{}, i int, s S, hasSeq func(S) bool, nodeSeq func(any) (S, bool), hasKey func(any) bool) (int, S, bool) {
	if hasSeq(s) {
		return i, s, true
	}
	for j := i; j < len(array); j += 2 {
		if hasKey(array[j]) {
			var zero S
			return j, zero, true
		}
		if ns, ok := nodeSeq(array[j+1]); ok && hasSeq(ns) {
			return j + 2, ns, true
		}
	}
	var zero S
	return i, zero, false
}
