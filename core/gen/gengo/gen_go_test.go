package gengo

import (
	"reflect"
	"testing"
)

func TestInvalidReflectionValueEmitsNil(t *testing.T) {
	generator := &GenGo{}
	if got := generator.ValueExpr("metadata", reflect.TypeOf((*interface{})(nil)), reflect.Value{}); got != "nil" {
		t.Fatalf("got %q want nil", got)
	}
}
