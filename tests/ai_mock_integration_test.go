package tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAIProvidersAgainstOfflineMock(t *testing.T) {
	var retryRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/device":
			_, _ = w.Write([]byte(`{"device_code":"device","user_code":"CODE","verification_uri":"https://example.test","interval":0}`))
			return
		case "/github-token":
			_, _ = w.Write([]byte(`{"access_token":"github-refresh"}`))
			return
		case "/copilot":
			_, _ = w.Write([]byte(`{"token":"copilot-access","expires_at":9999999999,"endpoints":{"api":"https://copilot.example"}}`))
			return
		case "/codex-token":
			_, _ = w.Write([]byte(`{"access_token":"codex-access","refresh_token":"codex-refresh","expires_in":3600,"id_token":"id"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"mock-model"}]}`))
			return
		}
		if strings.HasPrefix(r.URL.Path, "/retry/") && retryRequests.Add(1) == 1 {
			http.Error(w, "retry", http.StatusTooManyRequests)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/error/") {
			http.Error(w, "provider failure", http.StatusBadGateway)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/malformed/") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {not-json}\n\n"))
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.HasSuffix(r.URL.Path, "/responses") {
			_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"mock\"}}\n\n"))
			return
		}
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer server.Close()

	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`
(load-file "examples/ai/joker/ai.joke")
(def failures (atom []))
(doseq [provider [:openai :openai-compatible :github :opencode :codex]]
  (try
    (let [result (joker.ai/complete provider
                   {:messages [{:role :user :content "hello"}]}
                   {:base-url %q :api-key "offline" :model "mock-model"
                    :credentials {:access "offline" :expires 9999999999999}})]
      (when-not (= "ok" (:text result))
        (swap! failures conj [provider :completion result])))
    (catch coretypes.Error error
      (swap! failures conj [provider :error (ex-data error)]))))
(let [catalogue (joker.ai/models :openai
                 {:base-url %q :api-key "offline"
                  :credentials {:access "offline"}})]
  (when-not (= "mock-model" (get-in catalogue [:models 0 :id]))
    (swap! failures conj [:models catalogue])))
(let [retry-result (joker.ai/complete :openai
                    {:messages [{:role :user :content "retry"}]}
                    {:base-url %q :api-key "offline" :model "mock-model"
                     :credentials {:access "offline"}})]
  (when-not (= "ok" (:text retry-result))
    (swap! failures conj [:retry retry-result])))
(binding [joker.ai/*cancelled?* (fn [] true)]
  (try
    (joker.ai/complete :openai {:messages []}
                       {:base-url %q :api-key "offline" :model "mock-model"
                        :credentials {:access "offline"}})
    (swap! failures conj [:cancellation :not-cancelled])
    (catch coretypes.Error error
      (when-not (= :cancelled (:category (ex-data error)))
        (swap! failures conj [:cancellation (ex-data error)])))))
(doseq [[label path expected]
        [[:malformed "/malformed" :invalid-provider-json]
         [:provider-error "/error" :provider-error]]]
  (try
    (joker.ai/complete :openai {:messages []}
                       {:base-url (str %q path) :api-key "offline" :model "mock-model"
                        :max-retries 0 :credentials {:access "offline"}})
    (swap! failures conj [label :missing-error])
    (catch coretypes.Error error
      (when-not (= expected (:category (ex-data error)))
        (swap! failures conj [label (ex-data error)])))))
(let [saved (atom {})]
  (binding [joker.ai/*save-credentials!* (fn [provider value]
                                           (swap! saved assoc provider value))
            joker.ai/*oauth-progress!* (fn [_] nil)]
    (joker.ai/github-device-login! {:device-url %q :token-url %q})
    (when-not (= "github-refresh" (get-in @saved [:github :refresh]))
      (swap! failures conj [:github-device @saved]))
    (joker.ai/github-copilot-token! {:refresh "github-refresh"}
                                    {:copilot-token-url %q
                                     :base-url "https://configured.example"})
    (when-not (= "copilot-access" (get-in @saved [:github :access]))
      (swap! failures conj [:copilot @saved]))
    (joker.ai/codex-exchange-code! {:redirect-uri "http://localhost/cb"
                                    :verifier "verifier"}
                                   "code" {:token-url %q})
    (when-not (= "codex-access" (get-in @saved [:codex :access]))
      (swap! failures conj [:codex-exchange @saved]))
    (joker.ai/refresh-codex! {:refresh "old"} {:token-url %q})
    (when-not (= "codex-refresh" (get-in @saved [:codex :refresh]))
      (swap! failures conj [:codex-refresh @saved]))))
(when (seq @failures)
  (println @failures)
  (os/exit 1))
`, server.URL, server.URL, server.URL+"/retry", server.URL, server.URL,
		server.URL+"/device", server.URL+"/github-token", server.URL+"/copilot",
		server.URL+"/codex-token", server.URL+"/codex-token")
	path := filepath.Join(t.TempDir(), "ai-mock.joke")
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "run", "./cmd/joker", path)
	cmd.Dir = root
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("offline provider integration failed: %v\n%s", err, output)
	}
}
