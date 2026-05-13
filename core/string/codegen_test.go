package string

import "testing"

func TestCodegenHelpers(t *testing.T) {
	if got := SymbolGoName("joker.core/map?"); got != "joker_DOT_core_FW_map_Q_" {
		t.Fatalf("SymbolGoName() = %q", got)
	}
	if got := KeywordGoName(":joker.core/foo"); got != "joker_DOT_core_FW_foo" {
		t.Fatalf("KeywordGoName() = %q", got)
	}
	if got := VarRefExprName("var_demo"); got != "varRefExpr_demo" {
		t.Fatalf("VarRefExprName() = %q", got)
	}
	if got := TypeNameInCore("core.Symbol"); got != "Symbol" {
		t.Fatalf("TypeNameInCore() = %q", got)
	}
	if got := TypeNameAsGo("*Symbol"); got != "symbol" {
		t.Fatalf("TypeNameAsGo() = %q", got)
	}
}
