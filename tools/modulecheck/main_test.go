package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Fail before proxy/build preparation, even though local replace-based builds
// would accept this historical module/version combination.
func TestCheckRejectsOriginalRelease(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "core/runtime"), 0755); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		"go.mod":                  "module " + moduleBase + "\n\ngo 1.25.0\n",
		"core/runtime/version.go": "package runtime\nconst VERSION = \"v42.11.2\"\n",
	} {
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := check(root); err == nil || !strings.Contains(err.Error(), "requires module "+moduleBase+"/v42") {
		t.Fatalf("original release must fail with a module identity error; got %v", err)
	}
}

func TestReleaseModuleIdentity(t *testing.T) {
	for _, tc := range []struct {
		module, version string
		valid           bool
	}{
		{moduleBase + "/v42", "v42.11.3", true},
		{moduleBase, "v1.8.0", true},
		{moduleBase, "v42.11.2", false}, // Original published-tag defect.
		{moduleBase + "/v41", "v42.11.3", false},
		{moduleBase + "/v42", "v1.8.0", false},
		{moduleBase + "/v42", "v42.11.3+incompatible", false},
		{moduleBase + "/v42", "latest", false},
	} {
		t.Run(tc.module+"@"+tc.version, func(t *testing.T) {
			if err := checkIdentity(tc.module, tc.version); (err == nil) != tc.valid {
				t.Fatalf("checkIdentity = %v; want valid=%v", err, tc.valid)
			}
		})
	}
}
