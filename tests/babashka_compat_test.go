package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func buildJokerForCompat(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(t.TempDir(), "joker")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/joker")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build joker: %v\n%s", err, out)
	}
	return bin
}

func TestBabashkaCompatibilityPositiveFixtures(t *testing.T) {
	bin := buildJokerForCompat(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "path": r.URL.Path})
	}))
	defer server.Close()

	fixtures, err := filepath.Glob("babashka_compat/positive/*.joke")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no positive Babashka compatibility fixtures found")
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			cmd := exec.Command(bin, fixture)
			cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp", "BB_COMPAT_HTTP_URL="+server.URL+"/compat")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s failed: %v\n%s", fixture, err, out)
			}
			if !strings.Contains(string(out), "OK") {
				t.Fatalf("%s did not report OK:\n%s", fixture, out)
			}
		})
	}
}

func TestBabashkaCompatibilityExpectedFailureFixtures(t *testing.T) {
	bin := buildJokerForCompat(t)
	fixtures, err := filepath.Glob("babashka_compat/expected_failure/*.joke")
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no expected-failure Babashka compatibility fixtures found")
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			content, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatal(err)
			}
			want := ""
			for _, line := range strings.Split(string(content), "\n") {
				if strings.HasPrefix(line, ";; EXPECT-ERROR:") {
					want = strings.TrimSpace(strings.TrimPrefix(line, ";; EXPECT-ERROR:"))
					break
				}
			}
			if want == "" {
				t.Fatalf("%s missing EXPECT-ERROR marker", fixture)
			}
			cmd := exec.Command(bin, fixture)
			cmd.Env = append(os.Environ(), "TMPDIR=/workspace/tmp", "GOTMPDIR=/workspace/tmp")
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("%s unexpectedly succeeded:\n%s", fixture, out)
			}
			if !strings.Contains(string(out), want) {
				t.Fatalf("%s error did not contain %q:\n%s", fixture, want, out)
			}
		})
	}
}
