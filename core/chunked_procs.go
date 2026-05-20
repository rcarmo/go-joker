package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"
)

func init() {
	registerChunkedSeqProcs()
}

func registerChunkedSeqProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	cbVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-buffer"))
	cbVr.Value = Proc{Name: "procChunkBuffer", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		n := coretypes.EnsureArgIsInt(args, 0).I
		return &corecollections.ChunkBuffer{Arr: make([]coretypes.Object, 0, n)}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-buffer"), cbVr)

	caVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-append"))
	caVr.Value = Proc{Name: "procChunkAppend", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		buf, ok := args[0].(*corecollections.ChunkBuffer)
		if !ok {
			panic(coretypes.RuntimeError("chunk-append requires a ChunkBuffer"))
		}
		buf.Arr, buf.CountN = corecollections.ChunkAppend(buf.Arr, args[1])
		return coretypes.RuntimeNil
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-append"), caVr)

	chunkVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk"))
	chunkVr.Value = Proc{Name: "procChunk", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		buf, ok := args[0].(*corecollections.ChunkBuffer)
		if !ok {
			panic(coretypes.RuntimeError("chunk requires a ChunkBuffer"))
		}
		return &corecollections.ArrayChunk{Arr: buf.Arr, Off: 0, End: len(buf.Arr)}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk"), chunkVr)

	cfVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-first"))
	cfVr.Value = Proc{Name: "procChunkFirst", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		if cc, ok := args[0].(*corecollections.ChunkedCons); ok {
			return cc.Chunk
		}
		s := coretypes.EnsureObjectIsSeqable(args[0], "chunk-first requires a seq").Seq()
		arr := corecollections.ChunkFirstSingle(s)
		return &corecollections.ArrayChunk{Arr: arr, Off: 0, End: len(arr)}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-first"), cfVr)

	crVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-rest"))
	crVr.Value = Proc{Name: "procChunkRest", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		if cc, ok := args[0].(*corecollections.ChunkedCons); ok {
			return corecollections.ChunkRestFromRest(cc.RestSeq, corecollections.EmptyList)
		}
		s := coretypes.EnsureObjectIsSeqable(args[0], "chunk-rest requires a seq").Seq()
		return s.Rest()
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-rest"), crVr)

	cnVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-next"))
	cnVr.Value = Proc{Name: "procChunkNext", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		if cc, ok := args[0].(*corecollections.ChunkedCons); ok {
			return corecollections.ChunkNextFromRest(cc.RestSeq, coretypes.RuntimeNil)
		}
		s := coretypes.EnsureObjectIsSeqable(args[0], "chunk-next requires a seq").Seq()
		r := s.Rest()
		if r.IsEmpty() {
			return coretypes.RuntimeNil
		}
		return r
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-next"), cnVr)

	ccVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "chunk-cons"))
	ccVr.Value = Proc{Name: "procChunkCons", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		ac, ok := args[0].(*corecollections.ArrayChunk)
		if !ok {
			panic(coretypes.RuntimeError("chunk-cons requires an ArrayChunk as first argument"))
		}
		if ac.Count() == 0 {
			if args[1] == nil || IsNil(args[1]) {
				return corecollections.EmptyList
			}
			if s, ok := args[1].(coretypes.Seqable); ok {
				return s.Seq()
			}
			return corecollections.EmptyList
		}
		rest := corecollections.ChunkConsRest(args[1], IsNil)
		return &corecollections.ChunkedCons{Chunk: ac, RestSeq: rest, Idx: 0}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "chunk-cons"), ccVr)

	csqVr := ns.Resolve("chunked-seq?")
	if csqVr != nil {
		csqVr.Value = Proc{Name: "procChunkedSeqQ", Fn: func(args []coretypes.Object) coretypes.Object {
			runtimeCheckArity(args, 1, 1)
			_, ok := args[0].(*corecollections.ChunkedCons)
			return coretypes.MakeBoolean(ok)
		}}
	}
}
