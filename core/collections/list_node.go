package collections

// ListNode is root-independent persistent list storage. Root core owns Object,
// metadata, sequence protocols, printing, hashing, and error behavior.
type ListNode[T any] struct {
	first T
	rest  *ListNode[T]
	count int
}

func NewEmptyListNode[T any](zero T) *ListNode[T] {
	n := &ListNode[T]{first: zero}
	n.rest = n
	return n
}

func NewListNode[T any](first T, rest *ListNode[T]) *ListNode[T] {
	n := &ListNode[T]{first: first, rest: rest}
	if rest != nil {
		n.count = rest.count + 1
	}
	return n
}

func (n *ListNode[T]) First() T           { return n.first }
func (n *ListNode[T]) Rest() *ListNode[T] { return n.rest }
func (n *ListNode[T]) Count() int         { return n.count }
func (n *ListNode[T]) IsEmpty() bool      { return n == nil || n.count == 0 }
