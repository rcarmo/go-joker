package runtime

import (
	"testing"

	coretypes "github.com/rcarmo/go-joker/core/types"
)

type agentTestCallable func([]coretypes.Object) coretypes.Object

func (f agentTestCallable) Call(args []coretypes.Object) coretypes.Object { return f(args) }

func TestAgentSendAwaitAndDeref(t *testing.T) {
	a := NewAgent(coretypes.Int{I: 1})
	a.Send(agentTestCallable(func(args []coretypes.Object) coretypes.Object {
		current := args[0].(coretypes.Int).I
		delta := args[1].(coretypes.Int).I
		return coretypes.Int{I: current + delta}
	}), []coretypes.Object{coretypes.Int{I: 4}})
	a.Await()
	if got := a.Deref().(coretypes.Int).I; got != 5 {
		t.Fatalf("agent value = %d, want 5", got)
	}
}

func TestAgentRecordsCoretypesError(t *testing.T) {
	a := NewAgent(coretypes.Int{I: 1})
	sentinel := channelObjectTestError("agent boom")
	a.Send(agentTestCallable(func(args []coretypes.Object) coretypes.Object {
		panic(sentinel)
	}), nil)
	a.Await()
	if got := a.Error(); got != sentinel {
		t.Fatalf("agent error = %v, want sentinel", got)
	}
	if got := a.Deref().(coretypes.Int).I; got != 1 {
		t.Fatalf("agent value changed after failed action: %d", got)
	}
}
