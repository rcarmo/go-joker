package core

// core_api_gaps.go — Fills remaining core API gaps from divergence matrix.

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"os"
	"path/filepath"
	"regexp"
)

func init() {
	registerCoreAPIGaps()
}

func registerCoreAPIGaps() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// alter-var-root — (alter-var-root var fn & args)
	avrVr := ns.Intern(MakeSymbol("alter-var-root"))
	avrVr.Value = Proc{Name: "procAlterVarRoot", Fn: func(args []Object) Object {
		if len(args) < 2 {
			PanicArityMinMax(len(args), 2, 999)
		}
		vr := EnsureObjectIsVar(args[0], "alter-var-root requires a var, got %s")
		fn := EnsureObjectIsCallable(args[1], "alter-var-root requires a function, got %s")
		fnArgs := make([]Object, 1+len(args)-2)
		fnArgs[0] = vr.Value
		for i := 2; i < len(args); i++ {
			fnArgs[i-1] = args[i]
		}
		vr.Value = fn.Call(fnArgs)
		return vr.Value
	}}
	referToUser(MakeSymbol("alter-var-root"), avrVr)

	// re-groups — (re-groups matcher) — returns groups from last regex match
	// In Joker, re-find already returns groups. Provide re-groups for compat.
	rgVr := ns.Intern(MakeSymbol("re-groups"))
	rgVr.Value = Proc{Name: "procReGroups", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		// re-groups expects a Matcher, but Joker doesn't have Matcher objects.
		// Instead, accept [pattern string] and return groups.
		switch v := args[0].(type) {
		case *ArrayVector:
			if v.Count() >= 2 {
				re := EnsureObjectIsRegex(v.At(0), "re-groups requires [regex string]")
				s := EnsureObjectIsString(v.At(1), "re-groups requires [regex string]")
				matches := regexp.MustCompile(re.R.String()).FindStringSubmatch(s.S)
				if matches == nil {
					return NIL
				}
				if len(matches) == 1 {
					return String{S: matches[0]}
				}
				result := collectionConstruction.NewEmptyArrayVector()
				for _, m := range matches {
					result = result.Conj(String{S: m}).(*ArrayVector)
				}
				return result
			}
		}
		return NIL
	}}
	referToUser(MakeSymbol("re-groups"), rgVr)

	// file-seq — (file-seq dir) — returns a lazy seq of files
	fsVr := ns.Intern(MakeSymbol("file-seq"))
	fsVr.Value = Proc{Name: "procFileSeq", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		dir := EnsureObjectIsString(args[0], "file-seq requires a string path, got %s")
		var files []Object
		filepath.Walk(dir.S, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			files = append(files, String{S: path})
			return nil
		})
		if len(files) == 0 {
			return NIL
		}
		return &ArraySeq{arr: files, index: 0}
	}}
	referToUser(MakeSymbol("file-seq"), fsVr)

	// var-get — (var-get var)
	vgVr := ns.Intern(MakeSymbol("var-get"))
	vgVr.Value = Proc{Name: "procVarGet", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		vr := EnsureObjectIsVar(args[0], "var-get requires a var, got %s")
		if vr.Value == nil {
			return NIL
		}
		return vr.Value
	}}
	referToUser(MakeSymbol("var-get"), vgVr)

	// var-set — (var-set var val)
	vsVr := ns.Intern(MakeSymbol("var-set"))
	vsVr.Value = Proc{Name: "procVarSet", Fn: func(args []Object) Object {
		CheckArity(args, 2, 2)
		vr := EnsureObjectIsVar(args[0], "var-set requires a var, got %s")
		vr.Value = args[1]
		return args[1]
	}}
	referToUser(MakeSymbol("var-set"), vsVr)

	// var? — (var? x)
	vqVr := ns.Intern(MakeSymbol("var?"))
	vqVr.Value = Proc{Name: "procVarQ", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		_, ok := args[0].(*Var)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(MakeSymbol("var?"), vqVr)
}
