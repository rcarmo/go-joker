package string

import (
	"reflect"
	"testing"
)

func TestSplitWhitespace(t *testing.T) {
	got := SplitWhitespace("a b\n\tc\r\nd")
	want := []string{"a", "b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitWhitespace() = %#v, want %#v", got, want)
	}
}
