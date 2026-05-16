package os

import (
	"io"
	"math/big"
	"os"
	"os/exec"
	"strings"
	"syscall"

	. "github.com/rcarmo/go-joker/core"
)

func nativeIntObject(n int64) Object {
	maxNativeInt := int64(int(^uint(0) >> 1))
	minNativeInt := -maxNativeInt - 1
	if n > maxNativeInt || n < minNativeInt {
		return MakeBigInt(big.NewInt(n))
	}
	return MakeInt(int(n))
}

func env() Object {
	res := EmptyArrayMap()
	for _, v := range os.Environ() {
		parts := strings.SplitN(v, "=", 2)
		res.Add(String{S: parts[0]}, String{S: parts[1]})
	}
	return res
}

func getEnv(key string) Object {
	if v, ok := os.LookupEnv(key); ok {
		return MakeString(v)
	}
	return NIL
}

func commandArgs() Object {
	res := EmptyVector()
	for _, arg := range os.Args {
		res = res.Conjoin(String{S: arg})
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

func startProcess(name string, opts Map) int {
	dir, args, stdin, stdout, stderr := parseExecOpts(opts)

	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin

	cmd.Stdout = processOutputOrDiscard(stdout)
	cmd.Stderr = processOutputOrDiscard(stderr)

	err := cmd.Start()
	PanicOnErr(err)

	return cmd.Process.Pid
}

func sendSignal(pid, signal int) Object {
	p, err := os.FindProcess(pid)
	PanicOnErr(err)
	err = p.Signal(syscall.Signal(signal))
	PanicOnErr(err)
	return NIL
}

func killProcess(pid int) Object {
	p, err := os.FindProcess(pid)
	PanicOnErr(err)
	err = p.Kill()
	PanicOnErr(err)
	// Wait to avoid zombie child processes.
	// Ignore result and error (which may occur if p is not a child process)
	p.Wait()
	return NIL
}

func parseExecOpts(opts Map) (dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) {
	if ok, dirObj := opts.Get(MakeKeyword("dir")); ok && !dirObj.Equals(NIL) {
		dir = EnsureObjectIsString(dirObj, "dir: %s").S
	}
	if ok, argsObj := opts.Get(MakeKeyword("args")); ok {
		s := EnsureObjectIsSeqable(argsObj, "args: %s").Seq()
		for !s.IsEmpty() {
			args = append(args, EnsureObjectIsString(s.First(), "args: %s").S)
			s = s.Rest()
		}
	}
	if ok, stdinObj := opts.Get(MakeKeyword("stdin")); ok {
		// Check if the intent was to pipe stdin into the program being called and
		// use Stdin directly rather than GLOBAL_ENV.stdin.Value, which is a buffered wrapper.
		// TODO: this won't work correctly if GLOBAL_ENV.stdin is bound to something other than Stdin
		if GLOBAL_ENV.IsStdIn(stdinObj) {
			stdin = Stdin
		} else {
			switch s := stdinObj.(type) {
			case Nil:
			case *IOReader:
				stdin = s.Reader
			case io.Reader:
				stdin = s
			case String:
				stdin = strings.NewReader(s.S)
			default:
				panic(RT.NewError("stdin option must be either an IOReader or a string, got " + stdinObj.GetType().ToString(false)))
			}
		}
	}
	if ok, stdoutObj := opts.Get(MakeKeyword("stdout")); ok {
		switch s := stdoutObj.(type) {
		case Nil:
		case *IOWriter:
			stdout = s.Writer
		case io.Writer:
			stdout = s
		default:
			panic(RT.NewError("stdout option must be an IOWriter, got " + stdoutObj.GetType().ToString(false)))
		}
	}
	if ok, stderrObj := opts.Get(MakeKeyword("stderr")); ok {
		switch s := stderrObj.(type) {
		case Nil:
		case *IOWriter:
			stderr = s.Writer
		case io.Writer:
			stderr = s
		default:
			panic(RT.NewError("stderr option must be an IOWriter, got " + stderrObj.GetType().ToString(false)))
		}
	}
	return
}

func execute(name string, opts Map) Object {
	dir, args, stdin, stdout, stderr := parseExecOpts(opts)
	return sh(dir, stdin, stdout, stderr, name, args)
}

func readDir(dirname string) Object {
	entries, err := os.ReadDir(dirname)
	PanicOnErr(err)
	res := EmptyVector()
	name := MakeKeyword("name")
	size := MakeKeyword("size")
	mode := MakeKeyword("mode")
	isDir := MakeKeyword("dir?")
	modTime := MakeKeyword("modtime")
	for _, e := range entries {
		info, err := e.Info()
		PanicOnErr(err)
		m := EmptyArrayMap()
		m.Add(name, MakeString(e.Name()))
		m.Add(size, nativeIntObject(info.Size()))
		m.Add(mode, MakeInt(int(info.Mode())))
		m.Add(isDir, MakeBoolean(e.IsDir()))
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
