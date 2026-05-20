package collections

import coretypes "github.com/rcarmo/go-joker/core/types"

// CloneSlice returns a copy of src preserving its length and capacity.
func CloneSlice[T any](src []T) []T {
	if src == nil {
		return nil
	}
	dst := make([]T, len(src), cap(src))
	copy(dst, src)
	return dst
}

// AssocCopy returns a cloned slice with index i set to val.
func AssocCopy[T any](src []T, i int, val T) []T {
	dst := CloneSlice(src)
	dst[i] = val
	return dst
}

// FromValues returns a fresh slice containing vals.
func FromValues[T any](vals ...T) []T {
	if len(vals) == 0 {
		return nil
	}
	dst := make([]T, len(vals))
	copy(dst, vals)
	return dst
}

const vectorShift = 5
const vectorMask = 0x1f
const persistentVectorShift = 5
const persistentVectorMask = 0x1f

// TrieBranching is the branching factor used by persistent vector trie nodes.
const TrieBranching = 32

// TrieNode is a generic fixed-width trie node. Values are intentionally opaque
// so callers can store either child nodes or leaf values without importing root
// collection/object packages.
type TrieNode struct {
	arr [TrieBranching]any
}

// NewTrieNode returns an empty trie node.
func NewTrieNode() *TrieNode { return &TrieNode{} }

// Get returns the slot value at idx.
func (n *TrieNode) Get(idx int) any { return n.arr[idx] }

// Set assigns the slot value at idx.
func (n *TrieNode) Set(idx int, val any) { n.arr[idx] = val }

// CloneTrieNode returns a shallow copy of n.
func CloneTrieNode(n *TrieNode) *TrieNode {
	ret := &TrieNode{}
	if n != nil {
		*ret = *n
	}
	return ret
}

// NewTriePath creates a leftmost path of internal nodes ending in leaf.
func NewTriePath(level uint, shift uint, leaf *TrieNode) *TrieNode {
	if level == 0 {
		return leaf
	}
	ret := NewTrieNode()
	ret.Set(0, NewTriePath(level-shift, shift, leaf))
	return ret
}

func VectorTailOffset(count int) int {
	if count < TrieBranching {
		return 0
	}
	return ((count - 1) >> vectorShift) << vectorShift
}
func VectorArrayFor(i, count int, shift uint, root []interface{}, tail []interface{}) []interface{} {
	if i >= VectorTailOffset(count) {
		return tail
	}
	node := root
	for level := shift; level > 0; level -= vectorShift {
		node = node[(i>>level)&vectorMask].([]interface{})
	}
	return node
}
func VectorNewPath(level uint, node []interface{}) []interface{} {
	if level == 0 {
		return node
	}
	ret := make([]interface{}, TrieBranching)
	ret[0] = VectorNewPath(level-vectorShift, node)
	return ret
}
func VectorPushTail(level uint, count int, parent []interface{}, tailNode []interface{}) []interface{} {
	subIdx := ((count - 1) >> level) & vectorMask
	ret := CloneSlice(parent)
	if level == vectorShift {
		ret[subIdx] = tailNode
	} else {
		child := parent[subIdx]
		if child != nil {
			ret[subIdx] = VectorPushTail(level-vectorShift, count, child.([]interface{}), tailNode)
		} else {
			ret[subIdx] = VectorNewPath(level-vectorShift, tailNode)
		}
	}
	return ret
}
func VectorAssocNode(level uint, node []interface{}, i int, val interface{}) []interface{} {
	ret := CloneSlice(node)
	if level == 0 {
		ret[i&vectorMask] = val
	} else {
		subIdx := (i >> level) & vectorMask
		ret[subIdx] = VectorAssocNode(level-vectorShift, node[subIdx].([]interface{}), i, val)
	}
	return ret
}
func VectorPopTail(level uint, count int, node []interface{}) []interface{} {
	subIdx := ((count - 2) >> level) & vectorMask
	if level > vectorShift {
		newChild := VectorPopTail(level-vectorShift, count, node[subIdx].([]interface{}))
		if newChild == nil && subIdx == 0 {
			return nil
		}
		ret := CloneSlice(node)
		ret[subIdx] = newChild
		return ret
	}
	if subIdx == 0 {
		return nil
	}
	ret := CloneSlice(node)
	ret[subIdx] = nil
	return ret
}

func SeqFirst[T any](index, count int, at func(int) T) (T, bool) {
	if index < count {
		return at(index), true
	}
	var zero T
	return zero, false
}
func SeqRestIndex(index, count int) (int, bool) { next := index + 1; return next, next < count }
func SeqIsEmpty(index, count int) bool          { return index >= count }
func SeqIndexCount(index, count int) int {
	n := count - index
	if n < 0 {
		return 0
	}
	return n
}
func RSeqFirst[T any](index int, at func(int) T) (T, bool) {
	if index >= 0 {
		return at(index), true
	}
	var zero T
	return zero, false
}
func RSeqRestIndex(index int) (int, bool) { next := index - 1; return next, next >= 0 }
func RSeqIsEmpty(index int) bool          { return index < 0 }
func RSeqCount(index int) int {
	if index < 0 {
		return 0
	}
	return index + 1
}
func InterfacesFromValues[T any](values []T) []interface{} {
	res := make([]interface{}, len(values))
	for i, v := range values {
		res[i] = v
	}
	return res
}
func InterfaceNth(arr []interface{}, i int) (coretypes.Object, bool) {
	if i < 0 || i >= len(arr) {
		return nil, false
	}
	obj, _ := arr[i].(coretypes.Object)
	return obj, true
}

// AppendCopy returns a cloned slice with val appended.
func AppendCopy[T any](src []T, val T) []T {
	dst := CloneSlice(src)
	return append(dst, val)
}

// PopCopy returns a cloned slice with the last element removed.
func PopCopy[T any](src []T) []T {
	dst := CloneSlice(src)
	return dst[:len(dst)-1]
}

func VectorNth[T any](arr []T, i int) (T, bool) {
	if i < 0 || i >= len(arr) {
		var zero T
		return zero, false
	}
	return arr[i], true
}
func VectorPeek[T any](arr []T) (T, bool) {
	if len(arr) == 0 {
		var zero T
		return zero, false
	}
	return arr[len(arr)-1], true
}
func VectorAssoc[T any](arr []T, i int, val T) (next []T, appendMode bool, valid bool) {
	if i < 0 || i > len(arr) {
		return nil, false, false
	}
	if i == len(arr) {
		return nil, true, true
	}
	return AssocCopy(arr, i, val), false, true
}
func VectorAppendInPlace[T any](arr []T, obj T) []T { return append(arr, obj) }
func VectorConjoin[T any](arr []T, obj T, threshold int, overThreshold func() []T) []T {
	if len(arr) >= threshold {
		return overThreshold()
	}
	return AppendCopy(arr, obj)
}
func VectorConjoinCopy[T any](arr []T, obj T, threshold int) ([]T, bool) {
	if len(arr) >= threshold {
		return nil, true
	}
	return AppendCopy(arr, obj), false
}
func VectorCount[T any](arr []T) int { return len(arr) }
func VectorTryNth[T any](arr []T, i int, d T) T {
	if v, ok := VectorNth(arr, i); ok {
		return v
	}
	return d
}
func VectorPop[T any](arr []T) ([]T, bool) {
	if len(arr) == 0 {
		return nil, false
	}
	return PopCopy(arr), true
}

func VectorConjoinState(count int, shift uint, root []interface{}, tail []interface{}, obj interface{}, tailoff func(int) int, pushTail func(uint, []interface{}, []interface{}) []interface{}) (int, uint, []interface{}, []interface{}) {
	if count-tailoff(count) < 32 {
		return count + 1, shift, root, append(CloneSlice(tail), obj)
	}
	newShift := shift
	var newRoot []interface{}
	if (count >> 5) > (1 << shift) {
		newRoot = make([]interface{}, 32)
		newRoot[0] = root
		newRoot[1] = VectorNewPath(shift, tail)
		newShift += 5
	} else {
		newRoot = pushTail(shift, root, tail)
	}
	newTail := make([]interface{}, 1, 32)
	newTail[0] = obj
	return count + 1, newShift, newRoot, newTail
}
func VectorAssocState(i int, count int, tailoff int) (valid bool, appendMode bool, inRoot bool) {
	if i < 0 || i > count {
		return false, false, false
	}
	if i == count {
		return true, true, false
	}
	if i < tailoff {
		return true, false, true
	}
	return true, false, false
}

func PersistentVectorTailOffset(count int) int {
	if count < TrieBranching {
		return 0
	}
	return ((count - 1) >> persistentVectorShift) << persistentVectorShift
}
func PersistentVectorArrayFor[T any](count int, shift uint, root *TrieNode, tail []T, index int) []T {
	if index >= PersistentVectorTailOffset(count) {
		return tail
	}
	node := root
	for level := shift; level > 0; level -= persistentVectorShift {
		node = node.Get((index >> level) & persistentVectorMask).(*TrieNode)
	}
	leafStart := (index >> persistentVectorShift) << persistentVectorShift
	leafEnd := leafStart + TrieBranching
	tailOffset := PersistentVectorTailOffset(count)
	if leafEnd > tailOffset {
		leafEnd = tailOffset
	}
	result := make([]T, leafEnd-leafStart)
	for j := range result {
		result[j] = node.Get(j).(T)
	}
	return result
}
func PersistentVectorTrieOverflow(count int, shift uint) bool {
	return (count >> persistentVectorShift) > (1 << shift)
}
func PersistentVectorTailNode[T any](tail []T) *TrieNode {
	tailNode := NewTrieNode()
	for i, obj := range tail {
		tailNode.Set(i, obj)
	}
	return tailNode
}
func PersistentVectorPushTail(level uint, count int, parent *TrieNode, tailNode *TrieNode) *TrieNode {
	subIdx := ((count - 1) >> level) & persistentVectorMask
	ret := CloneTrieNode(parent)
	if level == persistentVectorShift {
		ret.Set(subIdx, tailNode)
	} else {
		child := parent.Get(subIdx)
		if child != nil {
			ret.Set(subIdx, PersistentVectorPushTail(level-persistentVectorShift, count, child.(*TrieNode), tailNode))
		} else {
			ret.Set(subIdx, NewTriePath(level-persistentVectorShift, persistentVectorShift, tailNode))
		}
	}
	return ret
}
func PersistentVectorAssocNode(level uint, node *TrieNode, index int, val any) *TrieNode {
	ret := CloneTrieNode(node)
	if level == 0 {
		ret.Set(index&persistentVectorMask, val)
	} else {
		subIdx := (index >> level) & persistentVectorMask
		ret.Set(subIdx, PersistentVectorAssocNode(level-persistentVectorShift, node.Get(subIdx).(*TrieNode), index, val))
	}
	return ret
}
func PersistentVectorPopTail(level uint, count int, node *TrieNode) *TrieNode {
	subIdx := ((count - 2) >> level) & persistentVectorMask
	if level > persistentVectorShift {
		newChild := PersistentVectorPopTail(level-persistentVectorShift, count, node.Get(subIdx).(*TrieNode))
		if newChild == nil && subIdx == 0 {
			return nil
		}
		ret := CloneTrieNode(node)
		ret.Set(subIdx, newChild)
		return ret
	}
	if subIdx == 0 {
		return nil
	}
	ret := CloneTrieNode(node)
	ret.Set(subIdx, nil)
	return ret
}
func PersistentVectorNth[T any](count int, tailOffset int, tail []T, shift uint, root *TrieNode, index int) (T, bool) {
	if index < 0 || index >= count {
		var zero T
		return zero, false
	}
	if index >= tailOffset {
		return tail[index-tailOffset], true
	}
	node := root
	for level := shift; level > 0; level -= persistentVectorShift {
		node = node.Get((index >> level) & persistentVectorMask).(*TrieNode)
	}
	return node.Get(index & persistentVectorMask).(T), true
}
func PersistentVectorToSlice[T any](count int, nth func(int) T) []T {
	result := make([]T, count)
	for i := 0; i < count; i++ {
		result[i] = nth(i)
	}
	return result
}
func PersistentVectorPopTailOnly(count int, tailOffset int) bool { return count-tailOffset > 1 }
func PersistentVectorPopShift(shift uint, root *TrieNode, minShift uint) (*TrieNode, uint, bool) {
	if root == nil {
		return nil, shift, true
	}
	if shift > minShift && root.Get(1) == nil {
		return root.Get(0).(*TrieNode), shift - minShift, false
	}
	return root, shift, false
}
