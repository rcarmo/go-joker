//go:build go_spew
// +build go_spew

package core

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"

	"github.com/jcburley/go-spew/spew"
)

var procGoSpew = func(args []coretypes.Object) (res coretypes.Object) {
	res = coretypes.MakeBoolean(false)
	CheckArity(args, 1, 2)
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(Stderr, "coretypes.Error: %v\n", r)
		}
	}()
	scs := spew.NewDefaultConfig()
	if len(args) > 1 {
		m := ExtractMap(args, 1)
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "Indent")); yes {
			scs.Indent = k.(coretypes.Native).Native().(string)
		}
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "MaxDepth")); yes {
			scs.MaxDepth = k.(coretypes.Native).Native().(int)
		}
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "DisableMethods")); yes {
			scs.DisableMethods = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "DisablePointerMethods")); yes {
			scs.DisablePointerMethods = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "DisablePointerAddresses")); yes {
			scs.DisablePointerAddresses = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "DisableCapacities")); yes {
			scs.DisableCapacities = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "ContinueOnMethod")); yes {
			scs.ContinueOnMethod = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "SortKeys")); yes {
			scs.SortKeys = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "SpewKeys")); yes {
			scs.SpewKeys = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "NoDuplicates")); yes {
			scs.NoDuplicates = k.(coretypes.Native).Native().(bool)
		}
		if yes, k := m.Get(coretypes.MakeKeyword(STRINGS.Intern, "UseOrdinals")); yes {
			scs.UseOrdinals = k.(coretypes.Native).Native().(bool)
		}
	}
	scs.Fdump(Stderr, args[0])
	return coretypes.MakeBoolean(true)
}
