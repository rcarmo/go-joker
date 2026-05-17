package core

// tagged_literals.go — Built-in tagged literal readers (#inst, #uuid).

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"time"
)

func init() {
	registerTaggedLiterals()
}

func registerTaggedLiterals() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// Register #inst reader — parses ISO 8601 date strings to Time
	instReaderVr := ns.Intern(MakeSymbol("__read-inst"))
	instReaderVr.Value = Proc{Name: "procReadInst", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		s := EnsureObjectIsString(args[0], "#inst argument must be a string, got %s")
		// Try RFC3339 first, then other common formats
		formats := []string{
			time.RFC3339Nano,
			time.RFC3339,
			"2006-01-02T15:04:05.000-07:00",
			"2006-01-02T15:04:05",
			"2006-01-02",
		}
		for _, f := range formats {
			if t, err := time.Parse(f, s.S); err == nil {
				return coretypes.Time{T: t}
			}
		}
		panic(RT.NewError(fmt.Sprintf("Cannot parse #inst \"%s\"", s.S)))
	}}
	instReaderVr.isPrivate = true

	// Register #uuid reader — stores as string (no java.util.UUID equivalent)
	uuidReaderVr := ns.Intern(MakeSymbol("__read-uuid"))
	uuidReaderVr.Value = Proc{Name: "procReadUuid", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		s := EnsureObjectIsString(args[0], "#uuid argument must be a string, got %s")
		// Basic UUID format validation
		if len(s.S) != 36 {
			panic(RT.NewError(fmt.Sprintf("Invalid UUID format: \"%s\"", s.S)))
		}
		return s
	}}
	uuidReaderVr.isPrivate = true

	// Install into default-data-readers
	readersVr := ns.Resolve("default-data-readers")
	if readersVr == nil {
		readersVr = ns.Intern(MakeSymbol("default-data-readers"))
	}

	m := collectionConstruction.NewEmptyArrayMap()
	m.Add(MakeSymbol("inst"), instReaderVr)
	m.Add(MakeSymbol("uuid"), uuidReaderVr)
	readersVr.Value = m

	// Also install *data-readers* dynamic var
	dataReadersVr := ns.Resolve("*data-readers*")
	if dataReadersVr == nil {
		dataReadersVr = ns.Intern(MakeSymbol("*data-readers*"))
	}
	dataReadersVr.Value = m
	referToUser(MakeSymbol("*data-readers*"), dataReadersVr)

	// Clojure-compatible fallback hook. If non-nil, called as (f tag value)
	// when a reader tag is not present in *data-readers* or default-data-readers.
	fallbackVr := ns.Resolve("*default-data-reader-fn*")
	if fallbackVr == nil {
		fallbackVr = ns.Intern(MakeSymbol("*default-data-reader-fn*"))
	}
	fallbackVr.Value = NIL
	referToUser(MakeSymbol("*default-data-reader-fn*"), fallbackVr)

	// Convenience alias used by some lightweight compatibility tests/docs.
	fallbackAliasVr := ns.Resolve("default-data-reader-fn")
	if fallbackAliasVr == nil {
		fallbackAliasVr = ns.Intern(MakeSymbol("default-data-reader-fn"))
	}
	fallbackAliasVr.Value = fallbackVr
	referToUser(MakeSymbol("default-data-reader-fn"), fallbackAliasVr)
}
