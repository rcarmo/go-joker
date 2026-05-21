package io

import (
	corert "github.com/rcarmo/go-joker/core/runtime"
	"io"
	"math/big"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"
)

func copyCountObject(n int64) coretypes.Object {
	maxNativeInt := int64(int(^uint(0) >> 1))
	if n > maxNativeInt {
		return coretypes.MakeBigInt(big.NewInt(n))
	}
	return coretypes.MakeInt(int(n))
}

func pipe() coretypes.Object {
	r, w := io.Pipe()
	res := corecollections.EmptyVector()
	res = res.Conjoin(corert.MakeIOReader(r))
	res = res.Conjoin(corert.MakeIOWriter(w))
	return res
}

func close(f coretypes.Object) Nil {
	if c, ok := f.(io.Closer); ok {
		if err := c.Close(); err != nil {
			panic(RT.NewError(err.Error()))
		}
		return NIL
	}
	panic(RT.NewError("coretypes.Object is not closable: " + f.ToString(false)))
}

func read(r io.Reader, n int) string {
	buf := make([]byte, n)
	cnt, err := r.Read(buf)
	if err != io.EOF {
		PanicOnErr(err)
	}
	return string(buf[:cnt])
}
