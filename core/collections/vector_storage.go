package collections

// CloneSlice returns a copy of src preserving its length and capacity.
func CloneSlice[T any](src []T) []T {
	if src == nil {
		return nil
	}
	dst := make([]T, len(src), cap(src))
	copy(dst, src)
	return dst
}

// AppendCopy returns a cloned slice with val appended.
func AppendCopy[T any](src []T, val T) []T {
	dst := CloneSlice(src)
	return append(dst, val)
}

// AssocCopy returns a cloned slice with index i set to val.
func AssocCopy[T any](src []T, i int, val T) []T {
	dst := CloneSlice(src)
	dst[i] = val
	return dst
}

// PopCopy returns a cloned slice with the last element removed.
func PopCopy[T any](src []T) []T {
	dst := CloneSlice(src)
	return dst[:len(dst)-1]
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
