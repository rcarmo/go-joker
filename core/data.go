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
	processGeneratedLinterPayload("linter_all.joke")
	if dialect == JOKER {
		markJokerNamespacesAsUsed()
		processGeneratedLinterPayload("linter_joker.joke")
		return
	}
	processGeneratedLinterPayload("linter_cljx.joke")
	switch dialect {
	case CLJ:
		processGeneratedLinterPayload("linter_clj.joke")
	case CLJS:
		processGeneratedLinterPayload("linter_cljs.joke")
	}
}

func processGeneratedLinterPayload(path string) {
	data, ok := coregenerated.LinterDataByPath(path)
	if !ok {
		panic(RT.NewError("missing generated linter payload: " + path))
	}
	processData(data)
}
