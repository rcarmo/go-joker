package core

import coretypes "github.com/rcarmo/go-joker/core/types"

// record_init.go — Register __defrecord and record constructors.

func init() {
	registerRecordProcs()
}

func registerRecordProcs() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// record? — always available
	recordQVr := ns.Intern(MakeSymbol("record?"))
	recordQVr.Value = Proc{Name: "procRecordQ", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		_, ok := args[0].(*Record)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(MakeSymbol("record?"), recordQVr)

	// __defrecord — internal helper
	// Args: [record-name-symbol field1-string field2-string ...]
	// Returns: the RecordType, and installs:
	//   - ->RecordName constructor fn
	//   - map->RecordName factory fn
	defRecordVr := ns.Intern(MakeSymbol("__defrecord"))
	defRecordVr.Value = Proc{Name: "procDefRecordInternal", Fn: func(args []Object) Object {
		if len(args) < 1 {
			panic(RT.NewError("__defrecord requires at least a name"))
		}
		name := EnsureObjectIsSymbol(args[0], "defrecord name must be a symbol")
		nameStr := name.ToString(false)

		fields := make([]string, len(args)-1)
		for i := 1; i < len(args); i++ {
			fields[i-1] = EnsureObjectIsString(args[i], "field name must be a string").S
		}

		rtype := MakeRecordType(nameStr, fields)

		currentNs := GLOBAL_ENV.CurrentNamespace()

		// Install positional constructor: (->RecordName field1 field2 ...)
		ctorName := "->" + nameStr
		ctorVr := currentNs.Intern(MakeSymbol(ctorName))
		ctorVr.Value = Proc{Name: "proc" + ctorName, Fn: func(ctorArgs []Object) Object {
			return NewRecord(rtype, ctorArgs)
		}}

		// Install map factory: (map->RecordName {:field1 v1 :field2 v2})
		mapCtorName := "map->" + nameStr
		mapCtorVr := currentNs.Intern(MakeSymbol(mapCtorName))
		mapCtorVr.Value = Proc{Name: "proc" + mapCtorName, Fn: func(ctorArgs []Object) Object {
			CheckArity(ctorArgs, 1, 1)
			m := EnsureObjectIsMap(ctorArgs[0], "map->"+nameStr+" requires a map argument")
			vals := make([]Object, len(fields))
			for i, fname := range fields {
				kw := MakeKeyword(fname)
				if ok, v := m.Get(kw); ok {
					vals[i] = v
				} else {
					vals[i] = NIL
				}
			}
			rec := NewRecord(rtype, vals)
			// Add any extra keys as extension fields
			for iter := m.Iter(); iter.HasNext(); {
				p := iter.Next()
				if kw, ok := p.Key.(Keyword); ok {
					kwName := kw.ToString(false)[1:]
					if _, isBase := rtype.fieldIdx[kwName]; isBase {
						continue
					}
				}
				rec = rec.Assoc(p.Key, p.Value).(*Record)
			}
			return rec
		}}

		return NIL
	}}
	defRecordVr.isPrivate = true
	referToUser(MakeSymbol("__defrecord"), defRecordVr)
}
