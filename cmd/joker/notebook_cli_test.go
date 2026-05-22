package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotebookRunAndExportCLI(t *testing.T) {
	bin := buildJokerBinary(t)
	dir := t.TempDir()
	nb := filepath.Join(dir, "basic.edn")
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "notebooks", "basic.edn"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nb, data, 0644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "notebook", "run", nb)
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("notebook run: %v\n%s", err, out)
	}
	updated, err := os.ReadFile(nb)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(updated), ":state :ok") || !strings.Contains(string(updated), ":text \"3\"") {
		t.Fatalf("updated notebook missing output:\n%s", updated)
	}
	outPath := filepath.Join(dir, "report.md")
	cmd = exec.Command(bin, "notebook", "status", nb)
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	statusOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("notebook status: %v\n%s", err, statusOut)
	}
	if !strings.Contains(string(statusOut), `"cellCount"`) || !strings.Contains(string(statusOut), `"outputCount"`) {
		t.Fatalf("status output: %s", statusOut)
	}
	cmd = exec.Command(bin, "notebook", "export", nb, "-o", outPath)
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("notebook export: %v\n%s", err, out)
	}
	md, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(md), "# Basic notebook fixture") || !strings.Contains(string(md), "```clojure") {
		t.Fatalf("markdown export:\n%s", md)
	}
}

func buildJokerBinary(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "joker")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/joker")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	return bin
}
