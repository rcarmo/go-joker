package os

import (
	corert "github.com/rcarmo/go-joker/core/runtime"
	"io"
	"math/big"
	"os"
	"os/exec"
	"strings"
	"syscall"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"
)

func nativeIntObject(n int64) coretypes.Object {
	maxNativeInt := int64(int(^uint(0) >> 1))
	minNativeInt := -maxNativeInt - 1
	if n > maxNativeInt || n < minNativeInt {
		return coretypes.MakeBigInt(big.NewInt(n))
	}
	return coretypes.MakeInt(int(n))
}

func env() coretypes.Object {
	res := corecollections.EmptyArrayMap()
	for _, v := range os.Environ() {
		parts := strings.SplitN(v, "=", 2)
		res.Add(coretypes.String{S: parts[0]}, coretypes.String{S: parts[1]})
	}
	return res
}

func getEnv(key string) coretypes.Object {
	if v, ok := os.LookupEnv(key); ok {
		return coretypes.MakeString(v)
	}
	return NIL
}

func commandArgs() coretypes.Object {
	res := corecollections.EmptyVector()
	for _, arg := range os.Args {
		res = res.Conjoin(coretypes.String{S: arg})
	}
	return res
}

const defaultFailedCode = 127 // seen from 'sh no-such-file' on OS X and Ubuntu

func processOutputOrDiscard(w io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return io.Discard
}

func startProcess(name string, opts coretypes.Map) int {
	dir, args, stdin, stdout, stderr := parseExecOpts(opts)

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin

	cmd.Stdout = processOutputOrDiscard(stdout)
	cmd.Stderr = processOutputOrDiscard(stderr)

	err := cmd.Start()
	corert.PanicOnErr(err)
	pid := cmd.Process.Pid
	corert.PanicOnErr(cmd.Process.Release())

	return pid
}

func sendSignal(pid, signal int) coretypes.Object {
	p, err := os.FindProcess(pid)
	corert.PanicOnErr(err)
	err = p.Signal(syscall.Signal(signal))
	corert.PanicOnErr(err)
	return NIL
}

func killProcess(pid int) coretypes.Object {
	p, err := os.FindProcess(pid)
	corert.PanicOnErr(err)
	err = p.Kill()
	corert.PanicOnErr(err)
	// Wait to avoid zombie child processes.
	// Ignore result and error (which may occur if p is not a child process)
	p.Wait()
	return NIL
}

func parseExecOpts(opts coretypes.Map) (dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) {
	if ok, dirObj := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "dir")); ok && !dirObj.Equals(NIL) {
		dir = coretypes.EnsureObjectIsString(dirObj, "dir: %s").S
	}
	if ok, argsObj := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "args")); ok {
		s := coretypes.EnsureObjectIsSeqable(argsObj, "args: %s").Seq()
		for !s.IsEmpty() {
			args = append(args, coretypes.EnsureObjectIsString(s.First(), "args: %s").S)
			s = s.Rest()
		}
	}
	if ok, stdinObj := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "stdin")); ok {
		// Check if the intent was to pipe stdin into the program being called and
		// use Stdin directly rather than GLOBAL_ENV.stdin.Value, which is a buffered wrapper.
		// TODO: this won't work correctly if GLOBAL_ENV.stdin is bound to something other than Stdin
		if GLOBAL_ENV.IsStdIn(stdinObj) {
			stdin = Stdin
		} else {
			switch s := stdinObj.(type) {
			case Nil:
			case *corert.IOReader:
				stdin = s.Reader
			case io.Reader:
				stdin = s
			case coretypes.String:
				stdin = strings.NewReader(s.S)
			default:
				panic(RT.NewError("stdin option must be either an IOReader or a string, got " + stdinObj.GetType().ToString(false)))
			}
		}
	}
	if ok, stdoutObj := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "stdout")); ok {
		switch s := stdoutObj.(type) {
		case Nil:
		case *corert.IOWriter:
			stdout = s.Writer
		case io.Writer:
			stdout = s
		default:
			panic(RT.NewError("stdout option must be an IOWriter, got " + stdoutObj.GetType().ToString(false)))
		}
	}
	if ok, stderrObj := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "stderr")); ok {
		switch s := stderrObj.(type) {
		case Nil:
		case *corert.IOWriter:
			stderr = s.Writer
		case io.Writer:
			stderr = s
		default:
			panic(RT.NewError("stderr option must be an IOWriter, got " + stderrObj.GetType().ToString(false)))
		}
	}
	return
}

func execute(name string, opts coretypes.Map) coretypes.Object {
	dir, args, stdin, stdout, stderr := parseExecOpts(opts)
	return sh(dir, stdin, stdout, stderr, name, args)
}

func readDir(dirname string) coretypes.Object {
	entries, err := os.ReadDir(dirname)
	corert.PanicOnErr(err)
	res := corecollections.EmptyVector()
	name := coretypes.MakeKeyword(STRINGS.Intern, "name")
	size := coretypes.MakeKeyword(STRINGS.Intern, "size")
	mode := coretypes.MakeKeyword(STRINGS.Intern, "mode")
	isDir := coretypes.MakeKeyword(STRINGS.Intern, "dir?")
	modTime := coretypes.MakeKeyword(STRINGS.Intern, "modtime")
	for _, e := range entries {
		info, err := e.Info()
		corert.PanicOnErr(err)
		m := corecollections.EmptyArrayMap()
		m.Add(name, coretypes.MakeString(e.Name()))
		m.Add(size, nativeIntObject(info.Size()))
		m.Add(mode, coretypes.MakeInt(int(info.Mode())))
		m.Add(isDir, coretypes.MakeBoolean(e.IsDir()))
		m.Add(modTime, nativeIntObject(info.ModTime().Unix()))
		res = res.Conjoin(m)
	}
	return res
}

func exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	panic(RT.NewError(err.Error()))
}
