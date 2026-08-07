## Audit notes

The useful part of `go-ai` is its small provider-neutral surface: normalized messages, explicit tools, structured output, and streaming events that do not leak transport details into the calling agent. Upstream `pi-ai` goes considerably further on provider compatibility -- especially GitHub Copilot and Codex authentication -- but its TypeScript runtime, package dependencies, and browser assumptions do not map directly onto Joker.

The resulting namespace keeps the data model and discards the dependency stack. Provider adapters translate the same Joker maps into Chat Completions, Responses, or Codex Responses requests, and stream adapters translate both SSE dialects back into one event vocabulary. Raw decoded provider payloads remain attached because normalisation inevitably loses new or vendor-specific fields.

## What Joker already had

Joker shipped JSON, EDN, base64, cryptography, random generation, file permissions, clocks, and an HTTP client/server. That is enough for request construction, PKCE, device authorization, token persistence, and ordinary model discovery without adding a Clojure dependency.

The missing piece was response streaming. `joker.http/send` buffered the entire body, so wrapping it in a lazy sequence would merely look asynchronous while retaining the response and offering no meaningful cancellation. `joker.http/send-sse` now parses SSE incrementally in native Go, invokes a synchronous callback, supports callback cancellation, applies a per-request timeout, bounds event size, and returns a bounded body for non-2xx responses.

## Protocol boundaries

OpenAI-compatible and OpenCode requests use Chat Completions. Their messages contain `text` and `image_url` blocks, nested function definitions, incremental `choices[0].delta` text, and indexed tool-call fragments.

OpenAI uses Responses. Input blocks become `input_text` and `input_image`; tools are top-level function records; structured output belongs under `text.format`; stream event names identify text, reasoning summaries, function argument fragments, completed output items, and terminal response state.

Codex is close to Responses but uses the ChatGPT backend, `codex/responses`, `store: false`, short-lived OAuth credentials, and an optional ChatGPT account identifier. Treating it as a plain OpenAI-compatible endpoint hides precisely the details that tend to break.

GitHub Copilot uses Chat Completions plus editor-identification headers. Authentication is two-stage: GitHub's device flow obtains a long-lived token, then the Copilot token endpoint returns the short-lived bearer token and API endpoint. Static fallback models are only a safety net when discovery is unavailable.

## Normalised contract

Messages use keyword roles and either strings or vectors of typed content blocks. Tool definitions and tool results are plain maps. Successful completion returns accumulated text, thinking text, tool calls, model/provider identity, stop information, and provider response data where available.

Streaming emits explicit lifecycle and delta events through a callback rather than lazy I/O. Returning `false` closes the response. External cancellation uses a dynamically bound predicate, which avoids introducing a particular channel or task library into the API.

Provider HTTP and stream failures become `ex-info` values with structured `ex-data`. The namespace preserves partial state on interrupted provider streams and redacts diagnostics unless unsafe debugging is explicitly enabled.

## Authentication and persistence

API keys and endpoint configuration are dynamic vars; the namespace never reads environment variables. OAuth tokens default to a bounded EDN file updated through private temporary files and atomic replacement with mode `0600`; in-process updates are serialized. Load/save functions remain bindable for keychains, databases, tests, or an embedding application's own encrypted store.

GitHub device authorization, Copilot exchange, Codex PKCE authorization-code exchange, and Codex refresh are implemented. Browser launching, prompting, and progress reporting are callbacks. Codex currently expects a pasted code or callback URL instead of running a temporary localhost receiver -- a deliberate limitation until Joker has a small, stoppable one-shot HTTP listener.

## Known gaps

The native SSE function bounds each event rather than imposing a total stream byte cap; long-lived streams are expected to be cancelled by their owner. Streaming is SSE-only -- JSONL is deliberately not advertised. Transport, provider, OAuth, JSON decoding, cancellation, timeout, and size failures are normalized into structured `ex-info` at the AI boundary.

`complete-with-tools` validates and executes registered handlers with bounded rounds and duplicate-call protection, but JSON Schema validation remains outside this namespace. Audio, video, file upload, batch jobs, realtime WebSockets, and provider-specific prompt caching are also outside the current example.

Offline fixtures cover both event dialects, deterministic interleaved tool calls, tool execution, request conversion for images/tools/JSON schemas, credential activation, strict callback parsing, structured failures, and redaction. Native tests cover bounded ordinary responses, SSE parsing, multiline data, cancellation, deadlines, and invalid limits. Live checks remain opt-in because credentials and provider availability should never decide whether Joker's repository tests pass.
