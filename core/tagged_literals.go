package core

// tagged_literals.go — Built-in tagged literal readers (#inst, #uuid).

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
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
	instReaderVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "__read-inst"))
	instReaderVr.Value = Proc{Name: "procReadInst", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		s := coretypes.EnsureObjectIsString(args[0], "#inst argument must be a string, got %s")
		t, err := coretypes.ParseInstString(s.S)
		if err != nil {
			panic(RT.NewError(err.Error()))
		}
		return t
	}}
	instReaderVr.isPrivate = true

	// Register #uuid reader — stores as string (no java.util.UUID equivalent)
	uuidReaderVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "__read-uuid"))
	uuidReaderVr.Value = Proc{Name: "procReadUuid", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		s := coretypes.EnsureObjectIsString(args[0], "#uuid argument must be a string, got %s")
		if err := coretypes.ValidateUUIDString(s.S); err != nil {
			panic(RT.NewError(err.Error()))
		}
		return s
	}}
	uuidReaderVr.isPrivate = true

	// Install into default-data-readers
	readersVr := ns.Resolve("default-data-readers")
	if readersVr == nil {
		readersVr = ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "default-data-readers"))
	}

	m := collectionConstruction.NewEmptyArrayMap()
	m.Add(coretypes.MakeSymbol(STRINGS.Intern, "inst"), instReaderVr)
	m.Add(coretypes.MakeSymbol(STRINGS.Intern, "uuid"), uuidReaderVr)
	readersVr.Value = m

	// Also install *data-readers* dynamic var
	dataReadersVr := ns.Resolve("*data-readers*")
	if dataReadersVr == nil {
		dataReadersVr = ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*data-readers*"))
	}
	dataReadersVr.Value = m
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "*data-readers*"), dataReadersVr)

	// Clojure-compatible fallback hook. If non-nil, called as (f tag value)
	// when a reader tag is not present in *data-readers* or default-data-readers.
	fallbackVr := ns.Resolve("*default-data-reader-fn*")
	if fallbackVr == nil {
		fallbackVr = ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "*default-data-reader-fn*"))
	}
	fallbackVr.Value = NIL
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "*default-data-reader-fn*"), fallbackVr)

	// Convenience alias used by some lightweight compatibility tests/docs.
	fallbackAliasVr := ns.Resolve("default-data-reader-fn")
	if fallbackAliasVr == nil {
		fallbackAliasVr = ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "default-data-reader-fn"))
	}
	fallbackAliasVr.Value = fallbackVr
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "default-data-reader-fn"), fallbackAliasVr)
}
