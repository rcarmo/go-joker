package runtime

import (
	"reflect"
	"sync"

	"github.com/rcarmo/go-joker/core/hashutil"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

var AgentRegisterGoroutine func()
var AgentUnregisterGoroutine func()

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

type agentCallableFunc func([]coretypes.Object) coretypes.Object

func (f agentCallableFunc) Call(args []coretypes.Object) coretypes.Object { return f(args) }

func NewAgent(initVal coretypes.Object) *Agent {
	a := &Agent{
		value: initVal,
		queue: make(chan agentAction, 256),
	}
	go a.processLoop()
	return a
}

func (a *Agent) processLoop() {
	if AgentRegisterGoroutine != nil {
		AgentRegisterGoroutine()
	}
	if AgentUnregisterGoroutine != nil {
		defer AgentUnregisterGoroutine()
	}
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
func (a *Agent) GetType() *coretypes.Type       { return coretypes.RuntimeTypes.Fn }
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

func (a *Agent) Send(fn coretypes.Callable, args []coretypes.Object) {
	a.queue <- agentAction{fn: fn, args: args}
}

func (a *Agent) Await() {
	done := make(chan struct{})
	a.queue <- agentAction{
		fn: agentCallableFunc(func(fnArgs []coretypes.Object) coretypes.Object {
			close(done)
			return fnArgs[0]
		}),
	}
	<-done
}

func (a *Agent) Error() coretypes.Error {
	a.mu.Lock()
	e := a.err
	a.mu.Unlock()
	return e
}
