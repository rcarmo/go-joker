# Introspection-driven Joker agent

`introspective-agent.joke` is the policy-driven successor to the deliberately unsafe `lisp-agent.joke`. It generates capability descriptions from live Joker metadata, exposes five bounded agent tools, and denies invocation unless a namespace-qualified public function is in the active profile.

This design reduces accidental authority. **It is not a language sandbox or a security boundary.** Joker metadata, allowlists, form inspection, and macro expansion all run inside a process. Hostile code still requires operating-system isolation.

## Architecture

```text
user → OpenRouter → metadata-derived system catalog
                    ↓ tool call
       isolated tool child (wall-clock bound)
                    ↓
       strict request validation → profile decision
                    ↓
       ns-resolve + metadata/arity check → invocation
                    ↓
       bounded, redacted observation envelope → model
```

The OpenRouter tools are:

- `search-symbols`: deterministic public-symbol search over profile namespaces;
- `describe-symbol`: metadata, normalized arglists, source provenance, and a generated JSON Schema;
- `list-namespace-publics`: bounded deterministic namespace inventory;
- `macroexpand-form`: exactly-one-form parsing, pre-expansion analysis, bounded macro expansion, and post-expansion analysis;
- `invoke-capability`: namespace-qualified, allowlisted function dispatch.

In addition, the OpenRouter request contains a bounded set of per-capability tools generated directly from each Var's live metadata schema (for example, `cap_joker_z46_string_z47_join`). Names use a collision-free OpenRouter-safe encoding and are tested for uniqueness. Those tools normalize named fixed arguments or positional overloaded/variadic arguments and enter the same controlled dispatcher; they do not bypass policy. The five generic tools remain available for discovery and explicit dispatch.

Run the credential-free interfaces directly:

```bash
./joker examples/agents/introspective-agent.joke --search join joker.string 10
./joker examples/agents/introspective-agent.joke --describe joker.string/join
./joker examples/agents/introspective-agent.joke --namespace joker.json 20
./joker examples/agents/introspective-agent.joke --macroexpand pure-data '(when true (+ 1 2))'
./joker examples/agents/introspective-agent.joke --invoke pure-data joker.string/join \
  '["-",["a","b"]]'
./joker examples/agents/introspective-agent.joke --tools
./joker examples/agents/introspective-agent.joke --probe
./joker examples/agents/introspective-agent.joke --self-test
```

For the model loop:

```bash
export OPENROUTER_API_KEY=sk-or-...
export JOKER_AGENT_PROFILE=pure-data       # default
export AGENT_MEMORY=.cache/introspective-agent-memory.json
./joker examples/agents/introspective-agent.joke \
  'Join alpha, beta, and gamma with slashes.'
```

`--forget` removes the persisted transcript. `--eval` intentionally does not exist in the hardened example.

## Discovery and schema contract

Discovery has explicit namespace scope, public-only defaults, stable qualified-name sorting, a default result limit of 20, and a hard limit of 100. `--probe` records bounded runtime shapes for `all-ns`, `ns-publics`, `ns-map`, `ns-resolve`, `meta`, `macroexpand`, and `joker.repl/apropos`. The probe exposed and the port fixes an `apropos` bug where stringifying a Namespace produced an invalid symbol namespace instead of using `ns-name`; `tests/eval/repl.joke` is the regression test. A description reports:

- qualified and unqualified name, namespace, and kind;
- documentation and normalized arglists;
- type tag and macro, dynamic, private, and invocation flags;
- file, line, and column metadata where available;
- a deterministic metadata-derived parameter schema.

One unambiguous fixed arglist becomes an object schema with named required properties and `additionalProperties: false`. Overloaded and variadic arglists use an `args` array with conservative minimum/maximum item bounds. These schemas are sent to OpenRouter as the parameter schemas of the generated per-capability tools. Missing metadata falls back to an unconstrained positional array instead of inventing types. JSON cannot represent Joker symbols, functions, lazy sequences, or tagged runtime objects; schemas therefore describe transport shape, not semantic type safety.

## Profiles and policy domains

The shipped executable profiles are:

| Profile | Authority |
| --- | --- |
| `discover-only` | Public metadata discovery; invocation denied. |
| `pure-data` | Explicit pure computation, collection/string, and JSON functions. No macros are invocable; a small macro allowlist exists only for inspected expansion. |
| `filesystem` | Named policy profile, disabled until a dedicated-root argument policy exists. |
| `environment` | Named policy profile, disabled until explicit variable-name and redaction policy exists. |
| `http-network` | Named policy profile, disabled until URL scheme/host policy exists. |
| `namespace-loading` | Named policy profile, disabled. |
| `subprocesses` | Named policy profile, disabled. |
| `unrestricted-evaluation` | Marker profile only; points callers to the unsafe compatibility example and exposes no hardened capability. |

The policy table marks filesystem, environment, HTTP/network, namespace-loading, subprocess, and unrestricted-evaluation domains deny-by-default and isolation-required. Disabled profiles intentionally have empty namespace and function allowlists, so selecting one still produces a structured denial. Each needs argument-aware constraints such as dedicated filesystem roots, explicit environment-variable names and redaction, or URL scheme/host allowlists. Enable a new profile only after implementing those checks and the matching OS policy.

An invocation must pass all of these checks:

1. known profile and vector arguments;
2. exact namespace-qualified allowlist membership;
3. public runtime resolution through `ns-publics`/`ns-resolve`;
4. callable, non-macro metadata;
5. metadata arity compatibility where arglists exist.

Resolution happens again at invocation time. Unknown, private, dynamic, macro, stale, and wrong-arity requests receive categorized envelopes. Runtime failures and timeouts are captured at the child-process boundary.

## Form analysis

`macroexpand-form` wraps input in a list and reads it, requiring exactly one form. It recursively collects symbols (excluding quoted data), resolves each symbol, records source metadata, and rejects unresolved, private, dynamic, or non-profile references. It then macroexpands with a depth limit of 12 and a rendered-size limit of 16,384 characters and analyzes the expansion again.

This is intentionally conservative and is not a complete lexical Clojure/Joker analyzer. The hardened path does not evaluate analyzed forms; controlled function invocation remains the execution mechanism. Macro functions can themselves execute code during expansion, so only explicitly allowed macros are expanded and the operation runs in the isolated tool child when called by the agent. Malformed reads happen in that bounded child and return an `invalid-form` envelope instead of terminating the parent CLI.

## Observability and data handling

Operations return `{ok, operation, value, meta}` or `{ok: false, error}` envelopes. Invocation observations include requested/resolved symbols, metadata provenance, normalized argument counts and runtime types (not values), result type and bounded preview, policy decision, and elapsed nanoseconds. Errors have stable categories. Child stdout/stderr is bounded to 4,096 rendered characters and values of secret-like environment keys are replaced with `[REDACTED]`.

Do not put secrets in prompts, tool arguments, or memory. Redaction is defense in depth, not proof that arbitrary error strings cannot disclose sensitive data.

## Isolation

Capability invocation and model-originated tools use a `timeout`-wrapped Joker child by default (`JOKER_AGENT_CHILD=1`). `JOKER_AGENT_TIMEOUT` controls the wall-clock limit in seconds (default `5`). The parent model conversation is additionally capped at 12 model turns and 32 tool calls. Set `JOKER_AGENT_CHILD=0` only for trusted local debugging. The child boundary captures crashes and prevents a long result rendering from blocking the parent, but `timeout` alone does **not** limit memory, CPU consumption, filesystem access, process creation, or networking.

Run the entire agent as an unprivileged container with, at minimum:

- a read-only root filesystem and no host paths, Docker socket, SSH agent, or device mounts;
- a dedicated writable volume containing only `AGENT_MEMORY`;
- a minimal environment containing only the OpenRouter credential;
- CPU, memory, process-count, and wall-clock limits;
- network egress restricted to the OpenRouter endpoint (and separately approved hosts for any future network profile);
- seccomp/AppArmor or equivalent policy that denies unnecessary syscalls and subprocesses.

Illustrative Docker flags (adapt the image and network policy to your environment):

```bash
docker run --rm --read-only --user 65532:65532 \
  --cpus 0.5 --memory 128m --pids-limit 32 \
  --cap-drop ALL --security-opt no-new-privileges \
  --tmpfs /tmp:rw,noexec,nosuid,size=16m \
  -v agent-memory:/memory \
  -e OPENROUTER_API_KEY -e AGENT_MEMORY=/memory/history.json \
  go-joker-agent:local
```

A general evaluator must additionally run in a disposable worker with enforced egress and filesystem policy. The compatibility example does not create such a worker and remains unsafe.

## Migration from `joker-eval`

1. Keep `lisp-agent.joke` only for the upstream-compatible educational demonstration.
2. List the exact namespace-qualified functions needed by a task.
3. Put only pure/data capabilities into an in-process policy profile; implement argument-aware checks for every external resource.
4. Replace generated code requests with `invoke-capability` calls using JSON-compatible arguments.
5. Enable the default child runner and deploy the whole process under OS/container limits.
6. Exercise `tests/introspective_agent_test.sh` with adversarial inputs before adding authority.

There is no compatibility promise for unrestricted forms. If a task genuinely requires arbitrary Joker evaluation, use a separate isolated service rather than weakening `pure-data`.

## Threat model and non-goals

The design addresses malformed model tool calls, accidental API hallucinations, unknown/private/dynamic symbols, direct allowlist bypasses, macro-expansion escapes, oversized forms/results, hangs, and common secret leakage in diagnostics. Tests cover these cases and deterministic prompt/schema generation.

It does not defend against a compromised Joker runtime, malicious allowed native functions, kernel/container escapes, side channels, denial of service without host resource limits, semantic abuse of an over-broad allowed function, prompt injection, or credentials deliberately supplied as model data.

## Verification and benchmark

```bash
tests/introspective_agent_test.sh .
tests/examples_smoke.sh .
tests/docs_paths_guard.sh .
make docs-check
git diff --check
./joker examples/agents/introspective-agent.joke --benchmark
```

`--benchmark` performs 20 in-process iterations and reports total/average nanoseconds for one symbol description and the bounded profile catalog. On one development run, description averaged about 1.23 ms and catalog generation about 136 ms. These figures are illustrative, include the current runtime implementation, and are not a performance guarantee. Catalog generation happens once per system prompt; tool dispatch does not regenerate it. On memory load, the stale first system message is always replaced with a freshly generated prompt for the active profile and runtime metadata.
