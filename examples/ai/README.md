## joker.ai

`joker.ai` is a provider-neutral AI client built entirely from Joker's shipped namespaces. It covers the two protocols that matter in practice -- OpenAI Chat Completions and the newer Responses API -- while keeping provider peculiarities in small data-driven adapters.

The implementation is an example namespace rather than part of Joker's standard library. Run examples from `examples/ai`, or put that directory on the namespace load path used by the embedding application.

```joke
(ns example
  (:require [joker.ai :as ai]))

(binding [ai/*provider* :openai
          ai/*api-key* "sk-..."
          ai/*model* "gpt-4.1"]
  (ai/complete {:messages [{:role :user :content "Say hello."}]}))
```

No environment variables are read. Applications bind `*api-key*`, `*base-url*`, `*model*`, `*account-id*`, timeout/retry controls, cancellation checks, and credential callbacks around each operation.

## Providers and protocols

| Provider | Default protocol | Default endpoint | Authentication |
| --- | --- | --- | --- |
| `:openai` | Responses | `https://api.openai.com/v1` | API key |
| `:openai-compatible` | Chat Completions | `http://localhost:11434/v1` | Optional API key |
| `:github` | Chat Completions | GitHub Copilot API | GitHub device flow, then Copilot token exchange |
| `:opencode` | Chat Completions | OpenCode Zen | API key |
| `:codex` | Codex Responses | ChatGPT backend | OAuth authorization code with PKCE |

`models` asks the provider's `/models` endpoint. GitHub, OpenCode, and Codex have deliberately small fallback lists because model catalogues change faster than this example should.

## Messages, images, tools and JSON

A context contains normalized messages and optional tools:

```joke
{:messages [{:role :user
             :content [{:type :text :text "What is in this image?"}
                       {:type :image
                        :mime-type "image/png"
                        :data "...base64..."}]}]
 :tools [{:name "weather"
          :description "Read current weather"
          :parameters {:type "object"
                       :properties {:city {:type "string"}}
                       :required ["city"]}}]}
```

Images can use `:url`, or `:data` and `:mime-type`. Assistant tool calls use `{:id ... :name ... :arguments ...}`; a result is a message with `{:role :tool :tool-call-id ... :content ...}`. Tool definitions may include a callable `:handler`; `complete-with-tools` validates and invokes handlers, rejects malformed or duplicate calls, limits execution rounds, and continues until the provider returns a terminal assistant response.

`complete-json` adds the provider's structured-output form and parses the resulting text:

```joke
(ai/complete-json
 {:messages [{:role :user :content "Return the population of Lisbon."}]}
 {:name "population"
  :schema {:type "object"
           :properties {:city {:type "string"}
                        :population {:type "integer"}}
           :required ["city" "population"]}})
```

## Streaming

`stream!` consumes SSE and emits `:start`, `:text-delta`, `:thinking-delta`, `:toolcall-delta`, `:toolcall-end`, and `:done` events. Every provider-derived event retains its decoded payload under `:raw`. JSONL streaming is intentionally not part of this API; providers must expose SSE or supply a separate transport adapter.

```joke
(binding [ai/*provider* :openai-compatible
          ai/*base-url* "http://localhost:11434/v1"
          ai/*model* "qwen3:8b"]
  (ai/stream!
   {:messages [{:role :user :content "Explain SSE in one paragraph."}]}
   (fn [event]
     (when (= :text-delta (:type event))
       (print (:delta event)))
     true)))
```

Returning `false` cancels the native response body. `*cancelled?*` supplies an external cancellation check. The native transport also enforces `*timeout-ms*` and `*max-event-bytes*`; retryable HTTP statuses use bounded exponential backoff controlled by `*max-retries*` and `*retry-base-ms*`.

## Credentials

The default token store is `.joker-ai-tokens.edn`, written with mode `0600` through a private temporary file and atomic replacement. That filename is ignored throughout this repository, but applications should still treat it as a credential file, keep it out of backups/logs where appropriate, and bind `*token-store-path*` or replace storage completely:

```joke
(binding [ai/*load-credentials* (fn [provider] (vault-read provider))
          ai/*save-credentials!* (fn [provider value]
                                   (vault-write provider value))]
  (ai/codex-login!))
```

`github-device-login!` obtains the long-lived GitHub token. `github-copilot-token!` exchanges it for a short-lived Copilot token. `codex-login!` creates a PKCE authorization URL, calls `*open-browser!*`, and asks `*oauth-prompt!*` for either the code or pasted callback URL. `complete`, `stream!`, and `models` automatically load and refresh stored credentials. Explicit operation options override dynamic bindings, which override stored credentials and adapter defaults.

OAuth callbacks, progress reporting, browser opening, and storage are all bindable, which keeps the namespace usable in a terminal, service, or embedded process without assuming a particular UI.

## Adapters

An adapter is a map with callable `:request` and `:event` hooks plus optional `:base-url`, `:headers`, `:models-url`, and callable `:models` or `:auth` hooks. Registration and use validate the hook contract.

```joke
(ai/register-provider!
 :local
 {:base-url "http://127.0.0.1:8080/v1"
  :request ai/chat-request
  :event ai/chat-stream-event!})
```

The request function translates normalized context into an HTTP request. The event function translates decoded SSE payloads into normalized events and accumulated state. Errors use `ex-info`; by default, sensitive `ex-data` fields such as `:body` and `:raw` are replaced with `"[REDACTED]"`, while category, provider status, and safe partial state remain available. Binding `*unsafe-debug?*` to true preserves raw fields and is an explicit secret-exposure opt-in.

## Security and awkward edges

Diagnostics are off by default. With `*debug?*` enabled, authorization, cookies, token-like fields, API keys, secrets, and nested values are redacted before reaching `*diagnostic-fn*`. `*unsafe-debug?*` disables that protection and should only be bound in throw-away local tests.

The implementation executes only explicitly registered tool handlers and does not fetch image URLs or validate model-produced JSON against the supplied schema. Codex login currently uses strict pasted-code callback handling rather than opening a local callback listener. Transport failures are normalized as structured errors; status-based transient failures are retried. Embedding applications still own tool policy, schema validation, and secret management.

Run the canonical offline AI check from the repository root:

```bash
make ai-check
```

Native HTTP bounds and SSE contracts are covered by `go test ./std/http`; the complete repository release gate runs both through `make release-check`. Optional live checks in `tests/ai_live_smoke.joke` are credential-gated and are never part of normal offline validation.
