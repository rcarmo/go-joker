package runtime

import (
	"strconv"
	"sync"
)

var wasmModSeq uint64
var wasmModMu sync.Mutex

func NextWasmModuleName() string {
	wasmModMu.Lock()
	wasmModSeq++
	n := wasmModSeq
	wasmModMu.Unlock()
	return "joker_wasm_" + strconv.FormatUint(n, 10)
}
