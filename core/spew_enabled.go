//go:build go_spew
// +build go_spew

package core

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"

	"github.com/jcburley/go-spew/spew"
)

var procGoSpew = func(args []Object) (res Object) {
	res = MakeBoolean(false)
	CheckArity(args, 1, 2)
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(Stderr, "Error: %v\n", r)
		}
	}()
	scs := spew.NewDefaultConfig()
	if len(args) > 1 {
		m := ExtractMap(args, 1)
		if yes, k := m.Get(MakeKeyword("Indent")); yes {
			scs.Indent = k.(coretypes.Native).Native().(string)
		}
		if yes, k := m.Get(MakeKeyword("MaxDepth")); yes {
			scs.MaxDepth = k.(coretypes.Native).Native().(int)
		}
		if yes, k := m.Get(MakeKeyword("DisableMethods")); yes {
			scs.DisableMethods = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(MakeKeyword("DisablePointerMethods")); yes {
			scs.DisablePointerMethods = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(MakeKeyword("DisablePointerAddresses")); yes {
			scs.DisablePointerAddresses = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(MakeKeyword("DisableCapacities")); yes {
			scs.DisableCapacities = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(MakeKeyword("ContinueOnMethod")); yes {
			scs.ContinueOnMethod = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(MakeKeyword("SortKeys")); yes {
			scs.SortKeys = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(MakeKeyword("SpewKeys")); yes {
			scs.SpewKeys = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(MakeKeyword("NoDuplicates")); yes {
			scs.NoDuplicates = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(MakeKeyword("UseOrdinals")); yes {
			scs.UseOrdinals = k.(coretypes.Native).Native().(bool)
		}
	}
	scs.Fdump(Stderr, args[0])
	return MakeBoolean(true)
}
