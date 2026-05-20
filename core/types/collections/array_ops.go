package collections

import (
	"fmt"
	"io"
	"strings"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

// FormatDelimited renders collection delimiters around caller-provided item
// strings. Root core supplies Object formatting; this package owns only the
// root-independent delimiter/separator mechanics.
func FormatDelimited(prefix string, suffix string, sep string, iter func(func(string) bool)) string {
	var b strings.Builder
	b.WriteString(prefix)
	first := true
	iter(func(item string) bool {
		if !first {
			b.WriteString(sep)
		}
		first = false
		b.WriteString(item)
		return true
	})
	b.WriteString(suffix)
	return b.String()
}

// FormatPairDelimited is the pair-oriented companion to FormatDelimited.
func FormatPairDelimited[K comparable, V any](prefix string, suffix string, pairSep string, entrySep string, iter func(func(Pair[K, V]) bool), formatKey func(K) string, formatValue func(V) string) string {
	return FormatDelimited(prefix, suffix, entrySep, func(yield func(string) bool) {
		iter(func(p Pair[K, V]) bool {
			return yield(formatKey(p.Key) + pairSep + formatValue(p.Value))
		})
	})
}

func MapIndexOf[T interface{ Equals(interface{}) bool }](arr []T, key T) int {
	for i := 0; i < len(arr); i += 2 {
		if arr[i].Equals(key) {
			return i
		}
	}
	return -1
}

func MapGet[T interface{ Equals(interface{}) bool }](arr []T, key T) (bool, T) {
	i := MapIndexOf(arr, key)
	if i != -1 {
		return true, arr[i+1]
	}
	var zero T
	return false, zero
}

func ArrayMapSet[T interface{ Equals(interface{}) bool }](arr []T, key T, value T) []T {
	i := MapIndexOf(arr, key)
	if i != -1 {
		arr[i+1] = value
		return arr
	}
	return append(arr, key, value)
}

func MapAdd[T interface{ Equals(interface{}) bool }](arr []T, key T, value T) ([]T, bool) {
	if MapIndexOf(arr, key) != -1 {
		return arr, false
	}
	return append(arr, key, value), true
}

func MapAssoc[T interface{ Equals(interface{}) bool }](arr []T, key T, value T, threshold int64) (next []T, useHash bool) {
	i := MapIndexOf(arr, key)
	if i != -1 {
		cp := CloneSlice(arr)
		cp[i+1] = value
		return cp, false
	}
	if int64(len(arr)) >= threshold {
		return nil, true
	}
	cp := CloneSlice(arr)
	return append(cp, key, value), false
}

func MapEntryAt[T interface{ Equals(interface{}) bool }](arr []T, key T) (T, bool) {
	i := MapIndexOf(arr, key)
	if i != -1 {
		return arr[i+1], true
	}
	var zero T
	return zero, false
}

func MapWithout[T interface{ Equals(interface{}) bool }](arr []T, key T) []T {
	result := make([]T, len(arr), cap(arr))
	var i, j int
	for i, j = 0, 0; i < len(arr); i += 2 {
		if arr[i].Equals(key) {
			continue
		}
		result[j], result[j+1] = arr[i], arr[i+1]
		j += 2
	}
	if i != j {
		result = result[:j]
	}
	return result
}

func MapKeys[T any](arr []T) []T {
	mlen := len(arr) / 2
	res := make([]T, mlen)
	for i := 0; i < mlen; i++ {
		res[i] = arr[i*2]
	}
	return res
}
func MapVals[T any](arr []T) []T {
	mlen := len(arr) / 2
	res := make([]T, mlen)
	for i := 0; i < mlen; i++ {
		res[i] = arr[i*2+1]
	}
	return res
}

func ArrayMapEquals[T interface{ Equals(interface{}) bool }](arr []T, other []T) bool {
	if len(arr) != len(other) {
		return false
	}
	for i := 0; i < len(arr); i += 2 {
		j := MapIndexOf(other, arr[i])
		if j < 0 || !arr[i+1].Equals(other[j+1]) {
			return false
		}
	}
	return true
}

func ArrayMapCount[T any](arr []T) int { return len(arr) / 2 }

func MapMergePairs[T interface{ Equals(interface{}) bool }](arr []T, pairs []coretypes.Pair, threshold int64) (next []T, useHash bool) {
	cp := CloneSlice(arr)
	for _, p := range pairs {
		cp = ArrayMapSet(cp, p.Key.(T), p.Value.(T))
		if int64(len(cp)) > threshold {
			return nil, true
		}
	}
	return cp, false
}

func MapFormatItems[T any](arr []T, isComment func(T) bool) []T {
	out := make([]T, 0, len(arr))
	for i := 0; i < len(arr); i++ {
		out = append(out, arr[i])
		if isComment(arr[i]) {
			i++
		}
	}
	return out
}

func ArrayMapFormat[T any](arr []T, w io.Writer, indent int, format func(T, int, io.Writer) int, maybeNewLine func(io.Writer, T, T, int, int) int, isComment func(T) bool, writeIndent func(io.Writer, int)) int {
	items := MapFormatItems(arr, isComment)
	ind := indent + 1
	fmt.Fprint(w, "{")
	if len(items) > 0 {
		for i := 0; i < len(items)-1; i++ {
			ind = format(items[i], ind, w)
			ind = maybeNewLine(w, items[i], items[i+1], indent+1, ind)
		}
		ind = format(items[len(items)-1], ind, w)
	}
	if len(items) > 0 && isComment(items[len(items)-1]) {
		fmt.Fprint(w, "\n")
		writeIndent(w, indent+1)
		ind = indent + 1
	}
	fmt.Fprint(w, "}")
	return ind + 1
}

func PairAt[T any](arr []T, i int) (key T, value T, ok bool) {
	if i >= 0 && i+1 < len(arr) {
		return arr[i], arr[i+1], true
	}
	var zero T
	return zero, zero, false
}
func NextPairIndex(i int) int                      { return i + 2 }
func PairIndexEmpty(i int, arrLen int) bool        { return i >= arrLen }
func IteratorHasNext(current int, arrLen int) bool { return current < arrLen }
func IteratorNextPair[T any](arr []T, current int) (key T, value T, next int) {
	key, value, _ = PairAt(arr, current)
	return key, value, current + 2
}
