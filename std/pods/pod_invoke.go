package pods

import (
	"fmt"

	. "github.com/candid82/joker/core"
)

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
	res, err := p.invoke(varSym, callArgs)
	if err != nil {
		panic(RT.NewError("pods/invoke: " + err.Error()))
	}
	return res
}

func (p *Pod) invoke(varName string, args []Object) (Object, error) {
	encoded, err := p.encodeArgs(args)
	if err != nil {
		return NIL, fmt.Errorf("encode args: %w", err)
	}
	id := p.nextID()
	ch := p.registerPending(id)
	if err := p.send(podMessage{"op": "invoke", "id": id, "var": varName, "args": encoded}); err != nil {
		return NIL, err
	}
	var result Object = NIL
	for msg := range ch {
		if exMsg, ok := msg["ex-message"].(string); ok {
			return NIL, fmt.Errorf("%s", exMsg)
		}
		if valStr, ok := msg["value"].(string); ok {
			result, err = p.decodePayload(valStr)
			if err != nil {
				return NIL, fmt.Errorf("decode value: %w", err)
			}
		}
	}
	return result, nil
}
