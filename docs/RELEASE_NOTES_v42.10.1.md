# Release Notes -- v42.10.1

This patch release consolidates and clarifies the documentation for the provider-neutral AI client, bounded HTTP/SSE APIs, repository workflows, and release verification introduced around `v42.10.0`.

## Documentation

* Added `joker.ai` to the project overview, contributor entry point, and runnable examples index.
* Clarified that `joker.ai` is an experimental, dependency-free example namespace rather than a standard-library namespace.
* Documented the canonical `make ai-check` lint and offline fixture gate, which is included in `make release-check`.
* Expanded client-side `joker.http/send` and `joker.http/send-sse` guidance for response/event limits, deadlines, callback cancellation, and bounded non-success responses.
* Clarified credential-store atomicity, repository ignore rules, default error redaction, and the explicit unsafe-debug opt-in.
* Updated the release checklist for major/minor/patch selection and current supply-chain validation.
* Recorded the completed `v42.10.0` AI audit remediation and release evidence.

## Portability and consistency

* Replaced active workspace-specific temporary paths with repository-local `.cache/tmp` and `.cache/gotmp` paths.
* Standardized CLI examples on `make cli` and `.cache/tmp/joker`, avoiding root build artifacts rejected by the layout guard.
* Corrected internal Markdown links and an unmatched tracing-document code fence.
* Updated tracing examples to use the repository's pure Joker SVG renderer without requiring a workspace-specific helper.

## Validation

The release was validated with Markdown link and code-fence checks, portability scans, `git diff --check`, `make docs-check`, and the complete `make pretag-check` release gate.
