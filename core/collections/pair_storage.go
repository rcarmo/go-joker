package collections

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
