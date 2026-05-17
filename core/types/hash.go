package types

import "github.com/rcarmo/go-joker/core/hashutil"

type Hash32 interface {
	Write([]byte) (int, error)
	Sum32() uint32
}

func NewHash32() Hash32 { return hashutil.New32() }
