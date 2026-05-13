//go:build !gen_code
// +build !gen_code

package core

import coregenerated "github.com/rcarmo/go-joker/core/generated"

var haveSetCoreNamespaces bool

func ProcessCoreData() {
	// Let MaybeLazy() handle initialization.
	if !haveSetCoreNamespaces {
		setCoreNamespaces()
		haveSetCoreNamespaces = true
	}
}

func ProcessReplData() {
	// Let MaybeLazy() handle initialization.
}

func ProcessLinterData(dialect Dialect) {
	if dialect == EDN {
		markJokerNamespacesAsUsed()
		return
	}
	processData(coregenerated.LinterAllData)
	if dialect == JOKER {
		markJokerNamespacesAsUsed()
		processData(coregenerated.LinterJokerData)
		return
	}
	processData(coregenerated.LinterCljxData)
	switch dialect {
	case CLJ:
		processData(coregenerated.LinterCljData)
	case CLJS:
		processData(coregenerated.LinterCljsData)
	}
}
