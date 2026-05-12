package core

import "testing"

func TestGeneratedCoreNamespacesDriveCoreNamespaceVar(t *testing.T) {
	ProcessCoreData()
	vr := GLOBAL_ENV.CoreNamespace.Resolve("*core-namespaces*")
	if vr == nil {
		t.Fatal("*core-namespaces* var not found")
	}
	set, ok := vr.Value.(*MapSet)
	if !ok {
		t.Fatalf("*core-namespaces* = %T, want *MapSet", vr.Value)
	}
	for _, ns := range generatedCoreNamespaces() {
		if found, _ := set.Get(MakeSymbol(ns)); !found {
			t.Fatalf("*core-namespaces* missing generated namespace %s", ns)
		}
	}
	if found, _ := set.Get(MakeSymbol("user")); !found {
		t.Fatal("*core-namespaces* missing user namespace")
	}
}
