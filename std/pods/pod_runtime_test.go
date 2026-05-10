package pods

import (
	"bytes"
	"io"
	"os"
	"testing"
	"time"

	. "github.com/candid82/joker/core"
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
