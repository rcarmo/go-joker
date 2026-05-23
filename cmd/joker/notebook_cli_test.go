package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNotebookHelpMentionsSecurityFlags(t *testing.T) {
	bin := buildJokerBinary(t)
	cmd := exec.Command(bin, "notebook", "--help")
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("notebook --help: %v\n%s", err, out)
	}
	for _, want := range []string{"--token secret", "--readonly", "--summary"} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("notebook --help missing %q:\n%s", want, out)
		}
	}
}

func TestNotebookNewCLI(t *testing.T) {
	bin := buildJokerBinary(t)
	dir := t.TempDir()
	nb := filepath.Join(dir, "new.edn")
	cmd := exec.Command(bin, "notebook", "new", nb, "--title", "Created")
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("notebook new: %v\n%s", err, out)
	}
	data, err := os.ReadFile(nb)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), ":format :joker/notebook") || !strings.Contains(string(data), "Created") {
		t.Fatalf("new notebook:\n%s", data)
	}
}

func TestNotebookRunSummaryCLI(t *testing.T) {
	bin := buildJokerBinary(t)
	dir := t.TempDir()
	nb := filepath.Join(dir, "summary.edn")
	cmd := exec.Command(bin, "notebook", "new", nb, "--title", "Summary")
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("notebook new: %v\n%s", err, out)
	}
	cmd = exec.Command(bin, "notebook", "run", nb, "--no-save", "--summary")
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("notebook run --summary: %v\n%s", err, out)
	}
	for _, want := range []string{`"title":"Summary"`, `"cellCount":2`, `"ok":1`, `"idle":1`, `"errors":0`, `"id":"cell-2"`, `"state":"ok"`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestNotebookRunNoSaveCLI(t *testing.T) {
	bin := buildJokerBinary(t)
	dir := t.TempDir()
	nb := filepath.Join(dir, "nosave.edn")
	cmd := exec.Command(bin, "notebook", "new", nb, "--title", "NoSave")
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("notebook new: %v\n%s", err, out)
	}
	before, err := os.ReadFile(nb)
	if err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(bin, "notebook", "run", nb, "--no-save")
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("notebook run --no-save: %v\n%s", err, out)
	}
	after, err := os.ReadFile(nb)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("--no-save changed notebook\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

func TestNotebookDemoCLI(t *testing.T) {
	bin := buildJokerBinary(t)
	dir := t.TempDir()
	nb := filepath.Join(dir, "demo.edn")
	cmd := exec.Command(bin, "notebook", "demo", nb)
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("notebook demo: %v\n%s", err, out)
	}
	data, err := os.ReadFile(nb)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Joker notebook rich demo") || !strings.Contains(string(data), "joker.notebook/chart") {
		t.Fatalf("demo notebook:\n%s", data)
	}
}

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
	cmd = exec.Command(bin, "notebook", "validate", nb)
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	validateOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("notebook validate: %v\n%s", err, validateOut)
	}
	if !strings.Contains(string(validateOut), "notebook ok") {
		t.Fatalf("validate output: %s", validateOut)
	}
	cmd = exec.Command(bin, "notebook", "status", nb)
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	statusOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("notebook status: %v\n%s", err, statusOut)
	}
	if !strings.Contains(string(statusOut), `"cellCount"`) || !strings.Contains(string(statusOut), `"outputCount"`) {
		t.Fatalf("status output: %s", statusOut)
	}
	cmd = exec.Command(bin, "notebook", "snapshots", nb)
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	snapOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("notebook snapshots: %v\n%s", err, snapOut)
	}
	if strings.TrimSpace(string(snapOut)) != "[]" && (!strings.Contains(string(snapOut), `"path"`) || !strings.Contains(string(snapOut), `"size"`)) {
		t.Fatalf("snapshots output: %s", snapOut)
	}
	cmd = exec.Command(bin, "notebook", "restore", nb, "missing")
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "snapshot") {
		t.Fatalf("notebook restore missing expected snapshot error, err=%v out=%s", err, out)
	}
	cmd = exec.Command(bin, "notebook", "deps", nb)
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	depsOut, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("notebook deps: %v\n%s", err, depsOut)
	}
	if !strings.Contains(string(depsOut), `"nodes"`) || !strings.Contains(string(depsOut), `"cycles"`) {
		t.Fatalf("deps output: %s", depsOut)
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
