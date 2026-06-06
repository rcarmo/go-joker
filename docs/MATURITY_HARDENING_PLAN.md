# Maturity hardening plan

Updated: 2026-06-06

## Goal

Move go-joker from an advanced, actively evolving runtime fork toward a stable scripting platform by tightening release hygiene, validating examples as supported surfaces, labelling API stability, and converting recent compiler/runtime fixes into permanent regression coverage.

## Current maturity summary

The project is strong in runtime capability, examples, documentation, benchmarking, and CI guardrails. The main maturity limits are:

1. root `core` still concentrates evaluator/runtime/compiler/WASM responsibilities;
2. WASM and IR are powerful but subtle and recently exposed bridge bugs;
3. stdlib/API growth needs a visible stability policy;
4. examples are useful but were not yet first-class CI surfaces;
5. docs can grow stale as paths and commands move;
6. release notes and tags need to map cleanly to shipped artifacts.

## Workstreams

### M1 — Example surface as CI contract

**Problem:** Examples are now part of the developer/user experience, but most are not guarded by CI.

**Actions:**

- [x] Add `tests/examples_smoke.sh`.
- [x] Add `make examples-check`.
- [x] Wire `examples-check` into `make docs-check`.
- [x] Run wiki static build smoke.
- [x] Run wiki dynamic serve smoke.
- [x] Run small WASM fractal render smoke.
- [x] Check expected example files exist.
- [ ] Add optional terminal/TUI non-interactive validation if `joker.term` gains a dry-run/test hook.

**Definition of done:** CI fails when documented runnable examples break.

### M2 — Documentation path and command guardrails

**Problem:** Example/script paths move during refactors and stale docs are easy to miss.

**Actions:**

- [x] Add `tests/docs_paths_guard.sh`.
- [x] Add `make docs-paths-check`.
- [x] Wire `docs-paths-check` into `make docs-check`.
- [x] Guard stale examples paths for the pre-reorganization fractal, Tetris, wiki-static, and sushy-static locations without keeping those literal paths in docs.
- [ ] Extend the guard when more examples are moved or renamed.

**Definition of done:** stale high-value paths are caught automatically.

### M3 — API stability matrix

**Problem:** The namespace surface is large and users need to know what is stable vs beta/experimental.

**Actions:**

- [x] Add `docs/API_STABILITY.md`.
- [x] Classify major namespaces as stable, beta, experimental, or internal/diagnostic.
- [ ] Add a docs-check assertion that `docs/API_STABILITY.md` exists and references core new namespaces.
- [ ] Add per-namespace stability metadata later if the runtime docs generator grows support for it.

**Definition of done:** new public namespaces must be deliberately classified.

### M4 — WASM regression suite

**Problem:** Realistic numeric kernels exposed WASM bridge bugs.

**Actions:**

- [x] Existing targeted WASM tests pass after the value-if/fn-loop fixes.
- [ ] Add named regression tests for:
  - value-producing `if` inside loop body;
  - fn-level loop with init stores and non-zero recur target;
  - nested `let` in WASM-eligible loop;
  - `from-rgba32-domain-fn` calling a `compile-wasm` kernel;
  - stale “eligible for pure WASM backend” diagnostic as an error.
- [ ] Include these in `runtime-contract-check`.

**Definition of done:** every fixed WASM bridge bug is represented by a named test.

### M5 — Release hygiene

**Problem:** `master` often moves beyond the last patch tag.

**Actions:**

- [ ] Add a release checklist doc.
- [ ] Decide whether examples/docs-only user-facing changes require patch bumps.
- [ ] Avoid adding “current master additions” to already-tagged release notes unless explicitly marked as post-tag.
- [ ] Add a command to compare `VERSION`/README/release-note/tag consistency.

**Definition of done:** release notes describe the tag, not an ambiguous moving target.

### M6 — Root-core decomposition follow-up

**Problem:** maintainability is limited by root `core` ownership of evaluator/compiler/runtime seams.

**Actions:**

- [ ] Continue only contract-first moves.
- [ ] Prioritize WASM/IR regression coverage before further compiler movement.
- [ ] Keep avoiding fake wrappers and cosmetic package moves.
- [ ] Use the existing refactor docs as the canonical boundary plan.

**Definition of done:** each extraction reduces actual coupling and is guarded by tests.

## First execution slice

This plan starts by implementing M1–M3 because they are low-risk and immediately improve project maturity without destabilizing runtime internals.

Completed in this slice:

- `docs/MATURITY_HARDENING_PLAN.md`
- `docs/API_STABILITY.md`
- `tests/examples_smoke.sh`
- `tests/docs_paths_guard.sh`
- `make examples-check`
- `make docs-paths-check`
- `docs-check` integration
