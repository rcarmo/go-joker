package system

import (
	"os"
	"os/user"
	"runtime"
	"time"

	. "github.com/candid82/joker/core"
)

func lineSeparator() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

func systemProperties() Map {
	home := os.Getenv("HOME")
	uname := os.Getenv("USER")
	if u, err := user.Current(); err == nil {
		if home == "" {
			home = u.HomeDir
		}
		if uname == "" {
			uname = u.Username
		}
	}
	cwd, _ := os.Getwd()
	m := EmptyArrayMap()
	for _, kv := range [][2]string{
		{"user.home", home},
		{"user.name", uname},
		{"user.dir", cwd},
		{"java.io.tmpdir", os.TempDir()},
		{"os.name", runtime.GOOS},
		{"os.arch", runtime.GOARCH},
		{"os.version", ""},
		{"file.separator", string(os.PathSeparator)},
		{"path.separator", string(os.PathListSeparator)},
		{"line.separator", lineSeparator()},
		{"file.encoding", "UTF-8"},
		{"joker.version", VERSION},
		{"go-joker.version", VERSION},
	} {
		m.Add(MakeString(kv[0]), MakeString(kv[1]))
	}
	return m
}

func getProperty(args []Object) Object {
	if len(args) < 1 || len(args) > 2 {
		panic(RT.NewError("System/getProperty expects 1 or 2 args"))
	}
	key := EnsureArgIsString(args, 0)
	props := systemProperties()
	if ok, v := props.Get(key); ok {
		return v
	}
	if len(args) == 2 {
		return args[1]
	}
	return NIL
}

func systemGetenv(args []Object) Object {
	CheckArity(args, 1, 1)
	key := EnsureArgIsString(args, 0).S
	if v, ok := os.LookupEnv(key); ok {
		return MakeString(v)
	}
	return NIL
}

func systemExit(args []Object) Object {
	CheckArity(args, 1, 1)
	code := EnsureArgIsInt(args, 0).I
	os.Exit(code)
	return NIL
}

func currentTimeMillis() Object { return MakeInt(int(time.Now().UnixMilli())) }
func nanoTime() Object          { return MakeInt(int(time.Now().UnixNano())) }
