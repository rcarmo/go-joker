package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestNoArgumentReplStarts(t *testing.T) {
	bin := buildJokerBinary(t)
	cmd := exec.Command(bin)
	cmd.Env = notebookCLIEnv(t)
	cmd.Stdin = strings.NewReader("(exit)\n")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("joker repl start: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "Welcome to joker") || !strings.Contains(string(out), "user=>") {
		t.Fatalf("unexpected repl output:\n%s", out)
	}
}
