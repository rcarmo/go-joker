package io

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"
	"math/big"

	. "github.com/rcarmo/go-joker/core"
)

func copyCountObject(n int64) Object {
	maxNativeInt := int64(int(^uint(0) >> 1))
	if n > maxNativeInt {
		return MakeBigInt(big.NewInt(n))
	}
	return coretypes.MakeInt(int(n))
}

func pipe() Object {
	r, w := io.Pipe()
	res := EmptyVector()
	res = res.Conjoin(MakeIOReader(r))
	res = res.Conjoin(MakeIOWriter(w))
	return res
}

func close(f Object) Nil {
	if c, ok := f.(io.Closer); ok {
		if err := c.Close(); err != nil {
			panic(RT.NewError(err.Error()))
		}
		return NIL
	}
	panic(RT.NewError("Object is not closable: " + f.ToString(false)))
}

func read(r io.Reader, n int) string {
	buf := make([]byte, n)
	cnt, err := r.Read(buf)
	if err != io.EOF {
		PanicOnErr(err)
	}
	return string(buf[:cnt])
}
