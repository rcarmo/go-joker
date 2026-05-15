package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	. "github.com/rcarmo/go-joker/core"
)

func processFile(filename string, phase Phase) error {
	var reader *Reader
	if filename == "-" {
		reader = NewReader(bufio.NewReader(Stdin), "<stdin>")
		filename = ""
	} else {
		var err error
		f, err := os.Open(filename)
		if err != nil {
			fmt.Fprintln(Stderr, "Error: ", err)
			return err
		}
		reader = NewReader(bufio.NewReader(f), filename)
		if phase == FORMAT && writeFlag {
			var b bytes.Buffer
			oldStdout := Stdout
			Stdout = &b
			defer func() {
				Stdout = oldStdout
				if err := f.Close(); err != nil {
					fmt.Fprintln(Stderr, "Error: ", err)
				}
				out, err := os.Create(filename)
				if err != nil {
					fmt.Fprintln(Stderr, "Error: ", err)
					return
				}
				if _, err := out.WriteString(b.String()); err != nil {
					fmt.Fprintln(Stderr, "Error: ", err)
				}
				if err := out.Close(); err != nil {
					fmt.Fprintln(Stderr, "Error: ", err)
				}
			}()
		} else {
			defer func() {
				if err := f.Close(); err != nil {
					fmt.Fprintln(Stderr, "Error: ", err)
				}
			}()
		}
	}
	if filename != "" {
		f, err := filepath.Abs(filename)
		PanicOnErr(err)
		GLOBAL_ENV.SetMainFilename(f)
	}
	if saveForRepl {
		reader = NewReader(&replayable{reader}, "<replay>")
	}
	return ProcessReader(reader, filename, phase)
}
