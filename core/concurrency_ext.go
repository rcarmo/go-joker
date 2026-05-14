package core

import (
	"reflect"
	"sync"
	"time"

	"github.com/rcarmo/go-joker/core/hashutil"
)

// concurrency_ext.go — Extended concurrency primitives: alts!, timeout, future, promise, pmap.
//
// These require the GIL-free runtime (goroutine_rt.go).

// installConcurrencyExt registers alts!, timeout, future, promise, deliver,
// future?, promise?, realized?, pmap, and pcalls.
func installConcurrencyExt() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// timeout — returns a channel that closes after ms milliseconds.
	// (timeout ms) -> Channel
	toVr := ns.Intern(MakeSymbol("timeout"))
	toVr.Value = Proc{Name: "procTimeout", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		ms := EnsureArgIsInt(args, 0)
		ch := MakeChannel(make(chan FutureResult, 1))
		go func() {
			time.Sleep(time.Duration(ms.I) * time.Millisecond)
			ch.Close()
		}()
		return ch
	}}
	referToUser(MakeSymbol("timeout"), toVr)

	// alts! — select-style multi-channel wait.
	// (alts! ports & opts) where ports is a vector of channels (take) or
	// [channel value] pairs (put).
	// Returns [value channel].
	// Options: :default val — return immediately if nothing ready.
	altsVr := ns.Intern(MakeSymbol("alts!"))
	altsVr.Value = Proc{Name: "procAlts", Fn: procAlts}
	referToUser(MakeSymbol("alts!"), altsVr)

	// future — runs body in a goroutine, returns a deref-able object.
	// (future body...) is a macro defined in core.joke; the runtime primitive is future-call.
	fcVr := ns.Intern(MakeSymbol("future-call"))
	fcVr.Value = Proc{Name: "procFutureCall", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		f := EnsureArgIsCallable(args, 0)
		fut := &Future{ch: make(chan struct{})}
		go func() {
			registerGoroutineRT()
			defer unregisterGoroutineRT()
			defer func() {
				if r := recover(); r != nil {
					switch e := r.(type) {
					case Error:
						fut.err = e
					default:
						fut.err = RT.NewError("future panic")
					}
				}
				close(fut.ch)
			}()
			fut.value = call0(f)
		}()
		return fut
	}}
	referToUser(MakeSymbol("future-call"), fcVr)

	// future — macro: (future body...) -> (future-call (fn [] body...))
	installMacro(ns, "future", func(args []Object) Object {
		// args: &form, &env, body...
		body := args[2:]
		fnForm := NewListFrom(append([]Object{MakeSymbol("fn"), collections.VectorFrom()}, body...)...)
		return NewListFrom(MakeSymbol("future-call"), fnForm)
	})

	// future? — true if obj is a Future.
	fqVr := ns.Intern(MakeSymbol("future?"))
	fqVr.Value = Proc{Name: "procFutureQ", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		_, ok := args[0].(*Future)
		return MakeBoolean(ok)
	}}
	referToUser(MakeSymbol("future?"), fqVr)

	// promise — creates a promise that can be delivered once.
	// (promise) -> Promise
	prVr := ns.Intern(MakeSymbol("promise"))
	prVr.Value = Proc{Name: "procPromise", Fn: func(args []Object) Object {
		CheckArity(args, 0, 0)
		return &Promise{ch: make(chan struct{})}
	}}
	referToUser(MakeSymbol("promise"), prVr)

	// deliver — delivers a value to a promise. Returns the promise.
	// (deliver p val) -> Promise
	dlVr := ns.Intern(MakeSymbol("deliver"))
	dlVr.Value = Proc{Name: "procDeliver", Fn: func(args []Object) Object {
		CheckArity(args, 2, 2)
		p, ok := args[0].(*Promise)
		if !ok {
			panic(RT.NewError("deliver requires a promise"))
		}
		p.mu.Lock()
		if !p.delivered {
			p.value = args[1]
			p.delivered = true
			close(p.ch)
		}
		p.mu.Unlock()
		return p
	}}
	referToUser(MakeSymbol("deliver"), dlVr)

	// promise? — true if obj is a Promise.
	pqVr := ns.Intern(MakeSymbol("promise?"))
	pqVr.Value = Proc{Name: "procPromiseQ", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		_, ok := args[0].(*Promise)
		return MakeBoolean(ok)
	}}
	referToUser(MakeSymbol("promise?"), pqVr)

	// realized? — true if a Future/Promise/Delay has been realized.
	rzVr := ns.Intern(MakeSymbol("realized?"))
	rzVr.Value = Proc{Name: "procRealizedQ", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		if p, ok := args[0].(Pending); ok {
			return MakeBoolean(p.IsRealized())
		}
		return Boolean{B: false}
	}}
	referToUser(MakeSymbol("realized?"), rzVr)

	// pmap — parallel map. (pmap f coll)
	// Applies f to each element in parallel goroutines, returns lazy seq of results in order.
	pmapVr := ns.Intern(MakeSymbol("pmap"))
	pmapVr.Value = Proc{Name: "procPmap", Fn: func(args []Object) Object {
		CheckArity(args, 2, 2)
		f := EnsureArgIsCallable(args, 0)
		coll := EnsureObjectIsSeqable(args[1], "pmap requires a Seqable collection").Seq()
		// Collect all elements first (pmap is not lazy in this impl).
		var elems []Object
		for s := coll; !s.IsEmpty(); s = s.Rest() {
			elems = append(elems, s.First())
		}
		if len(elems) == 0 {
			return NIL
		}
		results := make([]Object, len(elems))
		done := make(chan int, len(elems))
		panicCh := make(chan interface{}, len(elems))
		for i, elem := range elems {
			go func(idx int, val Object) {
				registerGoroutineRT()
				defer unregisterGoroutineRT()
				defer func() {
					if r := recover(); r != nil {
						panicCh <- r
					}
					done <- idx
				}()
				results[idx] = call1(f, val)
			}(i, elem)
		}
		for range elems {
			<-done
		}
		select {
		case r := <-panicCh:
			panic(r)
		default:
		}
		return NewListFrom(results...)
	}}
	referToUser(MakeSymbol("pmap"), pmapVr)

	// pcalls — parallel calls. (pcalls & fns)
	// Calls each no-arg fn in parallel, returns list of results.
	pcVr := ns.Intern(MakeSymbol("pcalls"))
	pcVr.Value = Proc{Name: "procPcalls", Fn: func(args []Object) Object {
		if len(args) == 0 {
			return NIL
		}
		results := make([]Object, len(args))
		done := make(chan int, len(args))
		panicCh := make(chan interface{}, len(args))
		for i, arg := range args {
			f := EnsureObjectIsCallable(arg, "pcalls requires callable arguments")
			go func(idx int, fn Callable) {
				registerGoroutineRT()
				defer unregisterGoroutineRT()
				defer func() {
					if r := recover(); r != nil {
						panicCh <- r
					}
					done <- idx
				}()
				results[idx] = call0(fn)
			}(i, f)
		}
		for range args {
			<-done
		}
		select {
		case r := <-panicCh:
			panic(r)
		default:
		}
		return NewListFrom(results...)
	}}
	referToUser(MakeSymbol("pcalls"), pcVr)
}

// procAlts implements (alts! ports & opts).
func procAlts(args []Object) Object {
	if len(args) < 1 {
		panic(RT.NewError("alts! requires at least one argument (ports vector)"))
	}
	ports := EnsureObjectIsSeqable(args[0], "alts! first arg must be a vector of ports").Seq()

	// Parse options.
	var defaultVal Object
	hasDefault := false
	for i := 1; i+1 < len(args); i += 2 {
		if k, ok := args[i].(Keyword); ok && k.ToString(false) == ":default" {
			defaultVal = args[i+1]
			hasDefault = true
		}
	}

	// Build reflect.Select cases.
	type portInfo struct {
		ch    *Channel
		isPut bool
	}
	var cases []reflect.SelectCase
	var infos []portInfo

	for s := ports; !s.IsEmpty(); s = s.Rest() {
		item := s.First()
		switch v := item.(type) {
		case *Channel:
			// Take operation.
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(v.ch),
			})
			infos = append(infos, portInfo{ch: v, isPut: false})
		default:
			// Check if it's a vector-like [channel value] for put.
			if ci, ok := item.(CountedIndexed); ok && ci.Count() == 2 {
				ch := EnsureObjectIsChannel(ci.At(0), "alts! put port first element must be a channel")
				if ch.IsClosed() {
					// Clojure-like semantics: put on closed channel returns false immediately.
					return collections.VectorFrom(MakeBoolean(false), ch)
				}
				val := ci.At(1)
				cases = append(cases, reflect.SelectCase{
					Dir:  reflect.SelectSend,
					Chan: reflect.ValueOf(ch.ch),
					Send: reflect.ValueOf(MakeFutureResult(val, nil)),
				})
				infos = append(infos, portInfo{ch: ch, isPut: true})
			} else {
				panic(RT.NewError("alts! port must be a channel or [channel value] vector"))
			}
		}
	}

	if len(cases) == 0 {
		panic(RT.NewError("alts! requires at least one port"))
	}

	// Add default case if :default option provided.
	if hasDefault {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectDefault})
	}

	// Select.
	chosen, recv, recvOK := reflect.Select(cases)

	// Default case.
	if hasDefault && chosen == len(cases)-1 {
		return collections.VectorFrom(defaultVal, MakeKeyword("default"))
	}

	info := infos[chosen]
	if info.isPut {
		// Put completed.
		return collections.VectorFrom(MakeBoolean(true), info.ch)
	}
	// Take completed.
	if !recvOK {
		// Channel closed.
		return collections.VectorFrom(NIL, info.ch)
	}
	fr := recv.Interface().(FutureResult)
	if fr.err != nil {
		panic(fr.err)
	}
	return collections.VectorFrom(fr.value, info.ch)
}

// --- Future type ---

// Future holds a value computed asynchronously.
type Future struct {
	value Object
	err   Error
	ch    chan struct{} // closed when done
}

func (f *Future) ToString(escape bool) string   { return "#object[Future]" }
func (f *Future) Equals(other interface{}) bool { return f == other }
func (f *Future) GetInfo() *ObjectInfo          { return nil }
func (f *Future) GetType() *Type                { return TYPE.Fn } // Clojure: futures are IFn
func (f *Future) Hash() uint32 {
	return hashutil.Ptr(uintptr(reflect.ValueOf(f).Pointer()))
}
func (f *Future) WithInfo(info *ObjectInfo) Object { return f }

func (f *Future) Deref() Object {
	<-f.ch // Block until done.
	if f.err != nil {
		panic(f.err)
	}
	return f.value
}

func (f *Future) IsRealized() bool {
	select {
	case <-f.ch:
		return true
	default:
		return false
	}
}

// --- Promise type ---

// Promise holds a value that can be delivered once.
type Promise struct {
	mu        sync.Mutex
	value     Object
	delivered bool
	ch        chan struct{} // closed when delivered
}

func (p *Promise) ToString(escape bool) string   { return "#object[Promise]" }
func (p *Promise) Equals(other interface{}) bool { return p == other }
func (p *Promise) GetInfo() *ObjectInfo          { return nil }
func (p *Promise) GetType() *Type                { return TYPE.Fn }
func (p *Promise) Hash() uint32 {
	return hashutil.Ptr(uintptr(reflect.ValueOf(p).Pointer()))
}
func (p *Promise) WithInfo(info *ObjectInfo) Object { return p }

func (p *Promise) Deref() Object {
	<-p.ch // Block until delivered.
	return p.value
}

func (p *Promise) IsRealized() bool {
	select {
	case <-p.ch:
		return true
	default:
		return false
	}
}

func init() {
	installConcurrencyExt()
	installAgentExt()
}

// --- Agent type ---

// Agent holds mutable state that is updated asynchronously via send/send-off.
type Agent struct {
	MetaHolder
	mu    sync.Mutex
	value Object
	queue chan agentAction
	err   Error
}

type agentAction struct {
	fn   Callable
	args []Object
}

func newAgent(initVal Object) *Agent {
	a := &Agent{
		value: initVal,
		queue: make(chan agentAction, 256),
	}
	go a.processLoop()
	return a
}

func (a *Agent) processLoop() {
	registerGoroutineRT()
	defer unregisterGoroutineRT()
	for action := range a.queue {
		a.mu.Lock()
		func() {
			defer func() {
				if r := recover(); r != nil {
					if e, ok := r.(Error); ok {
						a.err = e
					}
				}
			}()
			args := append([]Object{a.value}, action.args...)
			a.value = action.fn.Call(args)
		}()
		a.mu.Unlock()
	}
}

func (a *Agent) ToString(escape bool) string   { return "#object[Agent]" }
func (a *Agent) Equals(other interface{}) bool { return a == other }
func (a *Agent) GetInfo() *ObjectInfo          { return nil }
func (a *Agent) GetType() *Type                { return TYPE.Fn }
func (a *Agent) Hash() uint32 {
	return hashutil.Ptr(uintptr(reflect.ValueOf(a).Pointer()))
}
func (a *Agent) WithInfo(info *ObjectInfo) Object { return a }

func (a *Agent) Deref() Object {
	a.mu.Lock()
	v := a.value
	a.mu.Unlock()
	return v
}

func installAgentExt() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// agent — creates a new agent with initial value.
	agVr := ns.Intern(MakeSymbol("agent"))
	agVr.Value = Proc{Name: "procAgent", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		return newAgent(args[0])
	}}
	referToUser(MakeSymbol("agent"), agVr)

	// send — dispatches action to agent (returns agent immediately).
	sendVr := ns.Intern(MakeSymbol("send"))
	sendVr.Value = Proc{Name: "procSend", Fn: func(args []Object) Object {
		if len(args) < 2 {
			panic(RT.NewError("send requires at least 2 args: agent and fn"))
		}
		a, ok := args[0].(*Agent)
		if !ok {
			panic(RT.NewError("send first arg must be an agent"))
		}
		f := EnsureObjectIsCallable(args[1], "send second arg must be a fn")
		a.queue <- agentAction{fn: f, args: args[2:]}
		return a
	}}
	referToUser(MakeSymbol("send"), sendVr)

	// send-off — same as send for this implementation (no thread pool distinction).
	soVr := ns.Intern(MakeSymbol("send-off"))
	soVr.Value = Proc{Name: "procSendOff", Fn: func(args []Object) Object {
		if len(args) < 2 {
			panic(RT.NewError("send-off requires at least 2 args: agent and fn"))
		}
		a, ok := args[0].(*Agent)
		if !ok {
			panic(RT.NewError("send-off first arg must be an agent"))
		}
		f := EnsureObjectIsCallable(args[1], "send-off second arg must be a fn")
		a.queue <- agentAction{fn: f, args: args[2:]}
		return a
	}}
	referToUser(MakeSymbol("send-off"), soVr)

	// await — blocks until all actions dispatched to agents have completed.
	// Simple implementation: sends a sentinel and waits for it to be processed.
	awaitVr := ns.Intern(MakeSymbol("await"))
	awaitVr.Value = Proc{Name: "procAwait", Fn: func(args []Object) Object {
		for _, arg := range args {
			a, ok := arg.(*Agent)
			if !ok {
				panic(RT.NewError("await requires agent arguments"))
			}
			done := make(chan struct{})
			a.queue <- agentAction{
				fn: Proc{Name: "awaitSentinel", Fn: func(fnArgs []Object) Object {
					close(done)
					return fnArgs[0] // identity — don't change value
				}},
			}
			<-done
		}
		return NIL
	}}
	referToUser(MakeSymbol("await"), awaitVr)

	// agent-error — returns any error that has occurred on the agent.
	aeVr := ns.Intern(MakeSymbol("agent-error"))
	aeVr.Value = Proc{Name: "procAgentError", Fn: func(args []Object) Object {
		CheckArity(args, 1, 1)
		a, ok := args[0].(*Agent)
		if !ok {
			panic(RT.NewError("agent-error requires an agent"))
		}
		a.mu.Lock()
		e := a.err
		a.mu.Unlock()
		if e == nil {
			return NIL
		}
		if eo, ok := e.(Object); ok {
			return eo
		}
		return MakeString(e.Error())
	}}
	referToUser(MakeSymbol("agent-error"), aeVr)
}
