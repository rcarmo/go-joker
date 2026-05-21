package runtime

import (
	"fmt"

	"github.com/rcarmo/go-joker/core/bufferpool"
	coretypes "github.com/rcarmo/go-joker/core/types"
	corestr "github.com/rcarmo/go-joker/core/types/string"
)

// Traceable is the minimal runtime-facing interface required for stack frames.
type Traceable interface {
	Name() string
	Pos() coretypes.Position
}

type Frame struct {
	traceable Traceable
}

type Callstack struct {
	frames []Frame
}

func NewCallstack(capacity int) *Callstack {
	return &Callstack{frames: make([]Frame, 0, capacity)}
}

func (s *Callstack) Push(traceable Traceable) {
	s.frames = append(s.frames, Frame{traceable: traceable})
}

func (s *Callstack) Pop() {
	s.frames = s.frames[:len(s.frames)-1]
}

func (s *Callstack) Clone() *Callstack {
	res := &Callstack{frames: make([]Frame, len(s.frames))}
	copy(res.frames, s.frames)
	return res
}

func (s *Callstack) Len() int { return len(s.frames) }

func (s *Callstack) FirstPos() coretypes.Position {
	if len(s.frames) == 0 || s.frames[0].traceable == nil {
		return coretypes.Position{}
	}
	return s.frames[0].traceable.Pos()
}

func (s *Callstack) Stacktrace(current Traceable) string {
	b := bufferpool.Get()
	defer bufferpool.Put(b)
	pos := coretypes.Position{}
	if current != nil {
		pos = current.Pos()
	}
	name := "global"
	for _, f := range s.frames {
		framePos := f.traceable.Pos()
		b.WriteString(fmt.Sprintf("  %s %s:%d:%d\n", name, framePos.FilenameOrUnknown(), framePos.StartLine, framePos.StartColumn))
		name = corestr.TrimVarQuotePrefix(f.traceable.Name())
	}
	b.WriteString(fmt.Sprintf("  %s %s:%d:%d", name, pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn))
	return b.String()
}

func (s *Callstack) String() string {
	b := bufferpool.Get()
	defer bufferpool.Put(b)
	for _, f := range s.frames {
		pos := f.traceable.Pos()
		b.WriteString(fmt.Sprintf("%s %s:%d:%d\n", f.traceable.Name(), pos.FilenameOrUnknown(), pos.StartLine, pos.StartColumn))
	}
	if b.Len() > 0 {
		b.Truncate(b.Len() - 1)
	}
	return b.String()
}
