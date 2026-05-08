package http

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	. "github.com/candid82/joker/core"
	ws "github.com/gorilla/websocket"
)

var client = &http.Client{}

var upgrader = ws.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func extractMethod(request Map) string {
	if ok, m := request.Get(MakeKeyword("method")); ok {
		switch m := m.(type) {
		case String:
			return m.S
		case Keyword:
			return m.ToString(false)[1:]
		case Symbol:
			return m.ToString(false)
		default:
			panic(RT.NewError(fmt.Sprintf("method must be a string, keyword or symbol, got %s", m.GetType().ToString(false))))
		}
	}
	return "get"
}

func getOrPanic(m Map, k Object, errMsg string) Object {
	if ok, v := m.Get(k); ok {
		return v
	}
	panic(RT.NewError(errMsg))
}

func mapToReq(request Map) *http.Request {
	method := strings.ToUpper(extractMethod(request))
	url := EnsureObjectIsString(getOrPanic(request, MakeKeyword("url"), ":url key must be present in request map"), "url: %s").S
	var reqBody io.Reader
	if ok, b := request.Get(MakeKeyword("body")); ok {
		reqBody = strings.NewReader(EnsureObjectIsString(b, "body: %s").S)
	}
	req, err := http.NewRequest(method, url, reqBody)
	PanicOnErr(err)
	if ok, headers := request.Get(MakeKeyword("headers")); ok {
		h := EnsureObjectIsMap(headers, "headers: %s")
		for iter := h.Iter(); iter.HasNext(); {
			p := iter.Next()
			req.Header.Add(EnsureObjectIsString(p.Key, "header name: %s").S, EnsureObjectIsString(p.Value, "header value: %s").S)
		}
	}
	if ok, host := request.Get(MakeKeyword("host")); ok {
		req.Host = EnsureObjectIsString(host, "host: %s").S
	}
	return req
}

func reqToMap(host String, port String, req *http.Request) Map {
	defer req.Body.Close()
	res := EmptyArrayMap()
	body, err := io.ReadAll(req.Body)
	PanicOnErr(err)
	res.Add(MakeKeyword("request-method"), MakeKeyword(strings.ToLower(req.Method)))
	res.Add(MakeKeyword("body"), MakeString(string(body)))
	res.Add(MakeKeyword("uri"), MakeString(req.URL.Path))
	res.Add(MakeKeyword("query-string"), MakeString(req.URL.RawQuery))
	res.Add(MakeKeyword("server-name"), host)
	res.Add(MakeKeyword("server-port"), port)
	remoteAddr := req.RemoteAddr
	if i := strings.LastIndexByte(remoteAddr, byte(':')); i >= 0 {
		remoteAddr = remoteAddr[:i]
	}
	res.Add(MakeKeyword("remote-addr"), MakeString(remoteAddr))
	res.Add(MakeKeyword("protocol"), MakeString(req.Proto))
	res.Add(MakeKeyword("scheme"), MakeKeyword("http"))
	res.Add(MakeKeyword("host"), MakeString(req.Host))
	headers := EmptyArrayMap()
	for k, v := range req.Header {
		headers.Add(MakeString(strings.ToLower(k)), MakeString(strings.Join(v, ",")))
	}
	res.Add(MakeKeyword("headers"), headers)
	return res
}

func respToMap(resp *http.Response) Map {
	defer resp.Body.Close()
	res := EmptyArrayMap()
	body, err := io.ReadAll(resp.Body)
	PanicOnErr(err)
	res.Add(MakeKeyword("body"), MakeString(string(body)))
	res.Add(MakeKeyword("status"), MakeInt(resp.StatusCode))
	respHeaders := EmptyArrayMap()
	for k, v := range resp.Header {
		respHeaders.Add(MakeString(k), MakeStringVector(v))
	}
	res.Add(MakeKeyword("headers"), respHeaders)
	// TODO: 32-bit issue
	res.Add(MakeKeyword("content-length"), MakeInt(int(resp.ContentLength)))
	return res
}

func mapToResp(response Map, w http.ResponseWriter) {
	status := 0
	if ok, s := response.Get(MakeKeyword("status")); ok {
		status = EnsureObjectIsInt(s, "HTTP response status: %s").I
	}
	body := ""
	if ok, b := response.Get(MakeKeyword("body")); ok {
		body = EnsureObjectIsString(b, "HTTP response body: %s").S
	}
	if ok, headers := response.Get(MakeKeyword("headers")); ok {
		header := w.Header()
		h := EnsureObjectIsMap(headers, "HTTP response headers: %s")
		for iter := h.Iter(); iter.HasNext(); {
			p := iter.Next()
			hname := EnsureObjectIsString(p.Key, "HTTP response header name %s").S
			switch pvalue := p.Value.(type) {
			case String:
				header.Add(hname, pvalue.S)
			case Seqable:
				s := pvalue.Seq()
				for !s.IsEmpty() {
					header.Add(hname, EnsureObjectIsString(s.First(), "HTTP response header value: %s").S)
					s = s.Rest()
				}
			default:
				panic(RT.NewError("HTTP response header value must be a string or a seq of strings"))
			}
		}
	}
	if status != 0 {
		w.WriteHeader(status)
	}
	io.WriteString(w, body)
}

func sendRequest(request Map) Map {
	req := mapToReq(request)
	resp, err := client.Do(req)
	PanicOnErr(err)
	return respToMap(resp)
}

func startServer(addr string, handler Callable) Object {
	i := strings.LastIndexByte(addr, byte(':'))
	host, port := MakeString(addr), MakeString("")
	if i != -1 {
		host = MakeString(addr[:i])
		port = MakeString(addr[i+1:])
	}
	err := http.ListenAndServe(addr, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				w.WriteHeader(500)
				io.WriteString(w, "Internal server error")
				fmt.Fprintln(os.Stderr, r)
			}
		}()
		response := handler.Call([]Object{reqToMap(host, port, req)})
		respMap := EnsureObjectIsMap(response, "HTTP response: %s")

		// Check for WebSocket upgrade
		if ok, wsConf := respMap.Get(MakeKeyword("websocket")); ok {
			handleWebSocket(w, req, EnsureObjectIsMap(wsConf, "websocket config: %s"))
			return
		}

		// Check for SSE/streaming response
		if ok, streamFn := respMap.Get(MakeKeyword("stream")); ok {
			handleStream(w, respMap, EnsureObjectIsCallable(streamFn, "stream value must be callable: %s"))
			return
		}

		mapToResp(respMap, w)
	}))
	PanicOnErr(err)
	return NIL
}

func startFileServer(addr string, root string) Object {
	err := http.ListenAndServe(addr, http.FileServer(http.Dir(root)))
	PanicOnErr(err)
	return NIL
}

// handleWebSocket upgrades the connection and runs callbacks.
// Config map keys:
//
//	:on-open    (fn [send-fn close-fn]) — called once after upgrade
//	:on-message (fn [msg]) — called for each text message received
//	:on-close   (fn []) — called when connection closes
func handleWebSocket(w http.ResponseWriter, req *http.Request, conf Map) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "websocket upgrade error:", err)
		return
	}
	defer conn.Close()

	// Build send-fn: (fn [msg]) sends a text message
	sendFn := Proc{Name: "ws-send", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		msg := args[0].ToString(false)
		if err := conn.WriteMessage(ws.TextMessage, []byte(msg)); err != nil {
			panic(RT.NewError("websocket send error: " + err.Error()))
		}
		return NIL
	}}

	// Build close-fn: (fn []) closes the connection
	closeFn := Proc{Name: "ws-close", Fn: func(args []Object) Object {
		conn.Close()
		return NIL
	}}

	// Call on-open
	if ok, onOpen := conf.Get(MakeKeyword("on-open")); ok {
		EnsureObjectIsCallable(onOpen, "on-open must be callable: %s").Call([]Object{sendFn, closeFn})
	}

	// Read loop
	_, onMsg := conf.Get(MakeKeyword("on-message"))
	var onMsgFn Callable
	if onMsg != nil {
		onMsgFn = EnsureObjectIsCallable(onMsg, "on-message must be callable: %s")
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if onMsgFn != nil {
			onMsgFn.Call([]Object{MakeString(string(message))})
		}
	}

	// Call on-close
	if ok, onClose := conf.Get(MakeKeyword("on-close")); ok {
		EnsureObjectIsCallable(onClose, "on-close must be callable: %s").Call([]Object{})
	}
}

// handleStream writes an SSE/chunked streaming response.
// The stream fn receives a send-event callback: (fn [event-data])
// Response map can include :status and :headers (applied before streaming).
// Default Content-Type is text/event-stream.
func handleStream(w http.ResponseWriter, respMap Map, streamFn Callable) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(500)
		io.WriteString(w, "Streaming not supported")
		return
	}

	// Apply headers
	header := w.Header()
	header.Set("Content-Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	if ok, headers := respMap.Get(MakeKeyword("headers")); ok {
		h := EnsureObjectIsMap(headers, "stream headers: %s")
		for iter := h.Iter(); iter.HasNext(); {
			p := iter.Next()
			header.Set(EnsureObjectIsString(p.Key, "header name: %s").S,
				EnsureObjectIsString(p.Value, "header value: %s").S)
		}
	}

	// Apply status
	status := 200
	if ok, s := respMap.Get(MakeKeyword("status")); ok {
		status = EnsureObjectIsInt(s, "stream status: %s").I
	}
	w.WriteHeader(status)
	flusher.Flush()

	// Build send-event fn: (fn [data]) or (fn [event data])
	sendEvent := Proc{Name: "sse-send", Fn: func(args []Object) Object {
		switch len(args) {
		case 1:
			// data-only: sends "data: ...\n\n"
			data := args[0].ToString(false)
			for _, line := range strings.Split(data, "\n") {
				fmt.Fprintf(w, "data: %s\n", line)
			}
			fmt.Fprint(w, "\n")
		case 2:
			// event + data: sends "event: ...\ndata: ...\n\n"
			event := args[0].ToString(false)
			data := args[1].ToString(false)
			fmt.Fprintf(w, "event: %s\n", event)
			for _, line := range strings.Split(data, "\n") {
				fmt.Fprintf(w, "data: %s\n", line)
			}
			fmt.Fprint(w, "\n")
		default:
			panic(RT.NewError("send-event expects 1 or 2 args: [data] or [event data]"))
		}
		flusher.Flush()
		return NIL
	}}

	// Call the stream function with send-event
	streamFn.Call([]Object{sendEvent})
}
