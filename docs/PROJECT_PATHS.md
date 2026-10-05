# Project cache and temporary paths

On this host, all reproducible caches, generated build output and disposable work use `/workspace/tmp/go-joker/`:

| Path | Purpose |
| --- | --- |
| `cache/go-build`, `cache/go-mod` | Go build and module archives |
| `cache/gopath`, `cache/go-tools` | Downloadable tools and installed tool binaries |
| `cache/bun`, `cache/npm`, `cache/ms-playwright` | JS/package/browser caches |
| `cache/python`, `cache/pip`, `cache/uv`, `cache/xdg` | Tool caches, without adding Python profiling |
| `build/` | CLI, SDL host, debug binaries and release cross-builds |
| `runs/tmp`, `runs/go-build` | Parent directories for owned isolated tests and compiler scratch |

The vendored `scripts/project-tmp.sh` resolver uses a valid explicit `PROJECT_TMP_ROOT`, else writable `/workspace/tmp/go-joker`, then `RUNNER_TEMP/go-joker`, original `TMPDIR/go-joker`, or `/tmp/go-joker`. Invalid explicit roots fail; resolution precedes child TMPDIR export. No host Makefile is required.

Source `scripts/project-env.sh` for direct commands; Makefile recipes load it automatically. Bun notebook helpers load the matching module. Python benchmark launchers configure subprocess/cache paths only; they are not profiled by this change. The bootstrap rejects path escapes, unowned directories and symlinked project components; `/workspace` is the host's existing workspace alias.

GitHub CI maps the same hierarchy to `${github.workspace}/.project-tmp/go-joker`; CircleCI explicitly maps it to `/home/circleci/project-tmp/go-joker`. `PROJECT_TMP_ROOT` must be absolute and end in `/go-joker`. Test-owned roots remain isolated under `runs`; no test mutations point at real source/model/data state.

Retained CPU/heap profiles, matching binaries/logs, benchmark results and release receipts stay in repository `.cache/test-profiles`, `.cache/release-*` or documented benchmark evidence locations. Tracked screenshots stay in `docs/images`. These paths are excluded from disposable cleanup.

`make clean-cache CLEAN_CONFIRM=go-joker` removes only this project's `cache` and `build` trees. Stop jobs first. It does not remove `runs`, evidence, source, other projects or installed system dependencies. Old home/repository caches and retained evidence are not automatically moved or deleted; verify new-path execution and coordinate any disposal with the owner.
