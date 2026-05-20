package http

import (
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	ws "github.com/gorilla/websocket"
	. "github.com/rcarmo/go-joker/core"
	"github.com/rcarmo/go-joker/core/hashutil"
)

var client = newPersistentHTTPClient(100, 100, 90*time.Second)

type HTTPClient struct {
	client    *http.Client
	transport *http.Transport
	hash      uint32
}

func newPersistentHTTPClient(maxIdle, maxIdlePerHost int, idleTimeout time.Duration) *http.Client {
	return &http.Client{Transport: &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          maxIdle,
		MaxIdleConnsPerHost:   maxIdlePerHost,
		IdleConnTimeout:       idleTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}}
}

func newHTTPClient(maxIdle, maxIdlePerHost int, idleTimeout time.Duration) *HTTPClient {
	tr := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          maxIdle,
		MaxIdleConnsPerHost:   maxIdlePerHost,
		IdleConnTimeout:       idleTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	hc := &HTTPClient{client: &http.Client{Transport: tr}, transport: tr}
	hc.hash = hashutil.Ptr(uintptr(unsafe.Pointer(hc)))
	return hc
}

func (hc *HTTPClient) ToString(escape bool) string                          { return "#object[HTTPClient]" }
func (hc *HTTPClient) Equals(other interface{}) bool                        { return hc == other }
func (hc *HTTPClient) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (hc *HTTPClient) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return hc }
func (hc *HTTPClient) GetType() *coretypes.Type                             { return TYPE.Proc }
func (hc *HTTPClient) Hash() uint32                                         { return hc.hash }

var upgrader = ws.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func extractMethod(request coretypes.Map) string {
	if ok, m := request.Get(coretypes.MakeKeyword(STRINGS.Intern, "method")); ok {
		switch m := m.(type) {
		case coretypes.String:
			return m.S
		case coretypes.Keyword:
			return m.ToString(false)[1:]
		case coretypes.Symbol:
			return m.ToString(false)
		default:
			panic(RT.NewError(fmt.Sprintf("method must be a string, keyword or symbol, got %s", m.GetType().ToString(false))))
		}
	}
	return "get"
}

func getOrPanic(m coretypes.Map, k coretypes.Object, errMsg string) coretypes.Object {
	if ok, v := m.Get(k); ok {
		return v
	}
	panic(RT.NewError(errMsg))
}

func mapToReq(request coretypes.Map) *http.Request {
	method := strings.ToUpper(extractMethod(request))
	url := coretypes.EnsureObjectIsString(getOrPanic(request, coretypes.MakeKeyword(STRINGS.Intern, "url"), ":url key must be present in request map"), "url: %s").S
	var reqBody io.Reader
	if ok, b := request.Get(coretypes.MakeKeyword(STRINGS.Intern, "body")); ok {
		reqBody = strings.NewReader(coretypes.EnsureObjectIsString(b, "body: %s").S)
	}
	req, err := http.NewRequest(method, url, reqBody)
	PanicOnErr(err)
	if ok, headers := request.Get(coretypes.MakeKeyword(STRINGS.Intern, "headers")); ok {
		h := coretypes.EnsureObjectIsMap(headers, "headers: %s")
		for iter := h.Iter(); iter.HasNext(); {
			p := iter.Next()
			req.Header.Add(coretypes.EnsureObjectIsString(p.Key, "header name: %s").S, coretypes.EnsureObjectIsString(p.Value, "header value: %s").S)
		}
	}
	if ok, host := request.Get(coretypes.MakeKeyword(STRINGS.Intern, "host")); ok {
		req.Host = coretypes.EnsureObjectIsString(host, "host: %s").S
	}
	return req
}

func remoteHost(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(remoteAddr, "[]")
}

func reqToMap(host coretypes.String, port coretypes.String, req *http.Request) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	body, err := io.ReadAll(req.Body)
	closeErr := req.Body.Close()
	PanicOnErr(err)
	PanicOnErr(closeErr)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "request-method"), coretypes.MakeKeyword(STRINGS.Intern, strings.ToLower(req.Method)))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "body"), coretypes.MakeString(string(body)))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "uri"), coretypes.MakeString(req.URL.Path))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "query-string"), coretypes.MakeString(req.URL.RawQuery))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "server-name"), host)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "server-port"), port)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "remote-addr"), coretypes.MakeString(remoteHost(req.RemoteAddr)))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "protocol"), coretypes.MakeString(req.Proto))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "scheme"), coretypes.MakeKeyword(STRINGS.Intern, "http"))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "host"), coretypes.MakeString(req.Host))
	headers := corecollections.EmptyArrayMap()
	for k, v := range req.Header {
		headers.Add(coretypes.MakeString(strings.ToLower(k)), coretypes.MakeString(strings.Join(v, ",")))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "headers"), headers)
	return res
}

func respToMap(resp *http.Response) coretypes.Map {
	res := corecollections.EmptyArrayMap()
	body, err := io.ReadAll(resp.Body)
	closeErr := resp.Body.Close()
	PanicOnErr(err)
	PanicOnErr(closeErr)
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "body"), coretypes.MakeString(string(body)))
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "status"), coretypes.MakeInt(resp.StatusCode))
	respHeaders := corecollections.EmptyArrayMap()
	for k, v := range resp.Header {
		respHeaders.Add(coretypes.MakeString(k), MakeStringVector(v))
	}
	res.Add(coretypes.MakeKeyword(STRINGS.Intern, "headers"), respHeaders)
	maxNativeInt := int64(int(^uint(0) >> 1))
	minNativeInt := -maxNativeInt - 1
	if resp.ContentLength > maxNativeInt || resp.ContentLength < minNativeInt {
		res.Add(coretypes.MakeKeyword(STRINGS.Intern, "content-length"), coretypes.MakeBigInt(big.NewInt(resp.ContentLength)))
	} else {
		res.Add(coretypes.MakeKeyword(STRINGS.Intern, "content-length"), coretypes.MakeInt(int(resp.ContentLength)))
	}
	return res
}

func validHTTPStatus(status int) bool {
	return status >= 100 && status <= 999
}

func responseStatus(obj coretypes.Object, context string) int {
	status := coretypes.EnsureObjectIsInt(obj, context+": %s").I
	if !validHTTPStatus(status) {
		panic(RT.NewError(context + " must be between 100 and 999"))
	}
	return status
}

func mapToResp(response coretypes.Map, w http.ResponseWriter) {
	status := 0
	if ok, s := response.Get(coretypes.MakeKeyword(STRINGS.Intern, "status")); ok {
		status = responseStatus(s, "HTTP response status")
	}
	body := ""
	if ok, b := response.Get(coretypes.MakeKeyword(STRINGS.Intern, "body")); ok {
		body = coretypes.EnsureObjectIsString(b, "HTTP response body: %s").S
	}
	if ok, headers := response.Get(coretypes.MakeKeyword(STRINGS.Intern, "headers")); ok {
		header := w.Header()
		h := coretypes.EnsureObjectIsMap(headers, "HTTP response headers: %s")
		for iter := h.Iter(); iter.HasNext(); {
			p := iter.Next()
			hname := coretypes.EnsureObjectIsString(p.Key, "HTTP response header name %s").S
			switch pvalue := p.Value.(type) {
			case coretypes.String:
				header.Add(hname, pvalue.S)
			case coretypes.Seqable:
				s := pvalue.Seq()
				for !s.IsEmpty() {
					header.Add(hname, coretypes.EnsureObjectIsString(s.First(), "HTTP response header value: %s").S)
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
	_, err := io.WriteString(w, body)
	PanicOnErr(err)
}

func clientFromRequest(request coretypes.Map) *http.Client {
	if ok, c := request.Get(coretypes.MakeKeyword(STRINGS.Intern, "client")); ok {
		hc, ok := c.(*HTTPClient)
		if !ok {
			panic(RT.NewError(":client must be an HTTP client created by joker.http/client"))
		}
		return hc.client
	}
	return client
}

func sendRequest(request coretypes.Map) coretypes.Map {
	req := mapToReq(request)
	resp, err := clientFromRequest(request).Do(req)
	PanicOnErr(err)
	return respToMap(resp)
}

const maxHTTPMillisecondDuration = int64(1<<63-1) / int64(time.Millisecond)

func nonNegativeHTTPOption(obj coretypes.Object, name string) int {
	v := coretypes.EnsureObjectIsInt(obj, name+": %s").I
	if v < 0 {
		panic(RT.NewError(name + " must be non-negative"))
	}
	return v
}

func httpIdleTimeoutDuration(ms int) time.Duration {
	if int64(ms) > maxHTTPMillisecondDuration {
		panic(RT.NewError(":idle-timeout-ms is too large"))
	}
	return time.Duration(ms) * time.Millisecond
}

func makeClient(args []coretypes.Object) coretypes.Object {
	if len(args) > 1 {
		panic(RT.NewError("client expects zero args or one options map"))
	}
	maxIdle := 100
	maxIdlePerHost := 100
	idleTimeoutMs := 90000
	if len(args) == 1 && !args[0].Equals(NIL) {
		opts := coretypes.EnsureObjectIsMap(args[0], "client options must be a map: %s")
		if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "max-idle-conns")); ok {
			maxIdle = nonNegativeHTTPOption(v, ":max-idle-conns")
		}
		if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "max-idle-conns-per-host")); ok {
			maxIdlePerHost = nonNegativeHTTPOption(v, ":max-idle-conns-per-host")
		}
		if ok, v := opts.Get(coretypes.MakeKeyword(STRINGS.Intern, "idle-timeout-ms")); ok {
			idleTimeoutMs = nonNegativeHTTPOption(v, ":idle-timeout-ms")
		}
	}
	return newHTTPClient(maxIdle, maxIdlePerHost, httpIdleTimeoutDuration(idleTimeoutMs))
}

func closeClient(c coretypes.Object) coretypes.Object {
	hc, ok := c.(*HTTPClient)
	if !ok {
		panic(RT.NewError("close-client requires an HTTP client"))
	}
	hc.transport.CloseIdleConnections()
	return NIL
}

func listenHostPort(addr string) (coretypes.String, coretypes.String) {
	host, port, err := net.SplitHostPort(addr)
	if err == nil {
		return coretypes.MakeString(strings.Trim(host, "[]")), coretypes.MakeString(port)
	}
	return coretypes.MakeString(strings.Trim(addr, "[]")), coretypes.MakeString("")
}

func startServer(addr string, handler coretypes.Callable) coretypes.Object {
	host, port := listenHostPort(addr)
	err := http.ListenAndServe(addr, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if r := recover(); r != nil {
				w.WriteHeader(500)
				if _, err := io.WriteString(w, "Internal server error"); err != nil {
					fmt.Fprintln(os.Stderr, "failed writing internal server error response:", err)
				}
				fmt.Fprintln(os.Stderr, r)
			}
		}()
		response := handler.Call([]coretypes.Object{reqToMap(host, port, req)})
		respMap := coretypes.EnsureObjectIsMap(response, "HTTP response: %s")

		// Check for WebSocket upgrade
		if ok, wsConf := respMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "websocket")); ok {
			handleWebSocket(w, req, coretypes.EnsureObjectIsMap(wsConf, "websocket config: %s"))
			return
		}

		// Check for SSE/streaming response
		if ok, streamFn := respMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "stream")); ok {
			handleStream(w, respMap, coretypes.EnsureObjectIsCallable(streamFn, "stream value must be callable: %s"))
			return
		}

		mapToResp(respMap, w)
	}))
	PanicOnErr(err)
	return NIL
}

func startFileServer(addr string, root string) coretypes.Object {
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
func handleWebSocket(w http.ResponseWriter, req *http.Request, conf coretypes.Map) {
	conn, err := upgrader.Upgrade(w, req, nil)
	if err != nil {
		fmt.Fprintln(os.Stderr, "websocket upgrade error:", err)
		return
	}
	var closeMu sync.Mutex
	closed := false
	closeConn := func() error {
		closeMu.Lock()
		defer closeMu.Unlock()
		if closed {
			return nil
		}
		closed = true
		return conn.Close()
	}
	defer func() {
		if err := closeConn(); err != nil {
			fmt.Fprintln(os.Stderr, "websocket close error:", err)
		}
	}()

	var writeMu sync.Mutex

	// Build send-fn: (fn [msg]) sends a text message
	sendFn := Proc{Name: "ws-send", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		msg := args[0].ToString(false)
		writeMu.Lock()
		err := conn.WriteMessage(ws.TextMessage, []byte(msg))
		writeMu.Unlock()
		if err != nil {
			panic(RT.NewError("websocket send error: " + err.Error()))
		}
		return NIL
	}}

	// Build close-fn: (fn []) closes the connection
	closeFn := Proc{Name: "ws-close", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 0, 0)
		if err := closeConn(); err != nil {
			panic(RT.NewError("websocket close error: " + err.Error()))
		}
		return NIL
	}}

	// Call on-open
	if ok, onOpen := conf.Get(coretypes.MakeKeyword(STRINGS.Intern, "on-open")); ok {
		coretypes.EnsureObjectIsCallable(onOpen, "on-open must be callable: %s").Call([]coretypes.Object{sendFn, closeFn})
	}

	// Read loop
	_, onMsg := conf.Get(coretypes.MakeKeyword(STRINGS.Intern, "on-message"))
	var onMsgFn coretypes.Callable
	if onMsg != nil {
		onMsgFn = coretypes.EnsureObjectIsCallable(onMsg, "on-message must be callable: %s")
	}

	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if onMsgFn != nil {
			onMsgFn.Call([]coretypes.Object{coretypes.MakeString(string(message))})
		}
	}

	// Call on-close
	if ok, onClose := conf.Get(coretypes.MakeKeyword(STRINGS.Intern, "on-close")); ok {
		coretypes.EnsureObjectIsCallable(onClose, "on-close must be callable: %s").Call([]coretypes.Object{})
	}
}

// handleStream writes an SSE/chunked streaming response.
// The stream fn receives a send-event callback: (fn [event-data])
// Response map can include :status and :headers (applied before streaming).
// Default Content-coretypes.Type is text/event-stream.
func handleStream(w http.ResponseWriter, respMap coretypes.Map, streamFn coretypes.Callable) {
	closeInfo := sseCloseInfo("completed", nil)
	if ok, onClose := respMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "on-close")); ok {
		onCloseFn := coretypes.EnsureObjectIsCallable(onClose, "stream on-close must be callable: %s")
		defer func() {
			onCloseFn.Call([]coretypes.Object{closeInfo})
		}()
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		w.WriteHeader(500)
		if _, err := io.WriteString(w, "Streaming not supported"); err != nil {
			fmt.Fprintln(os.Stderr, "streaming unsupported response write error:", err)
		}
		return
	}

	// Apply headers
	header := w.Header()
	header.Set("Content-coretypes.Type", "text/event-stream")
	header.Set("Cache-Control", "no-cache")
	header.Set("Connection", "keep-alive")
	if ok, headers := respMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "headers")); ok {
		h := coretypes.EnsureObjectIsMap(headers, "stream headers: %s")
		for iter := h.Iter(); iter.HasNext(); {
			p := iter.Next()
			hname := coretypes.EnsureObjectIsString(p.Key, "header name: %s").S
			switch pvalue := p.Value.(type) {
			case coretypes.String:
				header.Add(hname, pvalue.S)
			case coretypes.Seqable:
				s := pvalue.Seq()
				for !s.IsEmpty() {
					header.Add(hname, coretypes.EnsureObjectIsString(s.First(), "header value: %s").S)
					s = s.Rest()
				}
			default:
				panic(RT.NewError("stream header value must be a string or a seq of strings"))
			}
		}
	}

	// Apply status
	status := 200
	if ok, s := respMap.Get(coretypes.MakeKeyword(STRINGS.Intern, "status")); ok {
		status = responseStatus(s, "stream status")
	}
	w.WriteHeader(status)
	flusher.Flush()

	writeStream := func(format string, args ...any) {
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			panic(RT.NewError("stream write error: " + err.Error()))
		}
	}

	// Build send-event fn: (fn [data]) or (fn [event data])
	sendEvent := Proc{Name: "sse-send", Fn: func(args []coretypes.Object) coretypes.Object {
		switch len(args) {
		case 1:
			// data-only: sends "data: ...\n\n"
			data := args[0].ToString(false)
			for _, line := range strings.Split(data, "\n") {
				writeStream("data: %s\n", line)
			}
			writeStream("\n")
		case 2:
			// event + data: sends "event: ...\ndata: ...\n\n"
			event := args[0].ToString(false)
			data := args[1].ToString(false)
			writeStream("event: %s\n", event)
			for _, line := range strings.Split(data, "\n") {
				writeStream("data: %s\n", line)
			}
			writeStream("\n")
		default:
			panic(RT.NewError("send-event expects 1 or 2 args: [data] or [event data]"))
		}
		flusher.Flush()
		return NIL
	}}

	defer func() {
		if r := recover(); r != nil {
			if err, ok := r.(error); ok {
				closeInfo = sseCloseInfo("error", err)
			} else {
				closeInfo = sseCloseInfo("error", nil)
			}
			panic(r)
		}
	}()

	// Call the stream function with send-event
	streamFn.Call([]coretypes.Object{sendEvent})
}

func sseCloseInfo(reason string, err error) coretypes.Object {
	m := corecollections.EmptyArrayMap()
	m.Add(coretypes.MakeKeyword(STRINGS.Intern, "reason"), coretypes.MakeKeyword(STRINGS.Intern, reason))
	if err != nil {
		m.Add(coretypes.MakeKeyword(STRINGS.Intern, "error"), RT.NewError(err.Error()))
	}
	return m
}
