package pods

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	. "github.com/rcarmo/go-joker/core"
)

func TestHelperProcessFakePod(t *testing.T) {
	if os.Getenv("GO_WANT_FAKE_POD") != "1" {
		return
	}
	obj, err := bencodeDecodeReader(os.Stdin)
	if err != nil {
		os.Exit(2)
	}
	msg := objectMapToPodMessage(obj)
	id, _ := msg["id"].(string)
	if msg["op"] == "invoke" {
		os.Stdout.Write(bencodeEncodePlain(map[string]any{
			"id":    id,
			"value": msg["args"],
			"done":  true,
		}))
		os.Exit(0)
	}
	os.Stdout.Write(bencodeEncodePlain(map[string]any{
		"id":     id,
		"format": "json",
		"namespaces": []any{map[string]any{
			"name": "fake.pod",
			"vars": []any{map[string]any{"name": "echo"}},
		}},
		"done": true,
	}))
	os.Exit(0)
}

func TestPodRegistryLifecycleAndIDs(t *testing.T) {
	shutdownAllPods()
	p := newPod("pod-test", "test", "json", io.Discard, nil, nil)
	registerPod(p)
	if lookupPod("pod-test") != p {
		t.Fatal("pod not registered")
	}
	if p.nextID() != "pod-test-1" || p.nextID() != "pod-test-2" {
		t.Fatal("unexpected request id sequence")
	}
	p.shutdownPod()
	if lookupPod("pod-test") != nil {
		t.Fatal("pod not unregistered after shutdown")
	}
}

func TestPodResponseRouting(t *testing.T) {
	shutdownAllPods()
	p := newPod("pod-route", "route", "json", io.Discard, nil, nil)
	ch := p.registerPending("req-1")
	p.routeMessage(podMessage{"id": "req-1", "value": "ok"})
	select {
	case msg := <-ch:
		if msg["value"] != "ok" {
			t.Fatalf("unexpected message: %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for routed message")
	}
	p.routeMessage(podMessage{"id": "req-1", "status": "done"})
	if done, ok := <-ch; !ok || done["status"] != "done" {
		t.Fatalf("expected done message before close: %#v ok=%v", done, ok)
	}
	if _, ok := <-ch; ok {
		t.Fatal("expected pending channel to close on done")
	}
}

func TestStartPodProcessSendsDescribe(t *testing.T) {
	shutdownAllPods()
	t.Setenv("GO_WANT_FAKE_POD", "1")
	p, describe, err := startPodProcess(os.Args[0], []string{"-test.run=TestHelperProcessFakePod"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.shutdownPod()
	if describe["format"] != "json" {
		t.Fatalf("describe format mismatch: %#v", describe)
	}
	if firstPodNamespaceName(describe) != "fake.pod" {
		t.Fatalf("namespace mismatch: %#v", describe)
	}
}

func TestInstallPodDescribeNamespaces(t *testing.T) {
	shutdownAllPods()
	p := newPod("pod-dynamic", "dynamic", "json", io.Discard, nil, nil)
	registerPod(p)
	describe := podMessage{"namespaces": []any{podMessage{"name": "fake.dynamic", "vars": []any{podMessage{"name": "echo", "doc": "Echoes its args."}}}}}
	if err := installPodDescribeNamespaces(p, describe); err != nil {
		t.Fatal(err)
	}
	ns := GLOBAL_ENV.EnsureSymbolIsNamespace(MakeSymbol("fake.dynamic"))
	vr := ns.Resolve("echo")
	if vr == nil {
		t.Fatal("dynamic pod var was not installed")
	}
	if vr.Value.GetType() != TYPE.Proc {
		t.Fatalf("dynamic pod var is not a proc: %T", vr.Value)
	}
}

func TestPodInvokeTimeoutOptionsRejectInvalidValues(t *testing.T) {
	for name, opt := range map[string]Object{
		"non-map":     MakeString("bad"),
		"non-integer": func() Object { m := EmptyArrayMap(); m.Add(MakeKeyword("timeout-ms"), MakeString("bad")); return m }(),
		"zero":        func() Object { m := EmptyArrayMap(); m.Add(MakeKeyword("timeout-ms"), MakeInt(0)); return m }(),
		"negative":    func() Object { m := EmptyArrayMap(); m.Add(MakeKeyword("timeout-ms"), MakeInt(-1)); return m }(),
		"too-large": func() Object {
			m := EmptyArrayMap()
			m.Add(MakeKeyword("timeout-ms"), MakeInt(int(^uint(0)>>1)))
			return m
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("invalid pod invoke timeout option did not panic")
				}
			}()
			_ = podInvokeTimeoutFromOpts(opt, time.Second)
		})
	}
}

func TestPodInvokeTimeoutCleansPending(t *testing.T) {
	shutdownAllPods()
	p := newPod("pod-timeout", "timeout", "json", io.Discard, nil, nil)
	if _, err := p.invokeWithTimeout("fake.pod/hang", []Object{MakeString("hi")}, time.Millisecond); err == nil || err.Error() != "timed out waiting for pod response" {
		t.Fatalf("expected timeout error, got %v", err)
	}
	p.pendingMu.Lock()
	pending := len(p.pending)
	p.pendingMu.Unlock()
	if pending != 0 {
		t.Fatalf("pending requests leaked after timeout: %d", pending)
	}
}

func TestPodInvokeJSON(t *testing.T) {
	shutdownAllPods()
	pr, pw := io.Pipe()
	defer pr.Close()
	defer pw.Close()
	p := newPod("pod-invoke", "invoke", "json", pw, nil, nil)
	go func() {
		for {
			obj, err := bencodeDecodeReader(pr)
			if err != nil {
				return
			}
			msg := objectMapToPodMessage(obj)
			if msg["op"] == "invoke" {
				p.routeMessage(podMessage{"id": msg["id"], "value": msg["args"], "done": true})
				return
			}
		}
	}()
	res, err := p.invoke("fake.pod/echo", []Object{MakeString("hi"), MakeInt(7)})
	if err != nil {
		t.Fatal(err)
	}
	vec := res.(CountedIndexed)
	if vec.Count() != 2 || vec.At(0).ToString(false) != "hi" || vec.At(1).(Int).I != 7 {
		t.Fatalf("invoke result mismatch: %s", res.ToString(false))
	}
}

func TestPodRouterReadsBencodeMessages(t *testing.T) {
	shutdownAllPods()
	msg := EmptyArrayMap()
	msg.Add(MakeString("id"), MakeString("req-1"))
	msg.Add(MakeString("value"), MakeString("ok"))
	msg.Add(MakeString("done"), Boolean{B: true})
	stdout := bytes.NewReader(bencodeEncodeObject(msg))
	p := newPod("pod-router", "router", "json", io.Discard, stdout, nil)
	ch := p.registerPending("req-1")
	p.startRouter()
	select {
	case routed, ok := <-ch:
		if !ok || routed["value"] != "ok" {
			t.Fatalf("unexpected routed message: %#v ok=%v", routed, ok)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for router")
	}
	if _, ok := <-ch; ok {
		t.Fatal("expected channel to close after done")
	}
}
