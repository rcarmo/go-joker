package core

import "testing"

func TestExtendTypeInternalRejectsOddMethodPairs(t *testing.T) {
	proto := &Protocol{name: MakeSymbol("AuditProto")}
	extendType := GLOBAL_ENV.CoreNamespace.Resolve("__extend-type")
	if extendType == nil {
		t.Fatal("missing __extend-type var")
	}
	proc, ok := extendType.Value.(Proc)
	if !ok {
		t.Fatalf("__extend-type value = %T, want Proc", extendType.Value)
	}
	assertPanics(t, "odd __extend-type method pairs", func() {
		proc.Call([]Object{proto, MakeString("AuditType"), MakeString("method")})
	})
}
