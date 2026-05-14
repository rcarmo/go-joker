package main

import (
	"testing"

	. "github.com/rcarmo/go-joker/core"
)

func TestDetectDialectFromFilename(t *testing.T) {
	tests := []struct {
		name string
		file string
		want Dialect
	}{
		{name: "edn", file: "deps.edn", want: EDN},
		{name: "cljs", file: "src/app.cljs", want: CLJS},
		{name: "joker", file: "script.joke", want: JOKER},
		{name: "default clj", file: "src/app.cljc", want: CLJ},
		{name: "extensionless default clj", file: "Makefile", want: CLJ},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectDialect(tt.file); got != tt.want {
				t.Fatalf("detectDialect(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestDialectFromArg(t *testing.T) {
	tests := []struct {
		arg  string
		want Dialect
	}{
		{arg: "clj", want: CLJ},
		{arg: "CLJS", want: CLJS},
		{arg: "joker", want: JOKER},
		{arg: "EdN", want: EDN},
		{arg: "unknown", want: UNKNOWN},
	}

	for _, tt := range tests {
		if got := dialectFromArg(tt.arg); got != tt.want {
			t.Fatalf("dialectFromArg(%q) = %v, want %v", tt.arg, got, tt.want)
		}
	}
}

func TestMatchesDialect(t *testing.T) {
	tests := []struct {
		path    string
		dialect Dialect
		want    bool
	}{
		{path: "src/app.clj", dialect: CLJ, want: true},
		{path: "src/app.cljs", dialect: CLJS, want: true},
		{path: "src/app.joke", dialect: JOKER, want: true},
		{path: "deps.edn", dialect: EDN, want: true},
		{path: "src/app.cljc", dialect: CLJ, want: false},
	}

	for _, tt := range tests {
		if got := matchesDialect(tt.path, tt.dialect); got != tt.want {
			t.Fatalf("matchesDialect(%q, %v) = %v, want %v", tt.path, tt.dialect, got, tt.want)
		}
	}
}
