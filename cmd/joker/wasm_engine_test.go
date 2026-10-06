package main

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStandaloneNativeAndInterpreterWorkflow(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("compiler not supported")
	}
	bin := buildJokerBinary(t)
	dir := t.TempDir()
	source := filepath.Join(dir, "program.joke")
	output := filepath.Join(dir, "program")
	if runtime.GOOS == "windows" {
		output += ".exe"
	}
	code := `(require '[joker.jit :as jit])
(def sum (jit/compile-wasm (fn [n] (loop [i 0 total 0] (if (< i n) (recur (+ i 1) (+ total i)) total)))))
(println (jit/wasm-engine) (sum 100))
(println "arguments:" joker.core/*command-line-args*)`
	if err := os.WriteFile(source, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "compile", "--native", "--run", source, "-o", output)
	cmd.Env = append(os.Environ(), "JOKER_WASM_ENGINE=")
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "compiler 4950") {
		t.Fatalf("build/run: %v\n%s", err, out)
	}
	cmd = exec.Command(bin, "compile", "--native", "--run", source, "-o", output, "--", "--wasm-engine=invalid-script-value")
	cmd.Env = append(os.Environ(), "JOKER_WASM_ENGINE=")
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "invalid-script-value") {
		t.Fatalf("compile script flags hijacked: %v\n%s", err, out)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"compiler", "interpreter"} {
		cmd := exec.Command(output, "--wasm-engine="+mode)
		if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), mode+" 4950") {
			t.Fatalf("standalone %s: %v\n%s", mode, err, out)
		}
	}
	for _, word := range []string{"doc", "compile", "notebook"} {
		cmd = exec.Command(output, word)
		if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "arguments: ("+word+")") {
			t.Fatalf("argument %s hijacked: %v\n%s", word, err, out)
		}
	}
	cmd = exec.Command(output)
	cmd.Env = append(os.Environ(), "JOKER_WASM_ENGINE=interpreter")
	if out, err := cmd.CombinedOutput(); err != nil || !strings.Contains(string(out), "interpreter 4950") {
		t.Fatalf("environment override: %v\n%s", err, out)
	}
}

func TestWASMEngineOptions(t *testing.T) {
	args, mode, err := extractWasmEngine([]string{"--wasm-engine=native", "script.joke", "--", "--wasm-engine=bad"})
	if err != nil || mode != "compiler" || len(args) != 3 {
		t.Fatal(args, mode, err)
	}
	for _, args := range [][]string{{"--wasm-engine"}, {"--wasm-engine="}, {"--wasm-engine=invalid"}} {
		if _, _, err := extractWasmEngine(args); err == nil {
			t.Fatal("invalid flag accepted", args)
		}
	}
}
func TestStandaloneEngineMetadata(t *testing.T) {
	data, err := json.Marshal(standaloneMetadata{Source: "(+ 20 22)", Engine: "compiler"})
	if err != nil {
		t.Fatal(err)
	}
	src, mode, ok := decodeStandalonePayload(append([]byte(standaloneMetadataPrefix), data...))
	if !ok || src != "(+ 20 22)" || mode != "compiler" {
		t.Fatal(src, mode, ok)
	}
	if src, mode, ok = decodeStandalonePayload([]byte("legacy source")); !ok || src != "legacy source" || mode != "" {
		t.Fatal(src, mode, ok)
	}
	bin := append([]byte("native runtime"), data...)
	footer := make([]byte, 12)
	binary.LittleEndian.PutUint64(footer, uint64(len(data)))
	copy(footer[8:], standaloneMagic)
	bin = append(bin, footer...)
	if string(stripEmbeddedPayload(bin)) != "native runtime" {
		t.Fatal("strip metadata")
	}
	binary.LittleEndian.PutUint64(bin[len(bin)-12:], ^uint64(0))
	if !strings.Contains(string(stripEmbeddedPayload(bin)), "native runtime") {
		t.Fatal("malformed length")
	}
}
