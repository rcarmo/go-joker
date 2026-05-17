package core_test

import (
	. "github.com/rcarmo/go-joker/core"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"testing"
)

const stringIterNthScript = `(let [s "The quick brown fox jumps over the lazy dog and does many other things"
                  len (count s)]
              (loop [i 0 c 0]
                (if (= i len)
                  c
                  (let [ch (nth s i)]
                    (recur (+ i 1) (if (= ch \space) (+ c 1) c))))))`

const stringIterCursorScript = `(let [s "The quick brown fox jumps over the lazy dog and does many other things"
                  cur (string-cursor s)]
              (loop [c cur spaces 0]
                (if (cursor-done? c)
                  spaces
                  (let [ch (cursor-char c)]
                    (recur (cursor-next c) (if (= ch \space) (+ spaces 1) spaces))))))`

func TestStringIterationBenchmarksAgree(t *testing.T) {
	initBenchStringCursorProcs()
	for name, script := range map[string]string{
		"nth":    stringIterNthScript,
		"cursor": stringIterCursorScript,
	} {
		t.Run(name, func(t *testing.T) {
			got := Eval(compileBenchExpr(t, script), nil)
			if got == nil || !got.Equals(coretypes.MakeInt(13)) {
				t.Fatalf("%s string iteration benchmark = %v, want 13 spaces", name, got)
			}
		})
	}
}
