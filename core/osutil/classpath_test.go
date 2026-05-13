package osutil

import (
	"reflect"
	"testing"
)

func TestClassPathElementsEmptyDefaultsToEmptyEntry(t *testing.T) {
	got := ClassPathElements("")
	want := []string{""}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ClassPathElements(\"\") = %#v, want %#v", got, want)
	}
}
