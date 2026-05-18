package core

import (
	coretypes "github.com/rcarmo/go-joker/core/types"
	"reflect"
	"sync"
	"time"

	"github.com/rcarmo/go-joker/core/hashutil"
	corert "github.com/rcarmo/go-joker/core/runtime"
)

// concurrency_ext.go — Extended concurrency primitives: alts!, timeout, future, promise, pmap.
//
// These require the GIL-free runtime (goroutine_rt.go).

const maxMillisecondDuration = int64(1<<63-1) / int64(time.Millisecond)

func checkedMillisecondDuration(ms int, context string) time.Duration {
	if ms < 0 {
		panic(RT.NewError(context + " requires a non-negative millisecond value"))
	}
	if int64(ms) > maxMillisecondDuration {
		panic(RT.NewError(context + " millisecond value is too large"))
	}
	return time.Duration(ms) * time.Millisecond
}

// installConcurrencyExt registers alts!, timeout, future, promise, deliver,
// future?, promise?, realized?, pmap, and pcalls.
func installConcurrencyExt() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// timeout — returns a channel that closes after ms milliseconds.
	// (timeout ms) -> Channel
	toVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "timeout"))
	toVr.Value = Proc{Name: "procTimeout", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		delay := checkedMillisecondDuration(coretypes.EnsureArgIsInt(args, 0).I, "timeout")
		ch := MakeChannel(make(chan FutureResult, 1))
		go func() {
			time.Sleep(delay)
			ch.Close()
		}()
		return ch
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "timeout"), toVr)

	// alts! — select-style multi-channel wait.
	// (alts! ports & opts) where ports is a vector of channels (take) or
	// [channel value] pairs (put).
	// Returns [value channel].
	// Options: :default val — return immediately if nothing ready.
	altsVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "alts!"))
	altsVr.Value = Proc{Name: "procAlts", Fn: procAlts}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "alts!"), altsVr)

	// future — runs body in a goroutine, returns a deref-able object.
	// (future body...) is a macro defined in core.joke; the runtime primitive is future-call.
	fcVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "future-call"))
	fcVr.Value = Proc{Name: "procFutureCall", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		f := coretypes.EnsureArgIsCallable(args, 0)
		fut := &Future{runtime: corert.NewFuture[coretypes.Object, coretypes.Error]()}
		go func() {
			registerGoroutineRT()
			defer unregisterGoroutineRT()
			var value coretypes.Object = NIL
			var err coretypes.Error
			defer func() {
				if r := recover(); r != nil {
					switch e := r.(type) {
					case coretypes.Error:
						err = e
					default:
						err = RT.NewError("future panic")
					}
				}
				fut.runtime.Complete(value, err)
			}()
			value = call0(f)
		}()
		return fut
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "future-call"), fcVr)

	// future — macro: (future body...) -> (future-call (fn [] body...))
	installMacro(ns, "future", func(args []coretypes.Object) coretypes.Object {
		// args: &form, &env, body...
		body := args[2:]
		fnForm := collectionConstruction.NewListFrom(append([]coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "fn"), collectionConstruction.NewVectorFrom()}, body...)...)
		return collectionConstruction.NewListFrom(coretypes.MakeSymbol(STRINGS.Intern, "future-call"), fnForm)
	})

	// future? — true if obj is a Future.
	fqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "future?"))
	fqVr.Value = Proc{Name: "procFutureQ", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		_, ok := args[0].(*Future)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "future?"), fqVr)

	// promise — creates a promise that can be delivered once.
	// (promise) -> Promise
	prVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "promise"))
	prVr.Value = Proc{Name: "procPromise", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 0, 0)
		return &Promise{runtime: corert.NewPromise[coretypes.Object]()}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "promise"), prVr)

	// deliver — delivers a value to a promise. Returns the promise.
	// (deliver p val) -> Promise
	dlVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "deliver"))
	dlVr.Value = Proc{Name: "procDeliver", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		p, ok := args[0].(*Promise)
		if !ok {
			panic(RT.NewError("deliver requires a promise"))
		}
		p.runtime.Deliver(args[1])
		return p
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "deliver"), dlVr)

	// promise? — true if obj is a Promise.
	pqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "promise?"))
	pqVr.Value = Proc{Name: "procPromiseQ", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		_, ok := args[0].(*Promise)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "promise?"), pqVr)

	// realized? — true if a Future/Promise/coretypes.Delay has been realized.
	rzVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "realized?"))
	rzVr.Value = Proc{Name: "procRealizedQ", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		if p, ok := args[0].(coretypes.Pending); ok {
			return coretypes.MakeBoolean(p.IsRealized())
		}
		return coretypes.Boolean{B: false}
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "realized?"), rzVr)

	// pmap — parallel map. (pmap f coll)
	// Applies f to each element in parallel goroutines, returns lazy seq of results in order.
	pmapVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "pmap"))
	pmapVr.Value = Proc{Name: "procPmap", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 2, 2)
		f := coretypes.EnsureArgIsCallable(args, 0)
		coll := coretypes.EnsureObjectIsSeqable(args[1], "pmap requires a coretypes.Seqable collection").Seq()
		// Collect all elements first (pmap is not lazy in this impl).
		var elems []coretypes.Object
		for s := coll; !s.IsEmpty(); s = s.Rest() {
			elems = append(elems, s.First())
		}
		if len(elems) == 0 {
			return NIL
		}
		results := make([]coretypes.Object, len(elems))
		done := make(chan int, len(elems))
		panicCh := make(chan interface{}, len(elems))
		for i, elem := range elems {
			go func(idx int, val coretypes.Object) {
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
		return collectionConstruction.NewListFrom(results...)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "pmap"), pmapVr)

	// pcalls — parallel calls. (pcalls & fns)
	// Calls each no-arg fn in parallel, returns list of results.
	pcVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "pcalls"))
	pcVr.Value = Proc{Name: "procPcalls", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) == 0 {
			return NIL
		}
		results := make([]coretypes.Object, len(args))
		done := make(chan int, len(args))
		panicCh := make(chan interface{}, len(args))
		for i, arg := range args {
			f := coretypes.EnsureObjectIsCallable(arg, "pcalls requires callable arguments")
			go func(idx int, fn coretypes.Callable) {
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
		return collectionConstruction.NewListFrom(results...)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "pcalls"), pcVr)
}

// procAlts implements (alts! ports & opts).
func procAlts(args []coretypes.Object) coretypes.Object {
	if len(args) < 1 {
		panic(RT.NewError("alts! requires at least one argument (ports vector)"))
	}
	ports := coretypes.EnsureObjectIsSeqable(args[0], "alts! first arg must be a vector of ports").Seq()

	// Parse options.
	if len(args[1:])%2 != 0 {
		panic(RT.NewError("alts! options must be key/value pairs"))
	}
	var defaultVal coretypes.Object
	hasDefault := false
	for i := 1; i+1 < len(args); i += 2 {
		if k, ok := args[i].(coretypes.Keyword); ok && k.ToString(false) == ":default" {
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
				Chan: reflect.ValueOf(v.raw()),
			})
			infos = append(infos, portInfo{ch: v, isPut: false})
		default:
			// Check if it's a vector-like [channel value] for put.
			if ci, ok := item.(coretypes.CountedIndexed); ok && ci.Count() == 2 {
				ch := EnsureObjectIsChannel(ci.At(0), "alts! put port first element must be a channel")
				if ch.IsClosed() {
					// Clojure-like semantics: put on closed channel returns false immediately.
					return collectionConstruction.NewVectorFrom(coretypes.MakeBoolean(false), ch)
				}
				val := ci.At(1)
				cases = append(cases, reflect.SelectCase{
					Dir:  reflect.SelectSend,
					Chan: reflect.ValueOf(ch.raw()),
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
		return collectionConstruction.NewVectorFrom(defaultVal, coretypes.MakeKeyword(STRINGS.Intern, "default"))
	}

	info := infos[chosen]
	if info.isPut {
		// Put completed.
		return collectionConstruction.NewVectorFrom(coretypes.MakeBoolean(true), info.ch)
	}
	// Take completed.
	if !recvOK {
		// Channel closed.
		return collectionConstruction.NewVectorFrom(NIL, info.ch)
	}
	fr := recv.Interface().(FutureResult)
	if fr.err != nil {
		panic(fr.err)
	}
	return collectionConstruction.NewVectorFrom(fr.value, info.ch)
}

// --- Future type ---

// Future holds a value computed asynchronously.
type Future struct {
	runtime *corert.Future[coretypes.Object, coretypes.Error]
}

func (f *Future) ToString(escape bool) string    { return "#object[Future]" }
func (f *Future) Equals(other interface{}) bool  { return f == other }
func (f *Future) GetInfo() *coretypes.ObjectInfo { return nil }
func (f *Future) GetType() *coretypes.Type       { return TYPE.Fn } // Clojure: futures are IFn
func (f *Future) Hash() uint32 {
	return hashutil.Ptr(uintptr(reflect.ValueOf(f).Pointer()))
}
func (f *Future) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return f }

func (f *Future) Deref() coretypes.Object {
	value, err := f.runtime.Await()
	if err != nil {
		panic(coretypes.Object(err))
	}
	return value
}

func (f *Future) IsRealized() bool {
	return f.runtime.IsRealized()
}

// --- Promise type ---

// Promise holds a value that can be delivered once.
type Promise struct {
	runtime *corert.Promise[coretypes.Object]
}

func (p *Promise) ToString(escape bool) string    { return "#object[Promise]" }
func (p *Promise) Equals(other interface{}) bool  { return p == other }
func (p *Promise) GetInfo() *coretypes.ObjectInfo { return nil }
func (p *Promise) GetType() *coretypes.Type       { return TYPE.Fn }
func (p *Promise) Hash() uint32 {
	return hashutil.Ptr(uintptr(reflect.ValueOf(p).Pointer()))
}
func (p *Promise) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return p }

func (p *Promise) Deref() coretypes.Object {
	return p.runtime.Await()
}

func (p *Promise) IsRealized() bool {
	return p.runtime.IsRealized()
}

func init() {
	installConcurrencyExt()
	installAgentExt()
}

// --- Agent type ---

// Agent holds mutable state that is updated asynchronously via send/send-off.
type Agent struct {
	coretypes.MetaHolder
	mu    sync.Mutex
	value coretypes.Object
	queue chan agentAction
	err   coretypes.Error
}

type agentAction struct {
	fn   coretypes.Callable
	args []coretypes.Object
}

func newAgent(initVal coretypes.Object) *Agent {
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
					if e, ok := r.(coretypes.Error); ok {
						a.err = e
					}
				}
			}()
			args := append([]coretypes.Object{a.value}, action.args...)
			a.value = action.fn.Call(args)
		}()
		a.mu.Unlock()
	}
}

func (a *Agent) ToString(escape bool) string    { return "#object[Agent]" }
func (a *Agent) Equals(other interface{}) bool  { return a == other }
func (a *Agent) GetInfo() *coretypes.ObjectInfo { return nil }
func (a *Agent) GetType() *coretypes.Type       { return TYPE.Fn }
func (a *Agent) Hash() uint32 {
	return hashutil.Ptr(uintptr(reflect.ValueOf(a).Pointer()))
}
func (a *Agent) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return a }

func (a *Agent) Deref() coretypes.Object {
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
	agVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "agent"))
	agVr.Value = Proc{Name: "procAgent", Fn: func(args []coretypes.Object) coretypes.Object {
		CheckArity(args, 1, 1)
		return newAgent(args[0])
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "agent"), agVr)

	// send — dispatches action to agent (returns agent immediately).
	sendVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "send"))
	sendVr.Value = Proc{Name: "procSend", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			panic(RT.NewError("send requires at least 2 args: agent and fn"))
		}
		a, ok := args[0].(*Agent)
		if !ok {
			panic(RT.NewError("send first arg must be an agent"))
		}
		f := coretypes.EnsureObjectIsCallable(args[1], "send second arg must be a fn")
		a.queue <- agentAction{fn: f, args: args[2:]}
		return a
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "send"), sendVr)

	// send-off — same as send for this implementation (no thread pool distinction).
	soVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "send-off"))
	soVr.Value = Proc{Name: "procSendOff", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			panic(RT.NewError("send-off requires at least 2 args: agent and fn"))
		}
		a, ok := args[0].(*Agent)
		if !ok {
			panic(RT.NewError("send-off first arg must be an agent"))
		}
		f := coretypes.EnsureObjectIsCallable(args[1], "send-off second arg must be a fn")
		a.queue <- agentAction{fn: f, args: args[2:]}
		return a
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "send-off"), soVr)

	// await — blocks until all actions dispatched to agents have completed.
	// Simple implementation: sends a sentinel and waits for it to be processed.
	awaitVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "await"))
	awaitVr.Value = Proc{Name: "procAwait", Fn: func(args []coretypes.Object) coretypes.Object {
		for _, arg := range args {
			a, ok := arg.(*Agent)
			if !ok {
				panic(RT.NewError("await requires agent arguments"))
			}
			done := make(chan struct{})
			a.queue <- agentAction{
				fn: Proc{Name: "awaitSentinel", Fn: func(fnArgs []coretypes.Object) coretypes.Object {
					close(done)
					return fnArgs[0] // identity — don't change value
				}},
			}
			<-done
		}
		return NIL
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "await"), awaitVr)

	// agent-error — returns any error that has occurred on the agent.
	aeVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "agent-error"))
	aeVr.Value = Proc{Name: "procAgentError", Fn: func(args []coretypes.Object) coretypes.Object {
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
		if eo, ok := e.(coretypes.Object); ok {
			return eo
		}
		return coretypes.MakeString(e.Error())
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "agent-error"), aeVr)
}
