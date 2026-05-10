package pods

import (
	"bytes"
	"io"
	"testing"
	"time"

	. "github.com/candid82/joker/core"
)

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
