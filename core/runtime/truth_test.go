package runtime

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

func TestToBool(t *testing.T) {
	if !IsNil(Nil{}) || !IsNil(nil) {
		t.Fatal("IsNil should detect nil values")
	}
	if ToBool(Nil{}) {
		t.Fatal("Nil should be false")
	}
	if ToBool(coretypes.Boolean{B: false}) {
		t.Fatal("false Boolean should be false")
	}
	if !ToBool(coretypes.Boolean{B: true}) || !ToBool(coretypes.MakeString("")) {
		t.Fatal("truthy values should be true")
	}
}
