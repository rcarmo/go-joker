package os

import (
	"bytes"
	corert "github.com/rcarmo/go-joker/core/runtime"
	"io"
	"os/exec"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"
)

func sh(dir string, stdin io.Reader, stdout io.Writer, stderr io.Writer, name string, args []string) coretypes.Object {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin

	var stdoutBuffer, stderrBuffer bytes.Buffer
	if stdout != nil {
		cmd.Stdout = stdout
	} else {
		cmd.Stdout = &stdoutBuffer
	}
	if stderr != nil {
		cmd.Stderr = stderr
	} else {
		cmd.Stderr = &stderrBuffer
	}

	err := cmd.Start()
	corert.PanicOnErr(err)

	err = cmd.Wait()

	res := corecollections.EmptyArrayMap()
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "success"), coretypes.Boolean{B: err == nil})

	var exitCode int
	if err != nil {
		res.Add(coretypes.MakeKeyword(STRINGS.Intern, "err-msg"), coretypes.String{S: err.Error()})
		exitCode = defaultFailedCode
	} else {
		exitCode = 0
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "exit"), coretypes.Int{I: exitCode})
	if stdout == nil {
		res.Add(coretypes.MakeKeyword(STRINGS.Intern, "out"), coretypes.String{S: string(stdoutBuffer.Bytes())})
	}
	if stderr == nil {
		res.Add(coretypes.MakeKeyword(STRINGS.Intern, "err"), coretypes.String{S: string(stderrBuffer.Bytes())})
	}
	return res
}
