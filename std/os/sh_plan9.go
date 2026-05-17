package os

import (
	"bytes"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"
	"os/exec"

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
	PanicOnErr(err)

	err = cmd.Wait()

	res := EmptyArrayMap()
	res.Add(MakeKeyword("success"), coretypes.Boolean{B: err == nil})

	var exitCode int
	if err != nil {
		res.Add(MakeKeyword("err-msg"), coretypes.String{S: err.Error()})
		exitCode = defaultFailedCode
	} else {
		exitCode = 0
	}
	res.Add(MakeKeyword("exit"), coretypes.Int{I: exitCode})
	if stdout == nil {
		res.Add(MakeKeyword("out"), coretypes.String{S: string(stdoutBuffer.Bytes())})
	}
	if stderr == nil {
		res.Add(MakeKeyword("err"), coretypes.String{S: string(stderrBuffer.Bytes())})
	}
	return res
}
