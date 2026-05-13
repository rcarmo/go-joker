package stringpool

import "testing"

func TestInternReusesStringPointer(t *testing.T) {
	p := Pool{}
	a := p.Intern("hello")
	b := p.Intern("hello")
	if a != b {
		t.Fatal("Intern should reuse pointer for identical strings")
	}
}
