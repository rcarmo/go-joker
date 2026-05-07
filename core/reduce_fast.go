package core

// reduce_fast.go — Seq-walking reduce fallback + IntRange creation at reduce time.

func seqReduceInit(s Seq, f Callable, init Object) Object {
	acc := init
	for !s.IsEmpty() {
		acc = f.Call([]Object{acc, s.First()})
		if isReducedObj(acc) {
			return unreducedObj(acc)
		}
		s = s.Rest()
	}
	return acc
}

func seqReduce(s Seq, f Callable) Object {
	if s.IsEmpty() {
		return f.Call(nil)
	}
	acc := s.First()
	s = s.Rest()
	for !s.IsEmpty() {
		acc = f.Call([]Object{acc, s.First()})
		if isReducedObj(acc) {
			return unreducedObj(acc)
		}
		s = s.Rest()
	}
	return acc
}

func isNilObject(obj Object) bool {
	if obj == nil {
		return true
	}
	_, ok := obj.(Nil)
	return ok
}

// LazySeq Reduce support — implements the Reduce interface so (reduce f init lazy-seq) works.
func (seq *LazySeq) reduce(f Callable) Object {
	return seqReduce(seq.Seq(), f)
}

func (seq *LazySeq) reduceInit(f Callable, init Object) Object {
	return seqReduceInit(seq.Seq(), f, init)
}

// ConsSeq Reduce support
func (seq *ConsSeq) reduce(f Callable) Object {
	return seqReduce(seq, f)
}

func (seq *ConsSeq) reduceInit(f Callable, init Object) Object {
	return seqReduceInit(seq, f, init)
}

// MappingSeq Reduce support
func (seq *MappingSeq) reduce(f Callable) Object {
	return seqReduce(seq, f)
}

func (seq *MappingSeq) reduceInit(f Callable, init Object) Object {
	return seqReduceInit(seq, f, init)
}
