package runtime

import (
	"fmt"
	"unsafe"

	"github.com/rcarmo/go-joker/core/hashutil"
	coretypes "github.com/rcarmo/go-joker/core/types"
)

type EvalError struct {
	msg        string
	pos        coretypes.Position
	rt         *GoroutineRT
	hash       uint32
	linterMode bool
}

func NewEvalError(msg string, pos coretypes.Position, rt *GoroutineRT, linterMode bool) *EvalError {
	res := &EvalError{msg: msg, pos: pos, rt: rt, linterMode: linterMode}
	res.hash = hashutil.Ptr(uintptr(unsafe.Pointer(res)))
	return res
}

func (err *EvalError) ToString(escape bool) string                          { return err.Error() }
func (err *EvalError) Equals(other interface{}) bool                        { return err == other }
func (err *EvalError) GetInfo() *coretypes.ObjectInfo                       { return nil }
func (err *EvalError) GetType() *coretypes.Type                             { return coretypes.RuntimeTypes.EvalError }
func (err *EvalError) Hash() uint32                                         { return err.hash }
func (err *EvalError) WithInfo(info *coretypes.ObjectInfo) coretypes.Object { return err }
func (err *EvalError) Message() coretypes.Object                            { return coretypes.MakeString(err.msg) }

func (err *EvalError) Error() string {
	pos := err.pos
	if err.rt != nil && err.rt.Callstack.Len() > 0 && !err.linterMode {
		return fmt.Sprintf("%s:%d:%d: Eval error: %s\nStacktrace:\n%s", pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn, err.msg, err.rt.Callstack.Stacktrace(asTraceable(err.rt.CurrentExpr)))
	}
	if err.rt != nil && err.rt.Callstack.Len() > 0 {
		pos = err.rt.Callstack.FirstPos()
	}
	return fmt.Sprintf("%s:%d:%d: Eval error: %s", pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn, err.msg)
}

func asTraceable(v any) Traceable {
	if t, ok := v.(Traceable); ok {
		return t
	}
	return nil
}
