package trace

import "os"

func NewFunctionTracerFromEnv() *FunctionTracer {
	enabled := os.Getenv("JOKER_FUNCTION_TRACE") != "" || os.Getenv("JOKER_FUNCTION_TRACE_OUT") != ""
	return NewFunctionTracer(enabled, os.Getenv("JOKER_FUNCTION_TRACE_OUT"))
}

func NewSymbolTracerFromEnv() *SymbolTracer {
	enabled := os.Getenv("JOKER_SYMBOL_TRACE") != "" || os.Getenv("JOKER_SYMBOL_TRACE_OUT") != ""
	return NewSymbolTracer(enabled, os.Getenv("JOKER_SYMBOL_TRACE_OUT"))
}

func NewIRProfileFromEnv() *IRProfile {
	enabled := os.Getenv("JOKER_IR_PROFILE") != "" || os.Getenv("JOKER_IR_PROFILE_OUT") != ""
	return NewIRProfile(enabled, os.Getenv("JOKER_IR_PROFILE_OUT"))
}
