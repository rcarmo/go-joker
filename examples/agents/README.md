# Lisp agent, in Joker

This is a Joker port of Jamie Beach's MIT-licensed [`lisp-agent`](https://github.com/jamiebeach/lisp-agent). The original demonstrates a complete Common Lisp agent with one tool—`eval`—and JSON transcript persistence. This version keeps the same architecture while using Joker's built-in HTTP and JSON namespaces, so it has no package dependencies.

```text
user → OpenRouter → joker-eval tool call → eval → tool result ─┐
  ↑                                                            │
  └──────────── persisted JSON history ← final response ←──────┘
```

## Run it

Build Joker if needed:

```bash
make cli
```

Set an OpenRouter key and optionally select another tool-capable model:

```bash
export OPENROUTER_API_KEY=sk-or-...
export OPENROUTER_MODEL=anthropic/claude-sonnet-4.5 # optional
export AGENT_MEMORY=.cache/lisp-agent-memory.json  # optional

./joker examples/agents/lisp-agent.joke \
  'What is the 30th Fibonacci number? Compute it rather than recalling it.'
./joker examples/agents/lisp-agent.joke 'My name is Rui.'
./joker examples/agents/lisp-agent.joke 'What is my name?'
./joker examples/agents/lisp-agent.joke --forget
```

The local evaluator can be exercised without an API key:

```bash
./joker examples/agents/lisp-agent.joke --eval '(reduce + (range 1 101))'
# 5050
```

## What maps to what

| Common Lisp original | Joker port |
| --- | --- |
| Dexador | `joker.http` and a persistent HTTP client |
| Shasht | `joker.json` |
| hash tables/vectors | maps/vectors |
| `read-from-string` + `eval` | `read-string` + `eval` |
| recursive `agent-loop` | tail-recursive `loop`/`recur` |
| UIOP environment access | `joker.os/env` |

Like the original, memory is the complete JSON message history, including tool calls and results. There is deliberately no framework, database, planner, or hidden state machine.

## Security warning

**The model can execute arbitrary Joker code with all privileges of the Joker process.** That includes reading environment variables and files, making network requests, loading namespaces, and starting child processes. Use a disposable container with:

- no host directories or sockets mounted;
- only the required API key in its environment;
- an unprivileged user and read-only root filesystem;
- explicit CPU, memory, process, network, and wall-clock limits;
- a dedicated writable volume only for `AGENT_MEMORY`.

This is an educational example, not a production sandbox.

## Introspection-driven successor

`introspective-agent.joke` is the hardened successor under development. Its first stage is already runnable without credentials:

```bash
./joker examples/agents/introspective-agent.joke --describe joker.string/join
./joker examples/agents/introspective-agent.joke --search join
./joker examples/agents/introspective-agent.joke --namespace joker.json 20
```

Joker exposes Clojure-style runtime metadata and namespace discovery through `all-ns`, `ns-publics`, `ns-map`, `ns-resolve`, `meta`, `macroexpand`, and `joker.repl/apropos`. The successor uses those APIs instead of teaching the model a stale, hand-written API catalog.

### Contract

The hardened path follows these rules:

1. **Discovery is scoped and bounded.** Searches inspect an explicit list of namespaces, expose public Vars by default, sort results by qualified name, and cap user-visible result counts at 100.
2. **Descriptions are data, not prose.** Each symbol record has a qualified name, namespace, unqualified name, kind, documentation, normalized arglists, type tag, macro/dynamic/private flags, source location, and invocation eligibility.
3. **Schemas are conservative.** Only metadata that can be represented unambiguously becomes a typed tool parameter. Overloaded, variadic, missing, or contradictory arglists fall back to a generic argument-array schema rather than inventing constraints.
4. **Invocation is denied by default.** A symbol must be namespace-qualified, public, callable, non-macro, and present in the active capability profile. Resolution at invocation time prevents a catalog entry from becoming stale.
5. **Forms are inspected twice.** Policy-checked evaluation reads exactly one form, validates its symbols, expands macros with depth and size limits, and validates the expansion again.
6. **Results use envelopes.** Successes and failures are structured maps with stable categories. Errors, traces, and previews are bounded; secrets and full environment values are never included.
7. **Metadata is not a sandbox.** Policy checks reduce accidental capability use, but hostile general evaluation must still execute in an isolated child process or container with operating-system limits.
8. **Compatibility is explicit.** `lisp-agent.joke` remains the small, unsafe upstream-style demonstration. Hardened behavior lives in `introspective-agent.joke`; callers must deliberately select the unsafe path.

The current implementation covers deterministic namespace discovery and metadata-backed symbol descriptions. Schema generation, controlled invocation, form analysis, profiles, observability, and isolation are the next staged additions.

## Attribution

Based on [`jamiebeach/lisp-agent`](https://github.com/jamiebeach/lisp-agent), copyright Jamie Beach, distributed under the MIT License. The Joker port is distributed under go-joker's Eclipse Public License 1.0.
