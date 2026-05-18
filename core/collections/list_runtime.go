package collections

// MaterializeListNode builds (or reuses) the list-node representation for a
// root list adapter without importing root object/runtime types.
func MaterializeListNode[T any](current *ListNode[T], count int, first T, rest *ListNode[T]) *ListNode[T] {
	if current != nil {
		return current
	}
	if count == 0 {
		return NewEmptyListNode(first)
	}
	return NewListNode(first, rest)
}

func ListSecond[T any](node *ListNode[T]) T {
	return node.Rest().First()
}

func ListThird[T any](node *ListNode[T]) T {
	return node.Rest().Rest().First()
}

func ListFourth[T any](node *ListNode[T]) T {
	return node.Rest().Rest().Rest().First()
}
