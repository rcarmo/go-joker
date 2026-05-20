package core

import (
	"reflect"
	"time"

	coretypes "github.com/rcarmo/go-joker/core/types"
	corecollections "github.com/rcarmo/go-joker/core/types/collections"

	corert "github.com/rcarmo/go-joker/core/runtime"
)

// concurrency_ext.go — Extended concurrency primitives: alts!, timeout, future, promise, pmap.
//
// These require the GIL-free runtime (goroutine_rt.go).

func checkedMillisecondDuration(ms int, context string) time.Duration {
	return corert.CheckedMillisecondDuration(ms, context, func(msg string) any { return coretypes.RuntimeError(msg) })
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
		runtimeCheckArity(args, 1, 1)
		delay := checkedMillisecondDuration(coretypes.EnsureArgIsInt(args, 0).I, "timeout")
		ch := corert.NewObjectChannel(make(chan corert.FutureResult, 1))
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
		runtimeCheckArity(args, 1, 1)
		f := coretypes.EnsureArgIsCallable(args, 0)
		fut := corert.NewObjectFuture()
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
						err = coretypes.RuntimeError("future panic").(coretypes.Error)
					}
				}
				fut.Complete(value, err)
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
		fnForm := corecollections.NewListFrom(append([]coretypes.Object{coretypes.MakeSymbol(STRINGS.Intern, "fn"), corecollections.NewVectorFrom()}, body...)...)
		return corecollections.NewListFrom(coretypes.MakeSymbol(STRINGS.Intern, "future-call"), fnForm)
	})

	// future? — true if obj is a Future.
	fqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "future?"))
	fqVr.Value = Proc{Name: "procFutureQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		_, ok := args[0].(*corert.ObjectFuture)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "future?"), fqVr)

	// promise — creates a promise that can be delivered once.
	// (promise) -> Promise
	prVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "promise"))
	prVr.Value = Proc{Name: "procPromise", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 0, 0)
		return corert.NewObjectPromise()
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "promise"), prVr)

	// deliver — delivers a value to a promise. Returns the promise.
	// (deliver p val) -> Promise
	dlVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "deliver"))
	dlVr.Value = Proc{Name: "procDeliver", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 2, 2)
		p, ok := args[0].(*corert.ObjectPromise)
		if !ok {
			panic(coretypes.RuntimeError("deliver requires a promise"))
		}
		p.Deliver(args[1])
		return p
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "deliver"), dlVr)

	// promise? — true if obj is a Promise.
	pqVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "promise?"))
	pqVr.Value = Proc{Name: "procPromiseQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		_, ok := args[0].(*corert.ObjectPromise)
		return coretypes.MakeBoolean(ok)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "promise?"), pqVr)

	// realized? — true if a Future/Promise/coretypes.Delay has been realized.
	rzVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "realized?"))
	rzVr.Value = Proc{Name: "procRealizedQ", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
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
		runtimeCheckArity(args, 2, 2)
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
		if r, panicked := corert.RunParallel(len(elems), func() { registerGoroutineRT() }, unregisterGoroutineRT, func(i int) {
			results[i] = call1(f, elems[i])
		}); panicked {
			panic(r)
		}
		return corecollections.NewListFrom(results...)
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
		fns := make([]coretypes.Callable, len(args))
		for i, arg := range args {
			fns[i] = coretypes.EnsureObjectIsCallable(arg, "pcalls requires callable arguments")
		}
		if r, panicked := corert.RunParallel(len(args), func() { registerGoroutineRT() }, unregisterGoroutineRT, func(i int) {
			results[i] = call0(fns[i])
		}); panicked {
			panic(r)
		}
		return corecollections.NewListFrom(results...)
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "pcalls"), pcVr)
}

// procAlts implements (alts! ports & opts).
func procAlts(args []coretypes.Object) coretypes.Object {
	if len(args) < 1 {
		panic(coretypes.RuntimeError("alts! requires at least one argument (ports vector)"))
	}
	ports := coretypes.EnsureObjectIsSeqable(args[0], "alts! first arg must be a vector of ports").Seq()

	// Parse options.
	if len(args[1:])%2 != 0 {
		panic(coretypes.RuntimeError("alts! options must be key/value pairs"))
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
		ch    *corert.ObjectChannel
		isPut bool
	}
	var cases []reflect.SelectCase
	var infos []portInfo

	for s := ports; !s.IsEmpty(); s = s.Rest() {
		item := s.First()
		switch v := item.(type) {
		case *corert.ObjectChannel:
			// Take operation.
			cases = append(cases, reflect.SelectCase{
				Dir:  reflect.SelectRecv,
				Chan: reflect.ValueOf(v.Raw()),
			})
			infos = append(infos, portInfo{ch: v, isPut: false})
		default:
			// Check if it's a vector-like [channel value] for put.
			if ci, ok := item.(coretypes.CountedIndexed); ok && ci.Count() == 2 {
				ch := EnsureObjectIsChannel(ci.At(0), "alts! put port first element must be a channel")
				if ch.IsClosed() {
					// Clojure-like semantics: put on closed channel returns false immediately.
					return corecollections.NewVectorFrom(coretypes.MakeBoolean(false), ch)
				}
				val := ci.At(1)
				cases = append(cases, reflect.SelectCase{
					Dir:  reflect.SelectSend,
					Chan: reflect.ValueOf(ch.Raw()),
					Send: reflect.ValueOf(corert.NewFutureResult(val, nil)),
				})
				infos = append(infos, portInfo{ch: ch, isPut: true})
			} else {
				panic(coretypes.RuntimeError("alts! port must be a channel or [channel value] vector"))
			}
		}
	}

	if len(cases) == 0 {
		panic(coretypes.RuntimeError("alts! requires at least one port"))
	}

	// Add default case if :default option provided.
	if hasDefault {
		cases = append(cases, reflect.SelectCase{Dir: reflect.SelectDefault})
	}

	// Select.
	chosen, recv, recvOK := reflect.Select(cases)

	// Default case.
	if hasDefault && chosen == len(cases)-1 {
		return corecollections.NewVectorFrom(defaultVal, coretypes.MakeKeyword(STRINGS.Intern, "default"))
	}

	info := infos[chosen]
	if info.isPut {
		// Put completed.
		return corecollections.NewVectorFrom(coretypes.MakeBoolean(true), info.ch)
	}
	// Take completed.
	if !recvOK {
		// Channel closed.
		return corecollections.NewVectorFrom(NIL, info.ch)
	}
	fr := recv.Interface().(corert.FutureResult)
	if fr.Err != nil {
		panic(fr.Err)
	}
	return corecollections.NewVectorFrom(fr.Value, info.ch)
}

func init() {
	corert.AgentRegisterGoroutine = func() { registerGoroutineRT() }
	corert.AgentUnregisterGoroutine = unregisterGoroutineRT
	installConcurrencyExt()
	installAgentExt()
}

func installAgentExt() {
	ns := GLOBAL_ENV.CoreNamespace
	if ns == nil {
		return
	}

	// agent — creates a new agent with initial value.
	agVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "agent"))
	agVr.Value = Proc{Name: "procAgent", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		return corert.NewAgent(args[0])
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "agent"), agVr)

	// send — dispatches action to agent (returns agent immediately).
	sendVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "send"))
	sendVr.Value = Proc{Name: "procSend", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			panic(coretypes.RuntimeError("send requires at least 2 args: agent and fn"))
		}
		a, ok := args[0].(*corert.Agent)
		if !ok {
			panic(coretypes.RuntimeError("send first arg must be an agent"))
		}
		f := coretypes.EnsureObjectIsCallable(args[1], "send second arg must be a fn")
		a.Send(f, args[2:])
		return a
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "send"), sendVr)

	// send-off — same as send for this implementation (no thread pool distinction).
	soVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "send-off"))
	soVr.Value = Proc{Name: "procSendOff", Fn: func(args []coretypes.Object) coretypes.Object {
		if len(args) < 2 {
			panic(coretypes.RuntimeError("send-off requires at least 2 args: agent and fn"))
		}
		a, ok := args[0].(*corert.Agent)
		if !ok {
			panic(coretypes.RuntimeError("send-off first arg must be an agent"))
		}
		f := coretypes.EnsureObjectIsCallable(args[1], "send-off second arg must be a fn")
		a.Send(f, args[2:])
		return a
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "send-off"), soVr)

	// await — blocks until all actions dispatched to agents have completed.
	// Simple implementation: sends a sentinel and waits for it to be processed.
	awaitVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "await"))
	awaitVr.Value = Proc{Name: "procAwait", Fn: func(args []coretypes.Object) coretypes.Object {
		for _, arg := range args {
			a, ok := arg.(*corert.Agent)
			if !ok {
				panic(coretypes.RuntimeError("await requires agent arguments"))
			}
			a.Await()
		}
		return NIL
	}}
	referToUser(coretypes.MakeSymbol(STRINGS.Intern, "await"), awaitVr)

	// agent-error — returns any error that has occurred on the agent.
	aeVr := ns.Intern(coretypes.MakeSymbol(STRINGS.Intern, "agent-error"))
	aeVr.Value = Proc{Name: "procAgentError", Fn: func(args []coretypes.Object) coretypes.Object {
		runtimeCheckArity(args, 1, 1)
		a, ok := args[0].(*corert.Agent)
		if !ok {
			panic(coretypes.RuntimeError("agent-error requires an agent"))
		}
		e := a.Error()
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
