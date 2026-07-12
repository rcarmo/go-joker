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

Images can use `:url`, or `:data` and `:mime-type`. Assistant tool calls use `{:id ... :name ... :arguments ...}`; a result is a message with `{:role :tool :tool-call-id ... :content ...}`.

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

`stream!` emits `:start`, `:text-delta`, `:thinking-delta`, `:toolcall-delta`, `:toolcall-end`, and `:done` events. Every provider-derived event retains its decoded payload under `:raw`.

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

The default token store is `.joker-ai-tokens.edn`, written with mode `0600`. Bind `*token-store-path*`, or replace storage completely:

```joke
(binding [ai/*load-credentials* (fn [provider] (vault-read provider))
          ai/*save-credentials!* (fn [provider value]
                                   (vault-write provider value))]
  (ai/codex-login!))
```

`github-device-login!` obtains the long-lived GitHub token. `github-copilot-token!` exchanges it for a short-lived Copilot token. `codex-login!` creates a PKCE authorization URL, calls `*open-browser!*`, and asks `*oauth-prompt!*` for either the code or pasted callback URL. `active-credentials` refreshes short-lived credentials when required; pass the result as `{:credentials ...}` to `complete`, `stream!`, or `models`.

OAuth callbacks, progress reporting, browser opening, and storage are all bindable, which keeps the namespace usable in a terminal, service, or embedded process without assuming a particular UI.

## Adapters

An adapter is a map with `:request` and `:event` functions plus optional `:base-url`, `:headers`, `:models-url`, and fallback `:models` data.

```joke
(ai/register-provider!
 :local
 {:base-url "http://127.0.0.1:8080/v1"
  :request ai/chat-request
  :event ai/chat-stream-event!})
```

The request function translates normalized context into an HTTP request. The event function translates decoded SSE payloads into normalized events and accumulated state. Errors use `ex-info`; `ex-data` includes the category, provider status, redacted body, and partial stream state where available.

## Security and awkward edges

Diagnostics are off by default. With `*debug?*` enabled, authorization, cookies, token-like fields, API keys, secrets, and nested values are redacted before reaching `*diagnostic-fn*`. `*unsafe-debug?*` disables that protection and should only be bound in throw-away local tests.

The implementation does not execute tools, fetch image URLs, or validate model-produced JSON against the supplied schema. Codex login currently uses pasted-code callback handling rather than opening a local callback listener. Network exceptions raised by Joker's native HTTP stack are not retried; status-based failures are. These boundaries are intentional -- an embedding application should own side effects, policy, and secret management.

Run the offline checks from the repository root:

```bash
TMPDIR=$PWD/.cache/tmp GOTMPDIR=$PWD/.cache/gotmp go test ./std/http
.cache/tmp/joker --lint examples/ai/joker/ai.joke
.cache/tmp/joker tests/ai_test.joke
```
