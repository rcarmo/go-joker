# Release Notes -- v42.10.0

This minor release adds a provider-neutral AI client example and hardens Joker's native HTTP support for bounded ordinary responses and incremental SSE consumption.

## AI client

* Added data-driven adapters for OpenAI Responses, OpenAI-compatible Chat Completions, GitHub Copilot, OpenCode, and Codex.
* Added normalized messages, multimodal request construction, structured-output requests, deterministic streamed tool-call accumulation, model discovery, retries, cancellation, and structured errors.
* Added a bounded provider-neutral tool execution loop with validated handlers, decoded arguments, duplicate-call protection, normalized tool results, and terminal-response continuation.
* Added GitHub device authorization and Copilot token exchange, plus Codex PKCE authorization, token exchange, and refresh.
* Added automatically activated credentials with explicit option, dynamic binding, stored credential, and adapter-default precedence.
* Added strict UTF-8 OAuth form and callback handling.
* Added bounded, private, atomic EDN credential persistence with bindable storage callbacks.
* Diagnostics remain disabled by default and redact prompts, serialized bodies, tool data, images, credentials, and raw provider payloads unless unsafe debugging is explicitly enabled.

## HTTP and streaming

* Added `joker.http/send-sse`, which incrementally parses SSE, preserves event metadata, supports synchronous callback cancellation and request deadlines, and bounds complete events.
* Added an 8 MiB default bound for ordinary `joker.http/send` response bodies through `:max-response-bytes`.
* Ensured response bodies close on success and failure without close errors masking primary SSE failures.
* Defined positive size-limit contracts and added exact-limit, overflow, cancellation, timeout, and resource-closure coverage.

## Scope

* Streaming support is SSE-only. JSONL is deliberately not advertised.
* JSON Schema requests and JSON decoding are supported, but semantic schema validation remains the caller's responsibility.
* Live provider checks are optional and credential-gated; repository validation remains offline.

## Validation

The release was validated with fresh Joker binaries, Joker lint and fixture tests, focused HTTP tests, the complete Go test suite, `go vet`, formatting checks, and repository diff checks.
