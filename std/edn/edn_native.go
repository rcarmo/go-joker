package edn

import (
	"bufio"
	"errors"
	"io"
	"strings"

	. "github.com/candid82/joker/core"
)

func readEDNString(s string) Object {
	r := NewReader(bufio.NewReader(strings.NewReader(s)), "<edn-string>")
	obj, err := TryRead(r)
	if err != nil {
		RT.NewError("EDN read error: " + err.Error())
	}
	return obj
}

func ReadEDNString(s string) (Object, error) {
	r := NewReader(bufio.NewReader(strings.NewReader(s)), "<edn-string>")
	obj, err := TryRead(r)
	if err != nil {
		return NIL, err
	}
	return obj, nil
}

func writeEDNString(obj Object) Object {
	return MakeString(WriteEDNString(obj))
}

func WriteEDNString(obj Object) string {
	if obj == nil {
		return "nil"
	}
	return obj.ToString(true)
}

func DecodeAllEDN(s string) ([]Object, error) {
	r := NewReader(bufio.NewReader(strings.NewReader(s)), "<edn-string>")
	out := []Object{}
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
