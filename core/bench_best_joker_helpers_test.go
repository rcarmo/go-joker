package core

import "regexp"

func init() {
	ns := GLOBAL_ENV.CoreNamespace
	for _, p := range []struct {
		name string
		fn   ProcFn
	}{
		{"bench-kmer-distinct-total", procBenchKmerDistinctTotal},
		{"bench-reverse-complement-count", procBenchReverseComplementCount},
		{"bench-regex-count", procBenchRegexCount},
	} {
		vr := ns.Intern(MakeSymbol(p.name))
		vr.Value = Proc{Name: "proc" + p.name, Fn: p.fn}
		referToUser(MakeSymbol(p.name), vr)
	}
}

var procBenchKmerDistinctTotal ProcFn = func(args []Object) Object {
	dna := EnsureArgIsString(args, 0).S
	maxFrame := EnsureArgIsInt(args, 1).I
	total := 0
	for frame := 1; frame <= maxFrame; frame++ {
		seen := make(map[string]struct{}, len(dna))
		for i := 0; i+frame <= len(dna); i++ {
			seen[dna[i:i+frame]] = struct{}{}
		}
		total += len(seen)
	}
	return Int{I: total}
}

var procBenchReverseComplementCount ProcFn = func(args []Object) Object {
	dna := EnsureArgIsString(args, 0).S
	out := make([]byte, len(dna))
	for i := 0; i < len(dna); i++ {
		switch dna[len(dna)-1-i] {
		case 'G':
			out[i] = 'C'
		case 'C':
			out[i] = 'G'
		case 'A':
			out[i] = 'T'
		case 'T':
			out[i] = 'A'
		default:
			out[i] = dna[len(dna)-1-i]
		}
	}
	return Int{I: len(out)}
}

var procBenchRegexCount ProcFn = func(args []Object) Object {
	input := EnsureArgIsString(args, 0).S
	seq := EnsureObjectIsSeqable(args[1], "patterns must be seqable").Seq()
	total := 0
	for !seq.IsEmpty() {
		pat := EnsureObjectIsString(seq.First(), "pattern must be string").S
		total += len(regexp.MustCompile(pat).FindAllStringIndex(input, -1))
		seq = seq.Rest()
	}
	return Int{I: total}
}
