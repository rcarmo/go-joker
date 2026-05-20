package filepath

import (
	"os"
	"path/filepath"

	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"
)

func fileSeq(root string) *corecollections.ArrayVector {
	res := corecollections.EmptyArrayVector()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		m := FileInfoMap(path, info)
		res.Append(m)
		return nil
	})
	if err != nil {
		panic(RT.NewError("filepath/file-seq: " + err.Error()))
	}
	return res
}
