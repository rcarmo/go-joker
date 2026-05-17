package pods

import (
	"errors"
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"

	. "github.com/rcarmo/go-joker/core"
)

type podMessage map[string]any

type Pod struct {
	id       string
	name     string
	format   string
	cmd      *exec.Cmd
	stdin    io.Writer
	stdout   io.Reader
	stderr   io.Reader
	shutdown atomic.Bool
	nextSeq  atomic.Int64

	writeMu sync.Mutex

	pendingMu sync.Mutex
	pending   map[string]chan podMessage
	routerUp  atomic.Bool
}

var podRegistry = struct {
	sync.Mutex
	pods map[string]*Pod
}{pods: make(map[string]*Pod)}

func newPod(id, name, format string, stdin io.Writer, stdout io.Reader, stderr io.Reader) *Pod {
	return &Pod{
		id:      id,
		name:    name,
		format:  format,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		pending: make(map[string]chan podMessage),
	}
}

func registerPod(p *Pod) {
	podRegistry.Lock()
	podRegistry.pods[p.id] = p
	podRegistry.Unlock()
}

func lookupPod(id string) *Pod {
	podRegistry.Lock()
	defer podRegistry.Unlock()
	return podRegistry.pods[id]
}

func unregisterPod(id string) {
	podRegistry.Lock()
	delete(podRegistry.pods, id)
	podRegistry.Unlock()
}

func shutdownAllPods() {
	podRegistry.Lock()
	pods := make([]*Pod, 0, len(podRegistry.pods))
	for _, p := range podRegistry.pods {
		pods = append(pods, p)
	}
	podRegistry.pods = make(map[string]*Pod)
	podRegistry.Unlock()
	for _, p := range pods {
		p.shutdownPod()
	}
}

func (p *Pod) nextID() string {
	return p.id + "-" + strconv.FormatInt(p.nextSeq.Add(1), 10)
}

func (p *Pod) registerPending(id string) chan podMessage {
	ch := make(chan podMessage, 8)
	p.pendingMu.Lock()
	p.pending[id] = ch
	p.pendingMu.Unlock()
	return ch
}

func (p *Pod) unregisterPending(id string) {
	p.pendingMu.Lock()
	if ch := p.pending[id]; ch != nil {
		delete(p.pending, id)
		safeClosePodChannel(ch)
	}
	p.pendingMu.Unlock()
}

func (p *Pod) routeMessage(msg podMessage) {
	id, _ := msg["id"].(string)
	if id == "" {
		return
	}
	p.pendingMu.Lock()
	ch := p.pending[id]
	if isPodDoneMessage(msg) {
		delete(p.pending, id)
	}
	p.pendingMu.Unlock()
	if ch == nil {
		return
	}
	safeSendPodMessage(ch, msg)
	if isPodDoneMessage(msg) {
		safeClosePodChannel(ch)
	}
}

func safeSendPodMessage(ch chan podMessage, msg podMessage) {
	defer func() { _ = recover() }()
	ch <- msg
}

func safeClosePodChannel(ch chan podMessage) {
	defer func() { _ = recover() }()
	close(ch)
}

func isPodDoneMessage(msg podMessage) bool {
	if done, ok := msg["done"].(bool); ok && done {
		return true
	}
	if status, ok := msg["status"].(string); ok && status == "done" {
		return true
	}
	return false
}

func (p *Pod) startRouter() {
	if p.stdout == nil || p.routerUp.Swap(true) {
		return
	}
	go func() {
		for {
			obj, err := bencodeDecodeReader(p.stdout)
			if err != nil {
				p.closePending(err)
				return
			}
			msg := objectMapToPodMessage(obj)
			if out, ok := msg["out"].(string); ok {
				fmt.Print(out)
			}
			if errText, ok := msg["err"].(string); ok {
				fmt.Fprint(os.Stderr, errText)
			}
			p.routeMessage(msg)
		}
	}()
}

func (p *Pod) closePending(_ error) {
	p.pendingMu.Lock()
	defer p.pendingMu.Unlock()
	for id, ch := range p.pending {
		delete(p.pending, id)
		safeClosePodChannel(ch)
	}
}

func (p *Pod) shutdownPod() {
	if p.shutdown.Swap(true) {
		return
	}
	closePodStream("stdin", p.stdin)
	closePodStream("stdout", p.stdout)
	closePodStream("stderr", p.stderr)
	if p.cmd != nil && p.cmd.Process != nil {
		if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			fmt.Fprintf(os.Stderr, "pod process kill error: %v\n", err)
		}
		if _, err := p.cmd.Process.Wait(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			fmt.Fprintf(os.Stderr, "pod process wait error: %v\n", err)
		}
	}
	p.closePending(nil)
	unregisterPod(p.id)
}

func closePodStream(name string, stream any) {
	c, ok := stream.(io.Closer)
	if !ok || c == nil {
		return
	}
	if err := c.Close(); err != nil {
		fmt.Fprintf(os.Stderr, "pod %s close error: %v\n", name, err)
	}
}

func objectMapToPodMessage(obj any) podMessage {
	m := podMessage{}
	jm, ok := obj.(Map)
	if !ok {
		return m
	}
	for it := jm.Iter(); it.HasNext(); {
		p := it.Next()
		m[podMessageKeyString(p.Key)] = podMessageValue(p.Value)
	}
	return m
}

func podMessageKeyString(k Object) string {
	switch v := k.(type) {
	case String:
		return v.S
	case Keyword:
		return v.ToString(false)[1:]
	case Symbol:
		return v.ToString(false)
	default:
		return v.ToString(false)
	}
}

func podMessageValue(v Object) any {
	switch x := v.(type) {
	case String:
		return x.S
	case Keyword, Symbol:
		return x.ToString(false)
	case coretypes.Int:
		return x.I
	case coretypes.Boolean:
		return x.B
	case Nil:
		return nil
	case Map:
		return objectMapToPodMessage(x)
	case Seqable:
		vals := []any{}
		for s := x.Seq(); !s.IsEmpty(); s = s.Rest() {
			vals = append(vals, podMessageValue(s.First()))
		}
		return vals
	default:
		return x.ToString(false)
	}
}
