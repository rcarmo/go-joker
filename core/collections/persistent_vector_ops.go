package collections

const persistentVectorShift = 5
const persistentVectorMask = 0x1f

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
