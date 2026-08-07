## joker.ai remediation baseline

Baseline audited on 2026-08-07 at repository HEAD `22971aaa03db8459d089709f4eb1a98077222ff2`.

The dependency map is `examples/ai/dependency-map.svg`. The implementation entered the repository in `d85aa870` and remains an experimental example namespace rather than a standard namespace.

## Acceptance matrix

| Area | Baseline gap | Closure evidence |
| --- | --- | --- |
| Diagnostics | Serialized bodies expose prompts, tool arguments/results and image data | Safe diagnostic tests prove payload fields are omitted; unsafe binding proves explicit opt-in |
| HTTP bounds | Ordinary `joker.http/send` uses unbounded `io.ReadAll` | Configurable bound, overflow contract, native tests, model/auth integration tests |
| Errors | JSON/native failures do not consistently become structured `ex-info` | Category/status/provider/raw/partial-state assertions for every failure family |
| Tools | Tool calls are represented but not executed in a bounded continuation loop | Multi-round mock conversations, invalid/duplicate/missing tool tests, cancellation limit |
| Credentials | Refresh is manual and GitHub credential endpoint is ignored | Precedence and refresh tests across `complete`, `stream!`, and `models` |
| Responses tools | Multiple calls have unstable final order | Interleaved multi-call fixture preserves output order and raw records |
| Adapters | Registration validates only `:request` and concrete map classes | Callable hook validation and structured errors for invalid adapters |
| URL handling | Form/query helpers are ASCII-only | UTF-8, malformed escape, duplicate parameter, state mismatch tests |
| Token store | Write precedes chmod and replacement is not atomic | Private temporary creation, atomic replacement, bounded/malformed/concurrent tests |
| Streaming | SSE is the supported streaming contract; JSONL is deliberately unsupported | Documentation explicitly limits the API to SSE and no capability claim advertises JSONL |
| SSE safety | Close errors can mask primary failures; edge semantics incomplete | Native callback/close/zero-limit/body-close tests |
| Providers | Existing fixtures test conversion, not transactions | Offline HTTP suites for OpenAI, compatible, GitHub, OpenCode and Codex |
| Authentication | OAuth paths lack full offline protocol coverage | Device, exchange, PKCE, refresh, expiry, endpoint, malformed body and persistence tests |
| Live smoke | One generic script does not cover provider-specific setup | Explicit opt-in scripts/modes for every provider family |
| Documentation | Capability claims exceed tested behaviour | README/AUDIT/examples/map agree with executable contracts |
| Release integration | AI tests are absent from canonical CI/release gates | Make/CI/release checks invoke lint and offline AI suite |

## Release closure

The remediation shipped in `v42.10.0` on 2026-08-07.

Closure evidence:

* `core/runtime/version.go`, the top-level README, and `docs/RELEASE_NOTES_v42.10.0.md` agree on `v42.10.0`.
* `examples/README.md` lists `examples/ai`, and `make release-check` includes `make ai-check` for lint plus offline fixtures.
* `make pretag-check` passed before tagging.
* The annotated tag and `master` were pushed; the tag-triggered workflow published six binaries, six SPDX SBOMs, and `SHA256SUMS`.
* All published checksums were downloaded and verified after publication.
