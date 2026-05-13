# Web Runtime + New Namespaces Guide

_Last updated: 2026-05-08_

This guide documents the new concurrency/web runtime surface and namespace additions in go-joker v42.8.2+.

---

## 1) `joker.http` web runtime: HTTP client, WebSocket, SSE

### 1.0 Persistent HTTP client

`joker.http/client` creates a reusable keep-alive HTTP client with an explicit connection pool. Pass it as `:client` in request maps to avoid socket tear-down churn across repeated calls.

```clojure
(require '[joker.http :as http])

(def c (http/client {:max-idle-conns 256
                    :max-idle-conns-per-host 64
                    :idle-timeout-ms 120000}))

(http/get "https://example.com" {:client c})
(http/post "https://example.com/api" "payload" {:client c
                                                :headers {"Content-Type" "text/plain"}})
(http/close-client c)
```

Available helpers:

- `client`, `close-client`
- `send` / `request` for full request maps
- `get`, `post`, `put`, `delete`

`joker.http/start-server` remains Ring-style:

- handler signature: `(fn [request-map] response-map)`
- request map keys include: `:request-method`, `:uri`, `:query-string`, `:body`, `:headers`, `:scheme`, `:host`, `:remote-addr`, etc.

### 1.1 Standard HTTP response

```clojure
{:status 200
 :headers {"Content-Type" "text/plain"}
 :body "Hello"}
```

### 1.2 WebSocket upgrade response

If handler returns `:websocket`, the HTTP connection is upgraded.

```clojure
{:websocket
 {:on-open    (fn [send close] ...)
  :on-message (fn [msg] ...)
  :on-close   (fn [] ...)}}
```

Callback contract:

- `:on-open` gets two fns:
  - `send`: `(fn [msg])` — sends a text frame
  - `close`: `(fn [])` — closes the connection
- `:on-message`: `(fn [msg])` for each incoming text frame
- `:on-close`: `(fn [])` when socket closes

Example:

```clojure
(defn handler [req]
  (if (= (:uri req) "/ws")
    {:websocket
     {:on-open    (fn [send close]
                    (send "Welcome"))
      :on-message (fn [msg]
                    (println "client says:" msg))
      :on-close   (fn []
                    (println "socket closed"))}}
    {:status 200 :body "Use /ws"}))
```

### 1.3 SSE / streaming response

If handler returns `:stream`, Joker uses `http.Flusher` and streams events.

```clojure
{:status 200
 :headers {"Cache-Control" "no-cache"}
 :stream (fn [send-event] ...)}
```

`send-event` supports:

- `(send-event data)` → emits
  ```
  data: <data>\n\n
  ```
- `(send-event event data)` → emits
  ```
  event: <event>\n
  data: <data>\n\n
  ```

Example:

```clojure
(defn handler [req]
  (if (= (:uri req) "/events")
    {:stream (fn [send]
               (dotimes [i 5]
                 (send (str "tick " i))
                 (joker.time/sleep 1000))
               (send "done" "complete"))}
    {:status 200 :body "Use /events"}))
```

Default SSE headers are applied:

- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`
- `Connection: keep-alive`

---

## 2) `joker.http.router` (Bottle-style routing)

File: `std/http/router/router.joke`

Load with classpath pointing at `std/http/router` or package it with the HTTP namespace resources. The classpath root must contain `joker/http/router.joke`, so `std/http/router` is the root for this namespace:

```bash
/workspace/tmp/go-joker --classpath std/http/router your_app.clj
```

Then:

```clojure
(require '[joker.http.router :refer [router GET POST PUT DELETE ANY wrap wrap-cors wrap-logger response]])
```

Example:

```clojure
(def app
  (wrap
    (router
      [(GET "/" (fn [_] (response "Hello")))
       (GET "/users/:id"
         (fn [req]
           (response (str "User " (get-in req [:path-params :id])))))
       (POST "/users" (fn [req] {:status 201 :body (:body req)}))])
    wrap-logger
    wrap-cors))

(joker.http/start-server "127.0.0.1:8080" app)
```

Features:

- method dispatch (`GET/POST/PUT/DELETE/ANY`)
- path params (`/users/:id`)
- wildcard segment (`*`)
- middleware composition (`wrap`)
- helpers: `response`, `not-found`, `redirect`, `json-response`, `prefix`

---

## 3) Concurrency and `clojure.core.async`

Core forms/fns:

- `go`, `chan`, `<!`, `>!`, `close!`
- `alts!`, `timeout`
- `future`, `future-call`, `future?`
- `promise`, `deliver`, `promise?`, `realized?`
- `agent`, `send`, `send-off`, `await`, `agent-error`
- `pmap`, `pcalls`

`clojure.core.async` is now available as a compatibility namespace over the same GIL-free channel runtime. Channel close is idempotent and safe to call from concurrent goroutines; sends after close report failure instead of panicking, and takes from closed channels yield `nil`.

- aliases: `chan`, `<!`, `>!`, `<!!`, `>!!`, `close!`, `alts!`, `timeout`, `go`
- macros: `go-loop`, `thread`, `thread-call`
- async callbacks: `put!`, `take!`
- channel construction/loading: `buffer`, `dropping-buffer`, `sliding-buffer`, `promise-chan`, `to-chan`, `to-chan!`, `onto-chan`, `onto-chan!`
- pipelines: `pipe`, `merge`, `split`, `map<`, `filter<`, `map>`, `filter>`
- reductions: `reduce`, `into`
- broadcast/topic helpers: `mult`, `tap`, `untap`, `untap-all`, `pub`, `sub`, `unsub`, `unsub-all`

Example:

```clojure
(require '[clojure.core.async :as a])

(let [c (a/chan 1)]
  (a/go-loop [i 0]
    (if (< i 3)
      (do (a/>! c i) (recur (inc i)))
      (a/close! c)))
  [(a/<!! c) (a/<!! c) (a/<!! c)])
```

These run on true Go scheduler goroutines. `go`/`go-loop` are intentionally real goroutines, not IOC state machines.

---

## 4) New namespaces

### 4.1 `joker.random`

- `int`
- `int-n`
- `int-between`
- `float`
- `boolean`
- `choice`
- `shuffle`
- `uuid`
- `secure-bytes`
- `secure-int`

Example:

```clojure
(require '[joker.random])
(joker.random/int-n 100)
(joker.random/choice [:a :b :c])
(joker.random/secure-bytes 16)
```

### 4.2 `joker.log`

- `debug`
- `info`
- `warn`
- `error`
- `set-level!`
- `get-level`
- `logf`

Example:

```clojure
(require '[joker.log])
(joker.log/set-level! :debug)
(joker.log/info "server started" {:port 8080})
```

### 4.3 `joker.edn` and `edn`

- `read-string`
- `write-string`

These reuse Joker reader/printer behavior without evaluating forms and are also used for EDN pod payloads.

```clojure
(require '[joker.edn :as edn])
(= {:ok true} (edn/read-string (edn/write-string {:ok true})))
```

### 4.4 `joker.transit`

Transit+JSON support now includes cache refs, set/list/quote/cmap tags, numeric payloads, `write-verbose`, and helper functions used by pod payloads.

### 4.5 `pods` and `babashka.pods`

Compatibility namespaces support `load-pod`, `invoke`, dynamic vars from pod `describe`, and JSON/EDN/Transit+JSON payload formats. End-to-end fake-pod coverage lives in `std/pods` tests.

---

## 5) Extended namespace: `joker.math`

Added in this cycle:

- `tan`, `asin`, `acos`, `atan`, `atan2`
- `sinh`, `cosh`, `tanh`
- `remainder`, `fmod`
- `max-val`, `min-val`
- `degrees`, `radians`

---

## 6) Notes on compatibility

- WebSocket and SSE are **Ring-compatible extensions** (extra response keys).
- Existing handlers returning plain `{:status ... :headers ... :body ...}` continue to work unchanged.
- Routing/middleware can be used purely at Clojure level (no Go changes needed in app code).
