package core

import (
	"fmt"
	coretypes "github.com/rcarmo/go-joker/core/types"
	"sync"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
)

// core_async_ext.go — clojure.core.async compatibility namespace.
//
// Joker's core already provides channels, go, alts!, timeout and blocking
// <!/>! operations. This file exposes a Clojure-shaped clojure.core.async
// namespace plus the most commonly used higher-level coordination helpers.

func init() { installCoreAsyncNamespace() }

func installCoreAsyncNamespace() {
	if GLOBAL_ENV == nil || GLOBAL_ENV.CoreNamespace == nil {
		return
	}
	ns := GLOBAL_ENV.EnsureSymbolIsLib(MakeSymbol("clojure.core.async"))
	ns.meta = MakeMeta(nil, "Clojure core.async-compatible channel helpers backed by Go goroutines.", "1.0")
	core := GLOBAL_ENV.CoreNamespace
	for _, name := range []string{"chan", "<!", ">!", "close!", "alts!", "timeout", "go"} {
		if vr := core.Resolve(name); vr != nil {
			ns.Refer(MakeSymbol(name), vr)
		}
	}
	if vr := core.Resolve("<!"); vr != nil {
		ns.Refer(MakeSymbol("<!!"), vr)
	}
	if vr := core.Resolve(">!"); vr != nil {
		ns.Refer(MakeSymbol(">!!"), vr)
	}
	installAsyncMacro(ns, "go-loop", "Like core.async/go with an initial loop/recur binding vector.", macroCoreAsyncGoLoop)
	installAsyncMacro(ns, "thread", "Runs body asynchronously on a native goroutine and returns a future.", macroCoreAsyncThread)
	installAsyncMacro(ns, "thread-call", "Runs a zero-argument function asynchronously and returns a future.", macroCoreAsyncThreadCall)

	installAsyncProc(ns, "buffer", "Returns a fixed-size channel buffer descriptor.", procAsyncBuffer)
	installAsyncProc(ns, "dropping-buffer", "Returns a dropping channel buffer descriptor.", procAsyncBuffer)
	installAsyncProc(ns, "sliding-buffer", "Returns a sliding channel buffer descriptor.", procAsyncBuffer)
	installAsyncProc(ns, "promise-chan", "Returns a channel that accepts exactly one value then closes.", procAsyncPromiseChan)
	installAsyncProc(ns, "to-chan", "Copies a collection onto a new channel and closes it.", procAsyncToChan)
	installAsyncProc(ns, "to-chan!", "Alias for to-chan.", procAsyncToChan)
	installAsyncProc(ns, "onto-chan", "Copies a collection onto a channel, optionally closing it.", procAsyncOntoChan)
	installAsyncProc(ns, "onto-chan!", "Alias for onto-chan.", procAsyncOntoChan)
	installAsyncProc(ns, "put!", "Asynchronously puts a value on a channel and optionally invokes a callback.", procAsyncPutBang)
	installAsyncProc(ns, "take!", "Asynchronously takes a value from a channel and invokes a callback.", procAsyncTakeBang)
	installAsyncProc(ns, "pipe", "Pipes values from one channel to another.", procAsyncPipe)
	installAsyncProc(ns, "merge", "Merges multiple input channels onto one output channel.", procAsyncMerge)
	installAsyncProc(ns, "split", "Splits an input channel into true/false output channels by predicate.", procAsyncSplit)
	installAsyncProc(ns, "map<", "Maps a function over values taken from a channel.", procAsyncMapFrom)
	installAsyncProc(ns, "filter<", "Filters values taken from a channel by predicate.", procAsyncFilterFrom)
	installAsyncProc(ns, "map>", "Maps values before putting them on a channel.", procAsyncMapTo)
	installAsyncProc(ns, "filter>", "Filters values before putting them on a channel.", procAsyncFilterTo)
	installAsyncProc(ns, "reduce", "Reduces values from a channel and returns a result channel.", procAsyncReduce)
	installAsyncProc(ns, "into", "Collects values from a channel into a collection.", procAsyncInto)
	installAsyncProc(ns, "mult", "Creates a multicast source from a channel.", procAsyncMult)
	installAsyncProc(ns, "tap", "Adds a tap channel to a mult.", procAsyncTap)
	installAsyncProc(ns, "untap", "Removes a tap channel from a mult.", procAsyncUntap)
	installAsyncProc(ns, "untap-all", "Removes all tap channels from a mult.", procAsyncUntapAll)
	installAsyncProc(ns, "pub", "Creates a topic publication from a channel.", procAsyncPub)
	installAsyncProc(ns, "sub", "Subscribes a channel to a publication topic.", procAsyncSub)
	installAsyncProc(ns, "unsub", "Unsubscribes a channel from a publication topic.", procAsyncUnsub)
	installAsyncProc(ns, "unsub-all", "Unsubscribes channels from publication topics.", procAsyncUnsubAll)
}

func installAsyncProc(ns *Namespace, name, doc string, fn ProcFn) {
	ns.InternVar(name, Proc{Name: "procCoreAsync" + name, Fn: fn}, MakeMeta(nil, doc, "1.0"))
}

func installAsyncMacro(ns *Namespace, name, doc string, fn func([]Object) Object) {
	vr := ns.InternVar(name, Proc{Name: "macro" + name, Fn: fn}, MakeMeta(nil, doc, "1.0"))
	vr.isMacro = true
}

func macroCoreAsyncGoLoop(args []Object) Object {
	if len(args) < 3 {
		panic(RT.NewError("go-loop requires bindings and body"))
	}
	return listObjs(MakeSymbol("go"), collectionConstruction.NewListFrom(append([]Object{MakeSymbol("loop"), args[2]}, args[3:]...)...))
}
func macroCoreAsyncThread(args []Object) Object {
	if len(args) < 2 {
		panic(RT.NewError("thread requires body"))
	}
	return listObjs(MakeSymbol("future"), doObj(args[2:]...))
}
func macroCoreAsyncThreadCall(args []Object) Object {
	if len(args) != 3 {
		panic(RT.NewError("thread-call requires one fn"))
	}
	return listObjs(MakeSymbol("future-call"), args[2])
}

func asyncBufferSize(o Object) int {
	if o == nil || o.Equals(NIL) {
		return 0
	}
	switch v := o.(type) {
	case coretypes.Int:
		return v.I
	default:
		panic(RT.NewError("buffer size must be an integer"))
	}
}
func procAsyncBuffer(args []Object) Object { CheckArity(args, 1, 1); return EnsureArgIsInt(args, 0) }
func procAsyncPromiseChan(args []Object) Object {
	CheckArity(args, 0, 0)
	return MakeChannel(make(chan FutureResult, 1))
}

func channelFromArg(args []Object, i int) *Channel {
	return EnsureObjectIsChannel(args[i], fmt.Sprintf("arg %d must be a channel", i))
}
func asyncSend(ch *Channel, v Object) bool {
	if v == nil || v.Equals(NIL) {
		panic(RT.NewError("Can't put nil on channel"))
	}
	return ch.Send(v)
}
func asyncRecv(ch *Channel) Object {
	v, _, err := ch.Receive(nil)
	if err != nil {
		panic(RT.NewError(err.Error()))
	}
	return v
}

func procAsyncPutBang(args []Object) Object {
	if len(args) != 2 && len(args) != 3 {
		panic(RT.NewError("put! requires channel, value, optional callback"))
	}
	ch := channelFromArg(args, 0)
	v := args[1]
	var cb Callable
	if len(args) == 3 {
		cb = EnsureArgIsCallable(args, 2)
	}
	go func() {
		registerGoroutineRT()
		ok := asyncSend(ch, v)
		if cb != nil {
			call1(cb, coretypes.MakeBoolean(ok))
		}
	}()
	return coretypes.MakeBoolean(!ch.IsClosed())
}

func procAsyncTakeBang(args []Object) Object {
	if len(args) != 2 && len(args) != 3 {
		panic(RT.NewError("take! requires channel, callback, optional on-caller?"))
	}
	ch := channelFromArg(args, 0)
	cb := EnsureArgIsCallable(args, 1)
	go func() { registerGoroutineRT(); call1(cb, asyncRecv(ch)) }()
	return NIL
}

func procAsyncToChan(args []Object) Object {
	if len(args) < 1 || len(args) > 2 {
		panic(RT.NewError("to-chan requires coll and optional close?"))
	}
	ch := MakeChannel(make(chan FutureResult, 0))
	closeOut := true
	if len(args) == 2 {
		closeOut = ToBool(args[1])
	}
	seq := EnsureObjectIsSeqable(args[0], "to-chan requires seqable").Seq()
	go func() {
		registerGoroutineRT()
		for !seq.IsEmpty() {
			asyncSend(ch, seq.First())
			seq = seq.Rest()
		}
		if closeOut {
			ch.Close()
		}
	}()
	return ch
}

func procAsyncOntoChan(args []Object) Object {
	if len(args) < 2 || len(args) > 3 {
		panic(RT.NewError("onto-chan requires channel, coll, optional close?"))
	}
	ch := channelFromArg(args, 0)
	seq := EnsureObjectIsSeqable(args[1], "onto-chan requires seqable").Seq()
	closeOut := true
	if len(args) == 3 {
		closeOut = ToBool(args[2])
	}
	go func() {
		registerGoroutineRT()
		for !seq.IsEmpty() {
			asyncSend(ch, seq.First())
			seq = seq.Rest()
		}
		if closeOut {
			ch.Close()
		}
	}()
	return ch
}

func procAsyncPipe(args []Object) Object {
	if len(args) < 2 || len(args) > 3 {
		panic(RT.NewError("pipe requires from, to, optional close?"))
	}
	from, to := channelFromArg(args, 0), channelFromArg(args, 1)
	closeOut := true
	if len(args) == 3 {
		closeOut = ToBool(args[2])
	}
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(from)
			if v.Equals(NIL) {
				if closeOut {
					to.Close()
				}
				return
			}
			asyncSend(to, v)
		}
	}()
	return to
}

func procAsyncMerge(args []Object) Object {
	if len(args) < 1 || len(args) > 2 {
		panic(RT.NewError("merge requires channels and optional buffer"))
	}
	chsSeq := EnsureObjectIsSeqable(args[0], "merge requires seqable channels").Seq()
	out := MakeChannel(make(chan FutureResult, 0))
	var wg sync.WaitGroup
	for !chsSeq.IsEmpty() {
		ch := EnsureObjectIsChannel(chsSeq.First(), "merge element must be channel")
		wg.Add(1)
		go func(c *Channel) {
			defer wg.Done()
			registerGoroutineRT()
			for {
				v := asyncRecv(c)
				if v.Equals(NIL) {
					return
				}
				asyncSend(out, v)
			}
		}(ch)
		chsSeq = chsSeq.Rest()
	}
	go func() { wg.Wait(); out.Close() }()
	return out
}

func procAsyncSplit(args []Object) Object {
	CheckArity(args, 2, 2)
	pred := EnsureArgIsCallable(args, 0)
	in := channelFromArg(args, 1)
	t := MakeChannel(make(chan FutureResult))
	f := MakeChannel(make(chan FutureResult))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(in)
			if v.Equals(NIL) {
				t.Close()
				f.Close()
				return
			}
			if ToBool(call1(pred, v)) {
				asyncSend(t, v)
			} else {
				asyncSend(f, v)
			}
		}
	}()
	return collectionConstruction.NewVectorFrom(t, f)
}

func procAsyncMapFrom(args []Object) Object {
	CheckArity(args, 2, 2)
	xf := EnsureArgIsCallable(args, 0)
	in := channelFromArg(args, 1)
	out := MakeChannel(make(chan FutureResult))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(in)
			if v.Equals(NIL) {
				out.Close()
				return
			}
			asyncSend(out, call1(xf, v))
		}
	}()
	return out
}
func procAsyncFilterFrom(args []Object) Object {
	CheckArity(args, 2, 2)
	pred := EnsureArgIsCallable(args, 0)
	in := channelFromArg(args, 1)
	out := MakeChannel(make(chan FutureResult))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(in)
			if v.Equals(NIL) {
				out.Close()
				return
			}
			if ToBool(call1(pred, v)) {
				asyncSend(out, v)
			}
		}
	}()
	return out
}
func procAsyncMapTo(args []Object) Object {
	CheckArity(args, 2, 2)
	xf := EnsureArgIsCallable(args, 0)
	ch := channelFromArg(args, 1)
	out := MakeChannel(make(chan FutureResult))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(out)
			if v.Equals(NIL) {
				ch.Close()
				return
			}
			asyncSend(ch, call1(xf, v))
		}
	}()
	return out
}
func procAsyncFilterTo(args []Object) Object {
	CheckArity(args, 2, 2)
	pred := EnsureArgIsCallable(args, 0)
	ch := channelFromArg(args, 1)
	out := MakeChannel(make(chan FutureResult))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(out)
			if v.Equals(NIL) {
				ch.Close()
				return
			}
			if ToBool(call1(pred, v)) {
				asyncSend(ch, v)
			}
		}
	}()
	return out
}

func procAsyncReduce(args []Object) Object {
	CheckArity(args, 3, 3)
	f := EnsureArgIsCallable(args, 0)
	acc := args[1]
	ch := channelFromArg(args, 2)
	out := MakeChannel(make(chan FutureResult, 1))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(ch)
			if v.Equals(NIL) {
				asyncSend(out, acc)
				out.Close()
				return
			}
			acc = call2(f, acc, v)
		}
	}()
	return out
}
func procAsyncInto(args []Object) Object {
	CheckArity(args, 2, 2)
	init := args[0]
	ch := channelFromArg(args, 1)
	out := MakeChannel(make(chan FutureResult, 1))
	go func() {
		registerGoroutineRT()
		acc := init
		for {
			v := asyncRecv(ch)
			if v.Equals(NIL) {
				asyncSend(out, acc)
				out.Close()
				return
			}
			if c, ok := acc.(coretypes.Conjable); ok {
				acc = c.Conj(v).(Object)
			} else {
				panic(RT.NewError("into init is not conjable"))
			}
		}
	}()
	return out
}

type asyncMult struct {
	mu   sync.Mutex
	src  *Channel
	taps map[*Channel]bool
	hash uint32
}

func (m *asyncMult) ToString(bool) string                  { return "#object[core.async.Mult]" }
func (m *asyncMult) Print(w fmt.State, printReadably bool) {}
func (m *asyncMult) Equals(o interface{}) bool             { return m == o }
func (m *asyncMult) GetInfo() *coretypes.ObjectInfo        { return nil }
func (m *asyncMult) WithInfo(*coretypes.ObjectInfo) Object { return m }
func (m *asyncMult) GetType() *coretypes.Type              { return TYPE.Proc }
func (m *asyncMult) Hash() uint32                          { return m.hash }

type asyncPub struct {
	mu      sync.Mutex
	src     *Channel
	topicFn Callable
	subs    map[string][]*Channel
	hash    uint32
}

func (p *asyncPub) ToString(bool) string                  { return "#object[core.async.Pub]" }
func (p *asyncPub) Equals(o interface{}) bool             { return p == o }
func (p *asyncPub) GetInfo() *coretypes.ObjectInfo        { return nil }
func (p *asyncPub) WithInfo(*coretypes.ObjectInfo) Object { return p }
func (p *asyncPub) GetType() *coretypes.Type              { return TYPE.Proc }
func (p *asyncPub) Hash() uint32                          { return p.hash }

func procAsyncMult(args []Object) Object {
	CheckArity(args, 1, 1)
	src := channelFromArg(args, 0)
	m := &asyncMult{src: src, taps: map[*Channel]bool{}}
	m.hash = hashutil.Ptr(uintptr(unsafe.Pointer(m)))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(src)
			m.mu.Lock()
			taps := make([]*Channel, 0, len(m.taps))
			for t := range m.taps {
				taps = append(taps, t)
			}
			m.mu.Unlock()
			if v.Equals(NIL) {
				for _, t := range taps {
					t.Close()
				}
				return
			}
			for _, t := range taps {
				asyncSend(t, v)
			}
		}
	}()
	return m
}
func procAsyncTap(args []Object) Object {
	if len(args) < 2 || len(args) > 3 {
		panic(RT.NewError("tap requires mult, channel, optional close?"))
	}
	m, ok := args[0].(*asyncMult)
	if !ok {
		panic(RT.NewError("tap requires mult"))
	}
	ch := channelFromArg(args, 1)
	closep := true
	if len(args) == 3 {
		closep = ToBool(args[2])
	}
	m.mu.Lock()
	m.taps[ch] = closep
	m.mu.Unlock()
	return ch
}
func procAsyncUntap(args []Object) Object {
	CheckArity(args, 2, 2)
	m, ok := args[0].(*asyncMult)
	if !ok {
		panic(RT.NewError("untap requires mult"))
	}
	ch := channelFromArg(args, 1)
	m.mu.Lock()
	delete(m.taps, ch)
	m.mu.Unlock()
	return NIL
}
func procAsyncUntapAll(args []Object) Object {
	CheckArity(args, 1, 1)
	m, ok := args[0].(*asyncMult)
	if !ok {
		panic(RT.NewError("untap-all requires mult"))
	}
	m.mu.Lock()
	m.taps = map[*Channel]bool{}
	m.mu.Unlock()
	return NIL
}

func procAsyncPub(args []Object) Object {
	CheckArity(args, 2, 2)
	src := channelFromArg(args, 0)
	tf := EnsureArgIsCallable(args, 1)
	p := &asyncPub{src: src, topicFn: tf, subs: map[string][]*Channel{}}
	p.hash = hashutil.Ptr(uintptr(unsafe.Pointer(p)))
	go func() {
		registerGoroutineRT()
		for {
			v := asyncRecv(src)
			p.mu.Lock()
			if v.Equals(NIL) {
				for _, ss := range p.subs {
					for _, ch := range ss {
						ch.Close()
					}
				}
				p.mu.Unlock()
				return
			}
			topic := call1(tf, v).ToString(false)
			ss := append([]*Channel(nil), p.subs[topic]...)
			p.mu.Unlock()
			for _, ch := range ss {
				asyncSend(ch, v)
			}
		}
	}()
	return p
}
func procAsyncSub(args []Object) Object {
	if len(args) < 3 || len(args) > 4 {
		panic(RT.NewError("sub requires pub, topic, channel, optional close?"))
	}
	p, ok := args[0].(*asyncPub)
	if !ok {
		panic(RT.NewError("sub requires pub"))
	}
	topic := args[1].ToString(false)
	ch := channelFromArg(args, 2)
	p.mu.Lock()
	p.subs[topic] = append(p.subs[topic], ch)
	p.mu.Unlock()
	return ch
}
func procAsyncUnsub(args []Object) Object {
	CheckArity(args, 3, 3)
	p, ok := args[0].(*asyncPub)
	if !ok {
		panic(RT.NewError("unsub requires pub"))
	}
	topic := args[1].ToString(false)
	ch := channelFromArg(args, 2)
	p.mu.Lock()
	xs := p.subs[topic]
	ys := xs[:0]
	for _, c := range xs {
		if c != ch {
			ys = append(ys, c)
		}
	}
	if len(ys) == 0 {
		delete(p.subs, topic)
	} else {
		p.subs[topic] = ys
	}
	p.mu.Unlock()
	return NIL
}
func procAsyncUnsubAll(args []Object) Object {
	if len(args) < 1 || len(args) > 2 {
		panic(RT.NewError("unsub-all requires pub and optional topic"))
	}
	p, ok := args[0].(*asyncPub)
	if !ok {
		panic(RT.NewError("unsub-all requires pub"))
	}
	p.mu.Lock()
	if len(args) == 2 {
		delete(p.subs, args[1].ToString(false))
	} else {
		p.subs = map[string][]*Channel{}
	}
	p.mu.Unlock()
	return NIL
}
