package core

import coretypes "github.com/rcarmo/go-joker/core/types"

// chunked_seq.go — Chunked sequence compatibility layer.
//
// Clojure uses chunked sequences internally for performance (processing
// elements in groups of 32). This provides the API surface for compatibility
// without the internal chunking optimization. Sequences remain unchunked
// but the chunk-* functions work correctly for code that uses them.

func init() {
	registerChunkedSeqProcs()
}

// ChunkBuffer is a mutable buffer for building chunks.
type ChunkBuffer struct {
	coretypes.InfoHolder
	MetaHolder
	arr   []Object
	count int
}

func (cb *ChunkBuffer) ToString(escape bool) string                { return "#object[ChunkBuffer]" }
func (cb *ChunkBuffer) Equals(other interface{}) bool              { return cb == other }
func (cb *ChunkBuffer) GetType() *coretypes.Type                   { return TYPE.ArrayVector }
func (cb *ChunkBuffer) Hash() uint32                               { return 0 }
func (cb *ChunkBuffer) WithInfo(info *coretypes.ObjectInfo) Object { return cb }
func (cb *ChunkBuffer) WithMeta(m Map) Object                      { return cb }

// ArrayChunk wraps a slice of objects as a chunk.
type ArrayChunk struct {
	coretypes.InfoHolder
	MetaHolder
	arr []Object
	off int
	end int
}

func (ac *ArrayChunk) ToString(escape bool) string                { return "#object[ArrayChunk]" }
func (ac *ArrayChunk) Equals(other interface{}) bool              { return ac == other }
func (ac *ArrayChunk) GetType() *coretypes.Type                   { return TYPE.ArrayVector }
func (ac *ArrayChunk) Hash() uint32                               { return 0 }
func (ac *ArrayChunk) WithInfo(info *coretypes.ObjectInfo) Object { return ac }
func (ac *ArrayChunk) WithMeta(m Map) Object                      { return ac }

func (ac *ArrayChunk) Count() int { return ac.end - ac.off }
func (ac *ArrayChunk) Nth(i int) Object {
	idx := ac.off + i
	if idx < 0 || idx >= ac.end {
		panic(RT.NewError("ArrayChunk index out of bounds"))
	}
	return ac.arr[idx]
}
func (ac *ArrayChunk) DropFirst() *ArrayChunk {
	if ac.off+1 >= ac.end {
		panic(RT.NewError("dropFirst on empty chunk"))
	}
	return &ArrayChunk{arr: ac.arr, off: ac.off + 1, end: ac.end}
}

// ChunkedCons wraps a chunk + rest seq for chunked-seq? compatibility.
type ChunkedCons struct {
	coretypes.InfoHolder
	MetaHolder
	chunk *ArrayChunk
	rest  Seq
	idx   int
}

func (cc *ChunkedCons) ToString(escape bool) string   { return SeqToString(cc, escape) }
func (cc *ChunkedCons) Equals(other interface{}) bool { return IsSeqEqual(cc, other) }
func (cc *ChunkedCons) GetType() *coretypes.Type      { return TYPE.LazySeq }
func (cc *ChunkedCons) Hash() uint32                  { return hashOrdered(cc) }
func (cc *ChunkedCons) WithInfo(info *coretypes.ObjectInfo) Object {
	res := *cc
	res.Info = info
	return &res
}
func (cc *ChunkedCons) WithMeta(m Map) Object {
	res := *cc
	res.meta = SafeMerge(res.meta, m)
	return &res
}
func (cc *ChunkedCons) Seq() Seq    { return cc }
func (cc *ChunkedCons) sequential() {}

func (cc *ChunkedCons) First() Object {
	return cc.chunk.Nth(cc.idx)
}

func (cc *ChunkedCons) Rest() Seq {
	if cc.idx+1 < cc.chunk.Count() {
		return &ChunkedCons{chunk: cc.chunk, rest: cc.rest, idx: cc.idx + 1}
	}
	if cc.rest != nil {
		return cc.rest
	}
	return EmptyList
}

func (cc *ChunkedCons) IsEmpty() bool {
	return false // ChunkedCons always has at least one element
}

func (cc *ChunkedCons) Cons(obj Object) Seq {
	return &ConsSeq{first: obj, rest: cc}
}

func registerChunkedSeqProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// chunk-buffer — (chunk-buffer size)
	cbVr := ns.Intern(MakeSymbol("chunk-buffer"))
	cbVr.Value = Proc{Name: "procChunkBuffer", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		n := EnsureArgIsInt(args, 0).I
		return &ChunkBuffer{arr: make([]Object, 0, n)}
	}}
	referToUser(MakeSymbol("chunk-buffer"), cbVr)

	// chunk-append — (chunk-append buffer val)
	caVr := ns.Intern(MakeSymbol("chunk-append"))
	caVr.Value = Proc{Name: "procChunkAppend", Fn: func(args []Object) Object {
		CheckArity(args, 2, 2)
		buf, ok := args[0].(*ChunkBuffer)
		if !ok {
			panic(RT.NewError("chunk-append requires a ChunkBuffer"))
		}
		buf.arr = append(buf.arr, args[1])
		buf.count++
		return NIL
	}}
	referToUser(MakeSymbol("chunk-append"), caVr)

	// chunk — (chunk buffer) — converts buffer to chunk
	chunkVr := ns.Intern(MakeSymbol("chunk"))
	chunkVr.Value = Proc{Name: "procChunk", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		buf, ok := args[0].(*ChunkBuffer)
		if !ok {
			panic(RT.NewError("chunk requires a ChunkBuffer"))
		}
		return &ArrayChunk{arr: buf.arr, off: 0, end: len(buf.arr)}
	}}
	referToUser(MakeSymbol("chunk"), chunkVr)

	// chunk-first — (chunk-first chunked-seq)
	cfVr := ns.Intern(MakeSymbol("chunk-first"))
	cfVr.Value = Proc{Name: "procChunkFirst", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		if cc, ok := args[0].(*ChunkedCons); ok {
			return cc.chunk
		}
		// For non-chunked seqs, wrap first element in a single-element chunk
		s := EnsureObjectIsSeqable(args[0], "chunk-first requires a seq").Seq()
		if s.IsEmpty() {
			return &ArrayChunk{arr: nil, off: 0, end: 0}
		}
		return &ArrayChunk{arr: []Object{s.First()}, off: 0, end: 1}
	}}
	referToUser(MakeSymbol("chunk-first"), cfVr)

	// chunk-rest — (chunk-rest chunked-seq)
	crVr := ns.Intern(MakeSymbol("chunk-rest"))
	crVr.Value = Proc{Name: "procChunkRest", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		if cc, ok := args[0].(*ChunkedCons); ok {
			if cc.rest != nil {
				return cc.rest
			}
			return EmptyList
		}
		s := EnsureObjectIsSeqable(args[0], "chunk-rest requires a seq").Seq()
		return s.Rest()
	}}
	referToUser(MakeSymbol("chunk-rest"), crVr)

	// chunk-next — (chunk-next chunked-seq)
	cnVr := ns.Intern(MakeSymbol("chunk-next"))
	cnVr.Value = Proc{Name: "procChunkNext", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		if cc, ok := args[0].(*ChunkedCons); ok {
			if cc.rest != nil && !cc.rest.IsEmpty() {
				return cc.rest
			}
			return NIL
		}
		s := EnsureObjectIsSeqable(args[0], "chunk-next requires a seq").Seq()
		r := s.Rest()
		if r.IsEmpty() {
			return NIL
		}
		return r
	}}
	referToUser(MakeSymbol("chunk-next"), cnVr)

	// chunk-cons — (chunk-cons chunk rest)
	ccVr := ns.Intern(MakeSymbol("chunk-cons"))
	ccVr.Value = Proc{Name: "procChunkCons", Fn: func(args []Object) Object {
		CheckArity(args, 2, 2)
		ac, ok := args[0].(*ArrayChunk)
		if !ok {
			panic(RT.NewError("chunk-cons requires an ArrayChunk as first argument"))
		}
		if ac.Count() == 0 {
			if args[1] == nil || IsNil(args[1]) {
				return EmptyList
			}
			if s, ok := args[1].(Seqable); ok {
				return s.Seq()
			}
			return EmptyList
		}
		var rest Seq
		if args[1] != nil && !IsNil(args[1]) {
			if s, ok := args[1].(Seqable); ok {
				rest = s.Seq()
			}
		}
		return &ChunkedCons{chunk: ac, rest: rest, idx: 0}
	}}
	referToUser(MakeSymbol("chunk-cons"), ccVr)

	// Override chunked-seq? to detect ChunkedCons
	csqVr := ns.Resolve("chunked-seq?")
	if csqVr != nil {
		csqVr.Value = Proc{Name: "procChunkedSeqQ", Fn: func(args []Object) Object {
			CheckArity(args, 1, 1)
			_, ok := args[0].(*ChunkedCons)
			return MakeBoolean(ok)
		}}
	}
}
