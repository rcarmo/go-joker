// Command modulecheck tests an external consumer against a local module proxy.
// It uses the current checkout as an immutable-version fixture, without replace
// directives or network access. Dependencies must already be in Go's download cache.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const moduleBase = "github.com/rcarmo/go-joker"

var releasePattern = regexp.MustCompile(`^v([1-9][0-9]*)\.[0-9]+\.[0-9]+$`)
var sourceVersionPattern = regexp.MustCompile(`(?m)^const VERSION = "([^"]+)"$`)

func checkIdentity(module, version string) error {
	match := releasePattern.FindStringSubmatch(version)
	if match == nil {
		return fmt.Errorf("invalid release version %q", version)
	}
	want := moduleBase
	if match[1] != "1" {
		want += "/v" + match[1]
	}
	if module != want {
		return fmt.Errorf("release %s requires module %s, got %s", version, want, module)
	}
	return nil
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	if err := check(root); err != nil {
		fmt.Fprintln(os.Stderr, "module consumer check:", err)
		os.Exit(1)
	}
}

func check(root string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	run := func(dir string, env []string, args ...string) ([]byte, error) {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir, cmd.Env = dir, env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, fmt.Errorf("%s: %w\n%s", strings.Join(args, " "), err, out)
		}
		return out, nil
	}
	// Disable workspace/flag overrides for both fixture preparation and consumer.
	env := append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	mod, err := run(root, env, "go", "mod", "edit", "-json")
	if err != nil {
		return err
	}
	var metadata struct {
		Module struct{ Path string }
		Go     string
	}
	if err := json.Unmarshal(mod, &metadata); err != nil {
		return err
	}
	source, err := os.ReadFile(filepath.Join(root, "core/runtime/version.go"))
	if err != nil {
		return err
	}
	match := sourceVersionPattern.FindSubmatch(source)
	if match == nil {
		return fmt.Errorf("source VERSION not found")
	}
	version, module := string(match[1]), metadata.Module.Path
	if err := checkIdentity(module, version); err != nil {
		return err
	}
	cache, err := run(root, env, "go", "env", "GOMODCACHE")
	if err != nil {
		return err
	}
	files, err := run(root, env, "git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if err != nil {
		return err
	}
	tmpRoot := os.Getenv("TMPDIR")
	if tmpRoot == "" { return fmt.Errorf("source scripts/project-env.sh before running modulecheck") }
	tmp, err := os.MkdirTemp(tmpRoot, "joker-module-check-")
	if err != nil {
		return err
	}
	defer func(){
		// Go's downloaded module files are read-only; make only this isolated
		// owned directory writable before removal.
		filepath.Walk(tmp, func(path string, info os.FileInfo, err error) error { if err==nil && info.IsDir(){_ = os.Chmod(path,0700)};return nil })
		os.RemoveAll(tmp)
	}()
	proxy := filepath.Join(tmp, "proxy")
	versions := filepath.Join(proxy, filepath.FromSlash(module), "@v")
	consumer := filepath.Join(tmp, "consumer")
	for _, dir := range []string{versions, consumer} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return err
	}
	goSum, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		return err
	}
	// Package the build inputs, including all generated sources and go:embed assets.
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	for _, file := range strings.Split(string(files), "\x00") {
		if !(strings.HasSuffix(file, ".go") || strings.HasSuffix(file, ".s") || strings.HasSuffix(file, ".h") ||
			file == "go.mod" || file == "go.sum" || file == "LICENSE" ||
			strings.HasPrefix(file, "internal/notebook/assets/") || strings.HasPrefix(file, "cmd/joker/notebook_assets/")) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			return err
		}
		entry, err := zw.Create(module + "@" + version + "/" + file)
		if err != nil {
			return err
		}
		if _, err := entry.Write(data); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	contents := map[string][]byte{
		filepath.Join(versions, version+".mod"):     goMod,
		filepath.Join(versions, version+".info"):    []byte(fmt.Sprintf(`{"Version":%q,"Time":"2026-01-01T00:00:00Z"}`, version)),
		filepath.Join(versions, version+".zip"):     archive.Bytes(),
		filepath.Join(consumer, "go.mod"):           []byte(fmt.Sprintf("module example.com/joker-consumer\n\ngo %s\n\nrequire %s %s\n", metadata.Go, module, version)),
		filepath.Join(consumer, "go.sum"):           goSum,
		filepath.Join(consumer, "consumer_test.go"): []byte(fmt.Sprintf(consumerTest, module, module, module, version)),
	}
	for path, data := range contents {
		if err := os.WriteFile(path, data, 0644); err != nil {
			return err
		}
	}
	fileURL := func(path string) string { return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String() }
	// A separate module cache prevents an existing public release from masking
	// checkout defects. All dependency downloads come from the existing file cache.
	env = append(env, "PROFILE_ROOT="+filepath.Join(os.Getenv("PROJECT_TMP_ROOT"), "runs/profiles/module-consumer"), "GOMODCACHE="+filepath.Join(tmp, "modcache"),
		"GOPROXY="+fileURL(proxy)+","+fileURL(filepath.Join(strings.TrimSpace(string(cache)), "cache/download")),
		"GOPRIVATE=", "GONOPROXY=none", "GOSUMDB=off", "GOTOOLCHAIN=local")
	for _, args := range [][]string{
		{"go", "mod", "tidy"},
		{"bash", filepath.Join(root, "scripts/test-profile.sh"), "./...", "--", "-count=1", "-v"},
		{"go", "build", "-mod=mod", "-o", filepath.Join(tmp, "joker"), module + "/cmd/joker"},
	} {
		out, err := run(consumer, env, args...)
		if err != nil {
			return err
		}
		fmt.Print(string(out))
	}
	fmt.Printf("verified external consumer and CLI for %s@%s (offline proxy; no replace)\n", module, version)
	return nil
}

const consumerTest = `package consumer

import (
 "strings"
 "testing"
 core "%s/core"
 runtime "%s/core/runtime"
 coretypes "%s/core/types"
)

func TestReleasedModule(t *testing.T) {
 if runtime.VERSION != %q { t.Fatalf("unexpected version: %%s", runtime.VERSION) }
 obj, err := core.TryRead(core.NewReader(strings.NewReader("(+ 20 22)"), "<consumer>"))
 if err != nil { t.Fatal(err) }
 expr, err := core.TryParse(obj, &core.ParseContext{GlobalEnv: core.GLOBAL_ENV})
 if err != nil { t.Fatal(err) }
 result, err := core.TryEval(expr)
 if err != nil { t.Fatal(err) }
 n, ok := result.(coretypes.Int)
 if !ok || n.I != 42 { t.Fatalf("unexpected result: %%v", result) }
}
`
