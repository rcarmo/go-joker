package system

import (
	corert "github.com/rcarmo/go-joker/core/runtime"
	"math/big"
	"os"
	"os/user"
	"runtime"
	"time"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	. "github.com/rcarmo/go-joker/core"
)

func lineSeparator() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}

func systemProperties() coretypes.Map {
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
	cwd := ""
	if dir, err := os.Getwd(); err == nil {
		cwd = dir
	}
	m := corecollections.EmptyArrayMap()
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
		{"joker.version", corert.VERSION},
		{"go-joker.version", corert.VERSION},
	} {
		m.Add(coretypes.MakeString(kv[0]), coretypes.MakeString(kv[1]))
	}
	return m
}

func getProperty(args []coretypes.Object) coretypes.Object {
	if len(args) < 1 || len(args) > 2 {
		panic(RT.NewError("System/getProperty expects 1 or 2 args"))
	}
	key := coretypes.EnsureArgIsString(args, 0)
	props := systemProperties()
	if ok, v := props.Get(key); ok {
		return v
	}
	if len(args) == 2 {
		return args[1]
	}
	return NIL
}

func systemGetenv(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	key := coretypes.EnsureArgIsString(args, 0).S
	if v, ok := os.LookupEnv(key); ok {
		return coretypes.MakeString(v)
	}
	return NIL
}

func systemExit(args []coretypes.Object) coretypes.Object {
	CheckArity(args, 1, 1)
	code := coretypes.EnsureArgIsInt(args, 0).I
	os.Exit(code)
	return NIL
}

func systemIntObject(n int64) coretypes.Object {
	maxNativeInt := int64(int(^uint(0) >> 1))
	minNativeInt := -maxNativeInt - 1
	if n > maxNativeInt || n < minNativeInt {
		return coretypes.MakeBigInt(big.NewInt(n))
	}
	return coretypes.MakeInt(int(n))
}

func currentTimeMillis() coretypes.Object { return systemIntObject(time.Now().UnixMilli()) }
func nanoTime() coretypes.Object          { return systemIntObject(time.Now().UnixNano()) }
