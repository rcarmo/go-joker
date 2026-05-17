package git

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"testing"

	gitConfig "github.com/go-git/go-git/v5/config"
	. "github.com/rcarmo/go-joker/core"
)

func expectGitPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}

func TestEnsureArgIsGitRepoChecksMissingArg(t *testing.T) {
	expectGitPanic(t, func() { EnsureArgIsGitRepo(nil, 0) })
}

func TestConfigPlacesURLRewritesUnderURLs(t *testing.T) {
	cfg := gitConfig.NewConfig()
	cfg.URLs["ssh://example/"] = &gitConfig.URL{Name: "ssh://example/", InsteadOf: "gh:"}
	m := makeConfigMap(cfg)
	ok, urlsObj := m.Get(MakeKeyword("urls"))
	if !ok {
		t.Fatal("missing :urls")
	}
	urls := urlsObj.(Map)
	if ok, _ := urls.Get(coretypes.MakeString("ssh://example/")); !ok {
		t.Fatalf("url rewrite missing from :urls: %s", urlsObj.ToString(false))
	}
	_, branches := m.Get(MakeKeyword("branches"))
	if ok, _ := branches.(Map).Get(coretypes.MakeString("ssh://example/")); ok {
		t.Fatal("url rewrite incorrectly placed under :branches")
	}
}
