package pods

import (
	"fmt"
	"time"

	. "github.com/rcarmo/go-joker/core"
)

const podInvokeTimeout = 30 * time.Second

const maxPodInvokeMillisecondDuration = int64(1<<63-1) / int64(time.Millisecond)

func podInvokeTimeoutFromOpts(opts Object, fallback time.Duration) time.Duration {
	if opts == nil || opts.Equals(NIL) {
		return fallback
	}
	m, ok := opts.(Map)
	if !ok {
		panic(RT.NewError("pods/invoke: opts must be a map"))
	}
	if found, v := m.Get(MakeKeyword("timeout-ms")); found {
		i, ok := v.(Int)
		if !ok {
			panic(RT.NewError("pods/invoke: :timeout-ms must be an integer"))
		}
		if i.I <= 0 {
			panic(RT.NewError("pods/invoke: :timeout-ms must be positive"))
		}
		if int64(i.I) > maxPodInvokeMillisecondDuration {
			panic(RT.NewError("pods/invoke: :timeout-ms is too large"))
		}
		return time.Duration(i.I) * time.Millisecond
	}
	return fallback
}

func invokePod(args []Object) Object {
	if len(args) < 3 || len(args) > 4 {
		panic(RT.NewError("pods/invoke expects pod-id, var symbol, args vector, and optional opts"))
	}
	podID := EnsureArgIsString(args, 0).S
	p := lookupPod(podID)
	if p == nil {
		panic(RT.NewError(fmt.Sprintf("pods/invoke: no pod with id %q", podID)))
	}
	varSym := EnsureArgIsSymbol(args, 1).ToString(false)
	callArgs := []Object{}
	seqable, ok := args[2].(Seqable)
	if !ok {
		panic(RT.NewError("pods/invoke: args must be sequential"))
	}
	for s := seqable.Seq(); !s.IsEmpty(); s = s.Rest() {
		callArgs = append(callArgs, s.First())
	}
	timeout := podInvokeTimeout
	if len(args) == 4 {
		timeout = podInvokeTimeoutFromOpts(args[3], timeout)
	}
	res, err := p.invokeWithTimeout(varSym, callArgs, timeout)
	if err != nil {
		panic(RT.NewError("pods/invoke: " + err.Error()))
	}
	return res
}

func (p *Pod) invoke(varName string, args []Object) (Object, error) {
	return p.invokeWithTimeout(varName, args, podInvokeTimeout)
}

func (p *Pod) invokeWithTimeout(varName string, args []Object, timeout time.Duration) (Object, error) {
	encoded, err := p.encodeArgs(args)
	if err != nil {
		return NIL, fmt.Errorf("encode args: %w", err)
	}
	id := p.nextID()
	ch := p.registerPending(id)
	if err := p.send(podMessage{"op": "invoke", "id": id, "var": varName, "args": encoded}); err != nil {
		p.unregisterPending(id)
		return NIL, err
	}
	var timer <-chan time.Time
	if timeout > 0 {
		timer = time.After(timeout)
	}
	var result Object = NIL
	for {
		select {
		case <-timer:
			p.unregisterPending(id)
			return NIL, fmt.Errorf("timed out waiting for pod response")
		case msg, ok := <-ch:
			if !ok {
				return result, nil
			}
			if exMsg, ok := msg["ex-message"].(string); ok {
				p.unregisterPending(id)
				return NIL, fmt.Errorf("%s", exMsg)
			}
			if valStr, ok := msg["value"].(string); ok {
				result, err = p.decodePayload(valStr)
				if err != nil {
					p.unregisterPending(id)
					return NIL, fmt.Errorf("decode value: %w", err)
				}
			}
		}
	}
}
