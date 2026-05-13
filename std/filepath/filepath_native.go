package filepath

import (
	"os"
	"path/filepath"

	. "github.com/rcarmo/go-joker/core"
)

func fileSeq(root string) *ArrayVector {
	res := EmptyArrayVector()
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
