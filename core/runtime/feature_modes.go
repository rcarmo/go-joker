package runtime

import "os"

func envMode(name, def string) string {
	mode := os.Getenv(name)
	if mode == "" {
		return def
	}
	return mode
}
func modeEnabled(mode string) bool  { return mode != "0" && mode != "off" && mode != "false" }
func modeForce(mode string) bool    { return mode == "1" || mode == "force" || mode == "all" }
func modeDisabled(mode string) bool { return !modeEnabled(mode) }

func IRStringBuilderMode() string   { return envMode("JOKER_IR_STRING_BUILDER", "auto") }
func IRStringBuilderForce() bool    { return modeForce(IRStringBuilderMode()) }
func IRStringBuilderDisabled() bool { return modeDisabled(IRStringBuilderMode()) }

func WasmMultiFnMode() string  { return envMode("JOKER_WASM_MULTIFN", "auto") }
func WasmMultiFnEnabled() bool { return modeEnabled(WasmMultiFnMode()) }
func WasmMultiFnForce() bool   { return modeForce(WasmMultiFnMode()) }
func WasmMemNthEnabled() bool  { return os.Getenv("JOKER_WASM_MEM_NTH") != "" }

func IRInlineMode() string   { return envMode("JOKER_IR_INLINE", "auto") }
func IRInlineForce() bool    { return modeForce(IRInlineMode()) }
func IRInlineDisabled() bool { return modeDisabled(IRInlineMode()) }

func IRTypedEnabled() bool    { return modeEnabled(os.Getenv("JOKER_IR_TYPED")) }
func IRTypedMapMode() string  { return envMode("JOKER_IR_TYPED_MAP", "auto") }
func IRTypedMapEnabled() bool { return modeEnabled(IRTypedMapMode()) }
func IRTypedVecEnabled() bool {
	mode := os.Getenv("JOKER_IR_TYPED_VEC")
	return mode == "1" || mode == "on" || mode == "true" || mode == "force"
}
func IRTypedMapForce() bool { return modeForce(IRTypedMapMode()) }
