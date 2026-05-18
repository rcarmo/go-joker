package types

func SeqsEqual(seq1, seq2 Seq) bool {
	a := seq1
	b := seq2
	for {
		aEmpty := a == nil || a.IsEmpty()
		bEmpty := b == nil || b.IsEmpty()
		if aEmpty || bEmpty {
			return aEmpty == bEmpty
		}
		if !a.First().Equals(b.First()) {
			return false
		}
		a = a.Rest()
		b = b.Rest()
	}
}

func IsSeqEqual(seq Seq, other interface{}) bool {
	if seq == other {
		return true
	}
	if sequential, ok := other.(Sequential); ok {
		if seqable, ok := sequential.(Seqable); ok {
			return SeqsEqual(seq, seqable.Seq())
		}
	}
	return false
}
