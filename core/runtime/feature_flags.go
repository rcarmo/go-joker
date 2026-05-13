package runtime

import "os"

func IRStringBuilderMode() string {
	mode := os.Getenv("JOKER_IR_STRING_BUILDER")
	if mode == "" {
		return "auto"
	}
	return mode
}

func IRStringBuilderForce() bool {
	mode := IRStringBuilderMode()
	return mode == "1" || mode == "force" || mode == "all"
}

func IRStringBuilderDisabled() bool {
	mode := IRStringBuilderMode()
	return mode == "0" || mode == "off" || mode == "false"
}

func WasmMultiFnMode() string {
	mode := os.Getenv("JOKER_WASM_MULTIFN")
	if mode == "" {
		return "auto"
	}
	return mode
}

func WasmMultiFnEnabled() bool {
	mode := WasmMultiFnMode()
	return mode != "0" && mode != "off" && mode != "false"
}

func WasmMultiFnForce() bool {
	mode := WasmMultiFnMode()
	return mode == "1" || mode == "force" || mode == "all"
}

func WasmMemNthEnabled() bool {
	return os.Getenv("JOKER_WASM_MEM_NTH") != ""
}
