package reader

import "testing"

func TestValidateIdentToken(t *testing.T) {
	valid := []struct {
		first rune
		token string
		last  rune
	}{
		{first: 'a', token: "abc", last: 'c'},
		{first: ':', token: "kw", last: 'w'},
		{first: ':', token: "/", last: '/'},
	}
	for _, tt := range valid {
		if err := ValidateIdentToken(tt.first, tt.token, tt.last); err != nil {
			t.Fatalf("ValidateIdentToken(%q, %q, %q) error: %v", tt.first, tt.token, tt.last, err)
		}
	}

	invalid := []struct {
		first rune
		token string
		last  rune
		msg   string
	}{
		{first: ':', token: "", last: 0, msg: "Invalid keyword: :"},
		{first: 'a', token: "abc:", last: ':', msg: "Invalid use of : in symbol name"},
		{first: 'a', token: "abc/", last: '/', msg: "Invalid use of / in symbol name"},
		{first: ':', token: "/abc", last: 'c', msg: "Blank namespaces are not allowed"},
	}
	for _, tt := range invalid {
		err := ValidateIdentToken(tt.first, tt.token, tt.last)
		if err == nil || err.Error() != tt.msg {
			t.Fatalf("ValidateIdentToken(%q, %q, %q) = %v, want %q", tt.first, tt.token, tt.last, err, tt.msg)
		}
	}
}
