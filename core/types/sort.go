package types

// NamedSlice sorts named values by their printed name.
type NamedSlice[T Named] []T

func (s NamedSlice[T]) Len() int           { return len(s) }
func (s NamedSlice[T]) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s NamedSlice[T]) Less(i, j int) bool { return namedString(s[i]) < namedString(s[j]) }

func namedString(n Named) string {
	if ns := n.Namespace(); ns != "" {
		return ns + "/" + n.Name()
	}
	return n.Name()
}

// ComparatorSlice sorts objects using a Joker comparator.
type ComparatorSlice[T Object] struct {
	Items []T
	Cmp   Comparator
}

func (s ComparatorSlice[T]) Len() int      { return len(s.Items) }
func (s ComparatorSlice[T]) Swap(i, j int) { s.Items[i], s.Items[j] = s.Items[j], s.Items[i] }
func (s ComparatorSlice[T]) Less(i, j int) bool {
	return s.Cmp.Compare(s.Items[i], s.Items[j]) == -1
}
