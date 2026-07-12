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

## Why Joker introspection is the interesting next step

Joker exposes Clojure-style runtime metadata and namespace discovery: `all-ns`, `ns-publics`, `ns-map`, `ns-resolve`, `meta`, `macroexpand`, and `joker.repl/apropos`. A safer and more capable successor can use those APIs to discover and describe available operations instead of giving the model unrestricted `eval`. See the project discussion accompanying this example for a staged design.

## Attribution

Based on [`jamiebeach/lisp-agent`](https://github.com/jamiebeach/lisp-agent), copyright Jamie Beach, distributed under the MIT License. The Joker port is distributed under go-joker's Eclipse Public License 1.0.
