package edn

import (
	"bufio"
	"errors"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"
	"strings"

	. "github.com/rcarmo/go-joker/core"
)

func readEDNString(s string) coretypes.Object {
	r := NewReader(bufio.NewReader(strings.NewReader(s)), "<edn-string>")
	obj, err := TryRead(r)
	if err != nil {
		panic(RT.NewError("EDN read error: " + err.Error()))
	}
	return obj
}

func ReadEDNString(s string) (coretypes.Object, error) {
	r := NewReader(bufio.NewReader(strings.NewReader(s)), "<edn-string>")
	obj, err := TryRead(r)
	if err != nil {
		return NIL, err
	}
	return obj, nil
}

func writeEDNString(obj coretypes.Object) coretypes.Object {
	return coretypes.MakeString(WriteEDNString(obj))
}

func WriteEDNString(obj coretypes.Object) string {
	if obj == nil {
		return "nil"
	}
	return obj.ToString(true)
}

func DecodeAllEDN(s string) ([]coretypes.Object, error) {
	r := NewReader(bufio.NewReader(strings.NewReader(s)), "<edn-string>")
	out := []coretypes.Object{}
	for {
		obj, err := TryRead(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return out, nil
			}
			return out, err
		}
		out = append(out, obj)
	}
}
