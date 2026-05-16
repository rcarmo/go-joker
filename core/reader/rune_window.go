package reader

type Window struct {
	arr   [5]rune
	start int // points to an element before the first one
	end   int // points to the last element
}

func (w *Window) Add(r rune) {
	w.end++
	if w.end == len(w.arr) {
		w.end = 0
	}
	w.arr[w.end] = r
	if w.end == w.start {
		w.start++
		if w.start == len(w.arr) {
			w.start = 0
		}
	}
}

func (w *Window) Size() int {
	if w.end >= w.start {
		return w.end - w.start
	}
	return len(w.arr) - (w.start - w.end)
}

func (w *Window) Top(i int) rune {
	if i >= w.Size() {
		panic("RuneWindow: index out of range")
	}
	index := w.end - i
	if index < 0 {
		index += len(w.arr)
	}
	return w.arr[index]
}
