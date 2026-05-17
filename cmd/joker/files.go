package main

import (
	"bufio"
	"bytes"
	"fmt"
	corereader "github.com/rcarmo/go-joker/core/reader"
	"io"
	"os"
	"path/filepath"

	. "github.com/rcarmo/go-joker/core"
)

func processFile(filename string, phase corereader.Phase) error {
	var reader *Reader
	var input *os.File
	var formatBuf *bytes.Buffer
	var oldStdout io.Writer
	if filename == "-" {
		reader = NewReader(bufio.NewReader(Stdin), "<stdin>")
		filename = ""
	} else {
		var err error
		input, err = os.Open(filename)
		if err != nil {
			fmt.Fprintln(Stderr, "Error: ", err)
			return err
		}
		reader = NewReader(bufio.NewReader(input), filename)
		if phase == corereader.FormatPhase && writeFlag {
			formatBuf = &bytes.Buffer{}
			oldStdout = Stdout
			Stdout = formatBuf
			defer func() { Stdout = oldStdout }()
		} else {
			defer func() {
				if err := input.Close(); err != nil {
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
	processErr := ProcessReader(reader, filename, phase)
	if formatBuf == nil {
		return processErr
	}
	if err := input.Close(); err != nil {
		fmt.Fprintln(Stderr, "Error: ", err)
		if processErr == nil {
			processErr = err
		}
	}
	if processErr != nil {
		return processErr
	}
	out, err := os.Create(filename)
	if err != nil {
		fmt.Fprintln(Stderr, "Error: ", err)
		return err
	}
	if _, err := out.WriteString(formatBuf.String()); err != nil {
		fmt.Fprintln(Stderr, "Error: ", err)
		if closeErr := out.Close(); closeErr != nil {
			fmt.Fprintln(Stderr, "Error: ", closeErr)
		}
		return err
	}
	if err := out.Close(); err != nil {
		fmt.Fprintln(Stderr, "Error: ", err)
		return err
	}
	return nil
}
